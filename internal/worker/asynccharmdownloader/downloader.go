package asynccharmdownloader

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/retry"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/logger"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/internal/charm/charmdownloader"
	"github.com/juju/juju/internal/errors"
)

const (
	retryAttempts = 3
	retryDelay    = 20 * time.Second
)

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
		Attempts: retryAttempts,
		Delay:    retryDelay,
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
	if err != nil && errors.Is(err, applicationerrors.CharmAlreadyResolved) {
		return errors.Capture(err)
	}

	// Exit cleanly, so the worker doesn't get restarted.
	return nil
}

func (w *downloadWorker) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(w.tomb.Context(context.Background()))
}
