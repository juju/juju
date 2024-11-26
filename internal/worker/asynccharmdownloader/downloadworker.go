// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package asynccharmdownloader

import (
	"context"
	"strings"
	"time"

	"github.com/juju/clock"
	jujuerrors "github.com/juju/errors"
	"github.com/juju/retry"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/application"
	corehttp "github.com/juju/juju/core/http"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/watcher"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/charm/charmdownloader"
	"github.com/juju/juju/internal/errors"
)

// ModelConfigService provides access to the model configuration.
type ModelConfigService interface {
	// ModelConfig returns the current config for the model.
	ModelConfig(context.Context) (*config.Config, error)
}

// ApplicationService describes the API exposed by the charm downloader facade.
type ApplicationService interface {
	// WatchApplicationsWithPendingCharms returns a watcher that notifies of
	// changes to applications that reference charms that have not yet been
	// downloaded.
	WatchApplicationsWithPendingCharms(ctx context.Context) (watcher.StringsWatcher, error)

	// ReserveCharmDownload reserves a charm download slot for the specified
	// application. If the charm is already being downloaded, the method will
	// return [applicationerrors.AlreadyDownloadingCharm]. The charm download
	// information is returned which includes the charm name, origin and the
	// digest.
	ReserveCharmDownload(ctx context.Context, appID application.ID) (application.CharmDownloadInfo, error)

	// ResolveCharmDownload resolves the charm download slot for the specified
	// application. The method will update the charm with the specified charm
	// information.
	ResolveCharmDownload(ctx context.Context, resolve application.ResolveCharmDownload) error
}

// Config defines the operation of a Worker.
type Config struct {
	ApplicationService ApplicationService
	ModelConfigService ModelConfigService
	HTTPClientGetter   corehttp.HTTPClientGetter
	NewHTTPClient      NewHTTPClientFunc
	NewDownloader      NewDownloaderFunc
	Logger             logger.Logger
	Clock              clock.Clock
}

// Validate returns an error if cfg cannot drive a Worker.
func (cfg Config) Validate() error {
	if cfg.ApplicationService == nil {
		return jujuerrors.NotValidf("nil ApplicationService")
	}
	if cfg.ModelConfigService == nil {
		return jujuerrors.NotValidf("nil ModelConfigService")
	}
	if cfg.HTTPClientGetter == nil {
		return jujuerrors.NotValidf("nil HTTPClientGetter")
	}
	if cfg.NewHTTPClient == nil {
		return jujuerrors.NotValidf("nil NewHTTPClient")
	}
	if cfg.NewDownloader == nil {
		return jujuerrors.NotValidf("nil NewDownloader")
	}
	if cfg.Clock == nil {
		return jujuerrors.NotValidf("nil Clock")
	}
	if cfg.Logger == nil {
		return jujuerrors.NotValidf("nil Logger")
	}
	return nil
}

// charmDownloaderWorker watches applications that reference charms that have not
// yet been downloaded and triggers an asynchronous download request for each
// one.
type charmDownloaderWorker struct {
	internalStates chan string
	catacomb       catacomb.Catacomb
	runner         *worker.Runner

	config Config
}

// NewWorker returns a new charmDownloaderWorker worker.
func NewWorker(config Config) (worker.Worker, error) {
	return newWorker(config, nil)
}

// newWorker returns a new charmDownloaderWorker worker.
func newWorker(config Config, internalState chan string) (*charmDownloaderWorker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Capture(err)
	}

	cd := &charmDownloaderWorker{
		config: config,
		runner: worker.NewRunner(worker.RunnerParams{
			IsFatal: func(err error) bool {
				return false
			},
			ShouldRestart: func(err error) bool {
				return false
			},
			RestartDelay: time.Second * 10,
			Clock:        config.Clock,
			Logger:       config.Logger,
		}),
		internalStates: internalState,
	}

	err := catacomb.Invoke(catacomb.Plan{
		Site: &cd.catacomb,
		Work: cd.loop,
	})
	if err != nil {
		return nil, errors.Capture(err)
	}
	return cd, nil
}

// Kill is part of the worker.Worker interface.
func (w *charmDownloaderWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *charmDownloaderWorker) Wait() error {
	return w.catacomb.Wait()
}

