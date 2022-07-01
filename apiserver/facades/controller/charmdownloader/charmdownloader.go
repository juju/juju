// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmdownloader

import (
	"sync"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/http/v2"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"

	apiservererrors "github.com/juju/juju/v3/apiserver/errors"
	"github.com/juju/juju/v3/apiserver/facades/client/charms/services"
	"github.com/juju/juju/v3/core/status"
	"github.com/juju/juju/v3/rpc/params"
	"github.com/juju/juju/v3/state/watcher"
)

var logger = loggo.GetLogger("juju.apiserver.charmdownloader")

// CharmDownloaderAPI implements an API for watching the charms collection for
// any entries that have not been yet downloaded to the blobstore and for
// triggering their download.
type CharmDownloaderAPI struct {
	authChecker      AuthChecker
	resourcesBackend ResourcesBackend
	stateBackend     StateBackend
	modelBackend     ModelBackend
	clock            clock.Clock
	httpClient       http.HTTPClient

	newStorage    func(modelUUID string) services.Storage
	newDownloader func(services.CharmDownloaderConfig) (Downloader, error)

	mu         sync.Mutex
	downloader Downloader
}

// newAPI is invoked both by the facade constructor and from our tests. It
// allows us to pass interfaces for the facade's dependencies.
func newAPI(
	authChecker AuthChecker,
	resourcesBackend ResourcesBackend,
	stateBackend StateBackend,
	modelBackend ModelBackend,
	clk clock.Clock,
	httpClient http.HTTPClient,
	newStorage func(string) services.Storage,
	newDownloader func(services.CharmDownloaderConfig) (Downloader, error),
) *CharmDownloaderAPI {
	return &CharmDownloaderAPI{
		authChecker:      authChecker,
		resourcesBackend: resourcesBackend,
		stateBackend:     stateBackend,
		modelBackend:     modelBackend,
		clock:            clk,
		httpClient:       httpClient,
		newStorage:       newStorage,
		newDownloader:    newDownloader,
	}
}

// WatchApplicationsWithPendingCharms registers and returns a watcher instance
// that reports the ID of applications that reference a charm which has not yet
// been downloaded.
func (a *CharmDownloaderAPI) WatchApplicationsWithPendingCharms() (params.StringsWatchResult, error) {
	if !a.authChecker.AuthController() {
		return params.StringsWatchResult{}, apiservererrors.ErrPerm
	}

	w := a.stateBackend.WatchApplicationsWithPendingCharms()
	if initialState, ok := <-w.Changes(); ok {
		return params.StringsWatchResult{
			StringsWatcherId: a.resourcesBackend.Register(w),
			Changes:          initialState,
		}, nil
	}

	return params.StringsWatchResult{}, watcher.EnsureErr(w)
}

// DownloadApplicationCharms iterates the list of provided applications and
// downloads any referenced charms that have not yet been persisted to the
// blob store.
func (a *CharmDownloaderAPI) DownloadApplicationCharms(args params.Entities) (params.ErrorResults, error) {
	if !a.authChecker.AuthController() {
		return params.ErrorResults{}, apiservererrors.ErrPerm
	}

	res := params.ErrorResults{Results: make([]params.ErrorResult, len(args.Entities))}
	for i, arg := range args.Entities {
		app, err := names.ParseApplicationTag(arg.Tag)
		if err != nil {
			res.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		res.Results[i].Error = apiservererrors.ServerError(a.downloadApplicationCharm(app))
	}
	return res, nil
}

func (a *CharmDownloaderAPI) downloadApplicationCharm(appTag names.ApplicationTag) error {
	app, err := a.stateBackend.Application(appTag.Name)
	if err != nil {
		return errors.Trace(err)
	}

	if !app.CharmPendingToBeDownloaded() {
		return nil // nothing to do
	}

	pendingCharm, force, err := app.Charm()
	if err != nil {
		return errors.Trace(err)
	}
	pendingCharmURL := pendingCharm.URL()

	macaroons, err := pendingCharm.Macaroon()
	if err != nil {
		return errors.Trace(err)
	}

	resolvedOrigin := app.CharmOrigin()
	if resolvedOrigin == nil {
		return errors.NotFoundf("download charm for application %q; resolved origin", appTag.Name)
	}

	downloader, err := a.getDownloader()
	if err != nil {
		return errors.Trace(err)
	}

	now := a.clock.Now()
	err = app.SetStatus(status.StatusInfo{
		Status:  status.Maintenance,
		Message: "downloading charm",
		Data: map[string]interface{}{
			"origin":    *resolvedOrigin,
			"charm-url": pendingCharmURL,
			"force":     force,
		},
		Since: &now,
	})
	if err != nil {
		return errors.Trace(err)
	}

	if _, err := downloader.DownloadAndStore(pendingCharmURL, *resolvedOrigin, macaroons, force); err != nil {
		now = a.clock.Now()
		// Update app status; it's fine if this fails as we just want
		// to report the download error back. Also, we use a fairly
		// generic error message instead of the actual error to avoid
		// accidentally leaking any auth-related details that may be
		// contained in the error.
		statusErr := app.SetStatus(status.StatusInfo{
			Status:  status.Blocked,
			Message: "unable to download charm",
			Since:   &now,
		})
		if statusErr != nil {
			logger.Errorf("unable to set application status: %v", err)
		}
		return errors.Trace(err)
	}

	// Update app status; it's fine if this fails as the charm has been
	// stored and can be fetched by the machine agents.
	now = a.clock.Now()
	statusErr := app.SetStatus(status.StatusInfo{
		// Let the charm set the application status
		Status: status.Unknown,
		// Just clear the "downloading charm" message to reduce noise
		// in juju status output.
		Message: "",
		Since:   &now,
	})
	if statusErr != nil {
		logger.Errorf("unable to set application status: %v", err)
	}

	return nil
}

func (a *CharmDownloaderAPI) getDownloader() (Downloader, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.downloader != nil {
		return a.downloader, nil
	}

	downloader, err := a.newDownloader(services.CharmDownloaderConfig{
		Logger:         logger,
		Transport:      a.httpClient,
		StorageFactory: a.newStorage,
		StateBackend:   a.stateBackend,
		ModelBackend:   a.modelBackend,
	})

	if err != nil {
		return nil, errors.Trace(err)
	}

	a.downloader = downloader
	return downloader, nil
}
