// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package asynccharmdownloader

import (
	"context"
	"strings"

	"github.com/juju/clock"
	jujuerrors "github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/core/application"
	corehttp "github.com/juju/juju/core/http"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/watcher"
	domainapplication "github.com/juju/juju/domain/application"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/errors"
)

const (
	// States which report the state of the worker.
	stateStarted = "started"
)

// ModelConfigService provides access to the model configuration.
type ModelConfigService interface {
	// ModelConfig returns the current config for the model.
	ModelConfig(context.Context) (*config.Config, error)

	// Watch returns a watcher that notifies of changes to the model config.
	Watch() (watcher.StringsWatcher, error)
}

// ApplicationService describes the API exposed by the charm downloader facade.
type ApplicationService interface {
	// WatchApplicationsWithPendingCharms returns a watcher that notifies of
	// changes to applications that reference charms that have not yet been
	// downloaded.
	// Each string will be an individual application ID. It's possible to
	// have the same application ID multiple times in the list.
	WatchApplicationsWithPendingCharms(ctx context.Context) (watcher.StringsWatcher, error)

	// ReserveCharmDownload reserves a charm download slot for the specified
	// application. If the charm is already being downloaded, the method will
	// return [applicationerrors.AlreadyDownloadingCharm]. The charm download
	// information is returned which includes the charm name, origin and the
	// digest.
	ReserveCharmDownload(ctx context.Context, appID application.ID) (domainapplication.CharmDownloadInfo, error)

	// ResolveCharmDownload resolves the charm download slot for the specified
	// application. The method will update the charm with the specified charm
	// information.
	ResolveCharmDownload(ctx context.Context, appID application.ID, resolve domainapplication.ResolveCharmDownload) error
}

// Config defines the operation of a Worker.
type Config struct {
	ApplicationService     ApplicationService
	ModelConfigService     ModelConfigService
	HTTPClientGetter       corehttp.HTTPClientGetter
	NewHTTPClient          NewHTTPClientFunc
	NewDownloader          NewDownloaderFunc
	NewAsyncDownloadWorker NewAsyncDownloadWorkerFunc
	Logger                 logger.Logger
	Clock                  clock.Clock
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
	if cfg.NewAsyncDownloadWorker == nil {
		return jujuerrors.NotValidf("nil NewAsyncDownloadWorker")
	}
	if cfg.Clock == nil {
		return jujuerrors.NotValidf("nil Clock")
	}
	if cfg.Logger == nil {
		return jujuerrors.NotValidf("nil Logger")
	}
	return nil
}

// Worker watches applications that reference charms that have not
// yet been downloaded and triggers an asynchronous download request for each
// one.
type Worker struct {
	internalStates chan string
	catacomb       catacomb.Catacomb
	runner         *worker.Runner

	config Config
}

// NewWorker returns a new Worker worker.
func NewWorker(config Config) (*Worker, error) {
	return newWorker(config, nil)
}

// newWorker returns a new Worker worker.
func newWorker(config Config, internalState chan string) (*Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Capture(err)
	}

	cd := &Worker{
		config: config,
		runner: worker.NewRunner(worker.RunnerParams{
			IsFatal: func(err error) bool {
				return false
			},
			ShouldRestart: func(err error) bool {
				return false
			},
			Clock:  config.Clock,
			Logger: config.Logger,
		}),
		internalStates: internalState,
	}

	err := catacomb.Invoke(catacomb.Plan{
		Site: &cd.catacomb,
		Work: cd.loop,
		Init: []worker.Worker{cd.runner},
	})
	if err != nil {
		return nil, errors.Capture(err)
	}
	return cd, nil
}

// Kill is part of the worker.Worker interface.
func (w *Worker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *Worker) Wait() error {
	return w.catacomb.Wait()
}

