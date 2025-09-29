// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package asynccharmdownloader

import (
	"context"
	"net/url"
	"os"
	"time"

	"github.com/juju/clock"
	"github.com/juju/retry"
	"github.com/juju/worker/v4"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/logger"
	domainapplication "github.com/juju/juju/domain/application"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/internal/charm/charmdownloader"
	"github.com/juju/juju/internal/errors"
)

const (
	retryAttempts = 3
	retryDelay    = 20 * time.Second
)

type asyncDownloadWorker struct {
	tomb tomb.Tomb

	appID              application.UUID
	applicationService ApplicationService
	downloader         Downloader

	clock  clock.Clock
	logger logger.Logger
}

// NewAsyncDownloadWorker creates a new async worker that downloads charms for
// the specified application.
func NewAsyncDownloadWorker(
	appID application.UUID,
	applicationService ApplicationService,
	downloader Downloader,
	clock clock.Clock,
	logger logger.Logger,
) worker.Worker {
	w := &asyncDownloadWorker{
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
func (w *asyncDownloadWorker) Kill() {
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *asyncDownloadWorker) Wait() error {
	return w.tomb.Wait()
}

func (w *asyncDownloadWorker) loop() error {
	ctx, cancel := w.scopedContext()
	defer cancel()

	w.logger.Infof(ctx, "downloading charm for application %q", w.appID)

	info, err := w.applicationService.GetAsyncCharmDownloadInfo(ctx, w.appID)
	if errors.Is(err, applicationerrors.CharmAlreadyAvailable) {
		// If the application is already downloading a charm, we can skip this
		// application.
		return nil
	} else if err != nil {
		return errors.Capture(err)
	}

	// Ensure we've got a valid URL.
	url, err := url.Parse(info.DownloadInfo.DownloadURL)
	if err != nil {
		return errors.Capture(err)
	}

	// Download the charm for the application.
	var result *charmdownloader.DownloadResult
	if err := retry.Call(retry.CallArgs{
		Func: func() error {
			result, err = w.downloader.Download(ctx, url, info.SHA256)
			if err != nil {
				return errors.Capture(err)
			}
			return nil
		},
		Attempts: retryAttempts,
		Delay:    retryDelay,
		Clock:    w.clock,
		NotifyFunc: func(err error, i int) {
			w.logger.Warningf(ctx, "failed to download charm for application %q, attempt %d: %v", w.appID, i, err)
		},
		Stop: w.tomb.Dying(),
	}); err != nil {
		return errors.Capture(err)
	}

	// Ensure the charm is removed after the worker has finished.
	defer func() {
		if err := os.Remove(result.Path); err != nil && !os.IsNotExist(err) {
			w.logger.Warningf(ctx, "failed to remove temporary file %q: %v", result.Path, err)
		}
	}()

	// The charm has been downloaded, we can now resolve the download slot.
	err = w.applicationService.ResolveCharmDownload(ctx, w.appID, domainapplication.ResolveCharmDownload{
		SHA256:    result.SHA256,
		SHA384:    result.SHA384,
		CharmUUID: info.CharmUUID,
		Path:      result.Path,
		Size:      result.Size,
	})
	if err != nil && !errors.Is(err, applicationerrors.CharmAlreadyResolved) {
		return errors.Capture(err)
	}

	// Exit cleanly, so the worker doesn't get restarted.
	return nil
}

func (w *asyncDownloadWorker) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(w.tomb.Context(context.Background()))
}