func (w *charmDownloaderWorker) loop() error {
	logger := w.config.Logger

	ctx, cancel := w.scopedContext()
	defer cancel()

	applicationService := w.config.ApplicationService
	watcher, err := applicationService.WatchApplicationsWithPendingCharms(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	if err := w.catacomb.Add(watcher); err != nil {
		return errors.Capture(err)
	}

	logger.Debugf("watching applications referencing charms that have not yet been downloaded")

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case changes, ok := <-watcher.Changes():
			if !ok {
				return errors.New("application watcher closed")
			}

			if len(changes) == 0 {
				continue
			}

			logger.Debugf("triggering asynchronous download of charms for the following applications: %v", strings.Join(changes, ", "))

			// Get a new downloader, this ensures that we've got a fresh
			// connection to the charm store.
			httpClient, err := w.config.NewHTTPClient(ctx, w.config.HTTPClientGetter)
			if err != nil {
				return errors.Capture(err)
			}

			downloader := w.config.NewDownloader(httpClient, w.config.ModelConfigService, logger)

			// Start up a series of workers to download the charms for the
			// applications asynchronously.
			for _, change := range changes {
				appID, err := application.ParseID(change)
				if err != nil {
					logger.Errorf("failed to parse application ID %q: %v", change, err)
					continue
				}

				if tracked, err := w.workerFromCache(appID); err != nil {
					return errors.Errorf("getting download worker from the cache %q: %v", appID, err)
				} else if tracked != nil {
					// Already tracking this application, skip it.
					continue
				}

				// Kick off the download worker for the application.
				if err := w.initTrackerWorker(appID, downloader); err != nil {
					return errors.Capture(err)
				}
			}
		}
	}
}

func (w *charmDownloaderWorker) workerFromCache(appID application.ID) (*downloadWorker, error) {
	// If the worker already exists, return the existing worker early.
	if wrk, err := w.runner.Worker(appID.String(), w.catacomb.Dying()); err == nil {
		return wrk.(*downloadWorker), nil
	} else if errors.Is(err, worker.ErrDead) {
		// Handle the case where the runner is dead due to this worker dying.
		select {
		case <-w.catacomb.Dying():
			return nil, w.catacomb.ErrDying()
		default:
			return nil, errors.Capture(err)
		}
	} else if !errors.Is(err, jujuerrors.NotFound) {
		// If it's not a NotFound error, return the underlying error. We should
		// only start a worker if it doesn't exist yet.
		return nil, errors.Capture(err)
	}
	// We didn't find the worker, so return nil, we'll create it in the next
	// step.
	return nil, nil
}

func (w *charmDownloaderWorker) initTrackerWorker(appID application.ID, downloader Downloader) error {
	err := w.runner.StartWorker(appID.String(), func() (worker.Worker, error) {
		wrk := newDownloadWorker(
			appID,
			w.config.ApplicationService,
			downloader,
			w.config.Clock,
			w.config.Logger,
		)
		return wrk, nil
	})
	if errors.Is(err, jujuerrors.AlreadyExists) {
		return nil
	}
	return errors.Capture(err)
}

func (w *charmDownloaderWorker) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(w.catacomb.Context(context.Background()))
}

type downloadWorker struct {
	tomb tomb.Tomb

	appID              application.ID
	applicationService ApplicationService
	downloader         Downloader

	clock  clock.Clock
	logger logger.Logger
}

func newDownloadWorker(
	appID application.ID,
	applicationService ApplicationService,
	downloader Downloader,
	clock clock.Clock,
	logger logger.Logger,
) *downloadWorker {
	w := &downloadWorker{
		appID:              appID,
		applicationService: applicationService,
		downloader:         downloader,
		clock:              clock,
		logger:             logger,
	}
	w.tomb.Go(w.loop)
	return w
}

// Kill is part of the worker.Worker interface.
func (w *downloadWorker) Kill() {
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *downloadWorker) Wait() error {
	return w.tomb.Wait()
}

func (w *downloadWorker) loop() error {
	ctx, cancel := w.scopedContext()
	defer cancel()

	w.logger.Infof("downloading charm for application %q", w.appID)

	info, err := w.applicationService.ReserveCharmDownload(ctx, w.appID)
	if errors.Is(err, applicationerrors.AlreadyDownloadingCharm) {
		// If the application is already downloading a charm, we can skip this
		// application.
		return nil
	} else if err != nil {
		return errors.Capture(err)
	}

	// Download the charm for the application.
	var result *charmdownloader.DownloadResult
	if err := retry.Call(retry.CallArgs{
		Func: func() error {
			result, err = w.downloader.Download(ctx, info.Name, info.Origin)
			if err != nil {
				return errors.Capture(err)
			}
			return nil
		},
		Attempts: 3,
		Delay:    5 * time.Second,
		Clock:    w.clock,
		IsFatalError: func(err error) bool {
			return false
		},
		NotifyFunc: func(err error, i int) {
			w.logger.Warningf("failed to download charm for application %q, attempt %d: %v", w.appID, i, err)
		},
		Stop: w.tomb.Dying(),
	}); err != nil {
		return errors.Capture(err)
	}

	// The charm has been downloaded, we can now resolve the download slot.
	err = w.applicationService.ResolveCharmDownload(ctx, application.ResolveCharmDownload{
		CharmUUID: info.CharmUUID,
		Path:      result.Path,
		Origin:    info.Origin,
		Size:      result.Size,
	})
	if err != nil {
		return errors.Capture(err)
	}

	return nil
}

func (w *downloadWorker) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(w.tomb.Context(context.Background()))
}