func (w *Worker) loop() error {
	// Report the initial started state.
	w.reportInternalState(stateStarted)

	logger := w.config.Logger

	ctx, cancel := w.scopedContext()
	defer cancel()

	applicationService := w.config.ApplicationService
	appWatcher, err := applicationService.WatchApplicationsWithPendingCharms(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	if err := w.catacomb.Add(appWatcher); err != nil {
		return errors.Capture(err)
	}

	// Watch the model config for new charmhub URL values, so we can swap the
	// downloader to use the new URL.

	modelConfigService := w.config.ModelConfigService
	configWatcher, err := modelConfigService.Watch()
	if err != nil {
		return errors.Capture(err)
	}

	if err := w.catacomb.Add(configWatcher); err != nil {
		return errors.Capture(err)
	}

	logger.Debugf("watching applications referencing charms that have not yet been downloaded")

	downloader, err := w.getDownloader(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()

		case changes, ok := <-appWatcher.Changes():
			if !ok {
				return errors.New("application watcher closed")
			}

			if len(changes) == 0 {
				continue
			}

			logger.Debugf("triggering asynchronous download of charms for the following applications: %v", strings.Join(changes, ", "))

			// Start up a series of workers to download the charms for the
			// applications asynchronously. We do not want to block the any
			// further changes to the watcher, so fire off the workers as fast
			// as possible.
			for _, change := range changes {
				appID, err := application.ParseID(change)
				if err != nil {
					logger.Errorf("failed to parse application ID %q: %v", change, err)
					continue
				}

				if cached, err := w.workerFromCache(appID); err != nil {
					return errors.Errorf("getting download worker from the cache %q: %v", appID, err)
				} else if cached {
					// Already tracking this application, skip it.
					continue
				}

				// Kick off the async download worker for the application.
				if err := w.initAsyncDownloadWorker(appID, downloader); err != nil {
					return errors.Capture(err)
				}
			}

		case change, ok := <-configWatcher.Changes():
			if !ok {
				return errors.New("model config watcher closed")
			}

			var refresh bool
			for _, key := range change {
				if key == config.CharmHubURLKey {
					refresh = true
					break
				}
			}

			if !refresh {
				continue
			}

			logger.Debugf("refreshing downloader due to model config change")

			downloader, err = w.getDownloader(ctx)
			if err != nil {
				return errors.Capture(err)
			}
		}
	}
}

func (w *Worker) getDownloader(ctx context.Context) (Downloader, error) {
	// Get a new downloader, this ensures that we've got a fresh
	// connection to the charm store.
	httpClient, err := w.config.NewHTTPClient(ctx, w.config.HTTPClientGetter)
	if err != nil {
		return nil, errors.Capture(err)
	}

	config, err := w.config.ModelConfigService.ModelConfig(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	charmhubURL, _ := config.CharmHubURL()

	return w.config.NewDownloader(httpClient, charmhubURL, w.config.Logger)
}

func (w *Worker) workerFromCache(appID application.ID) (bool, error) {
	// If the worker already exists, return the existing worker early.
	if _, err := w.runner.Worker(appID.String(), w.catacomb.Dying()); err == nil {
		return true, nil
	} else if errors.Is(err, worker.ErrDead) {
		// Handle the case where the runner is dead due to this worker dying.
		select {
		case <-w.catacomb.Dying():
			return false, w.catacomb.ErrDying()
		default:
			return false, errors.Capture(err)
		}
	} else if !errors.Is(err, jujuerrors.NotFound) {
		// If it's not a NotFound error, return the underlying error. We should
		// only start a worker if it doesn't exist yet.
		return false, errors.Capture(err)
	}
	// We didn't find the worker, so return nil, we'll create it in the next
	// step.
	return false, nil
}

func (w *Worker) initAsyncDownloadWorker(appID application.ID, downloader Downloader) error {
	err := w.runner.StartWorker(appID.String(), func() (worker.Worker, error) {
		wrk := w.config.NewAsyncDownloadWorker(
			appID,
			w.config.ApplicationService,
			downloader,
			w.config.Clock,
			w.config.Logger,
		)
		return wrk, nil
	})

	// This can happen, because the StartWorker runner is asynchronous, so
	// multiple workers can be started at the same time for the same
	// application.
	if errors.Is(err, jujuerrors.AlreadyExists) {
		return nil
	}
	return errors.Capture(err)
}

func (w *Worker) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(w.catacomb.Context(context.Background()))
}

func (w *Worker) reportInternalState(state string) {
	select {
	case <-w.catacomb.Dying():
	case w.internalStates <- state:
	default:
	}
}
