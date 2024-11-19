// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmdownloader

import (
	"context"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/watcher"
)

// ApplicationService describes the API exposed by the charm downloader facade.
type ApplicationService interface {
	// WatchApplicationsWithPendingCharms returns a watcher that notifies of
	// changes to applications that reference charms that have not yet been
	// downloaded.
	WatchApplicationsWithPendingCharms(ctx context.Context) (watcher.StringsWatcher, error)
	// DownloadApplicationCharms triggers a download of the charms referenced by
	// the given applications.
	DownloadApplicationCharms(ctx context.Context, applications []application.ID) error
}

// Config defines the operation of a Worker.
type Config struct {
	Logger             logger.Logger
	ApplicationService ApplicationService
}

// Validate returns an error if cfg cannot drive a Worker.
func (cfg Config) Validate() error {
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if cfg.ApplicationService == nil {
		return errors.NotValidf("nil ApplicationService")
	}
	return nil
}

// CharmDownloader watches applications that reference charms that have not
// yet been downloaded and triggers an asynchronous download request for each
// one.
type CharmDownloader struct {
	applicationService ApplicationService
	logger             logger.Logger

	catacomb catacomb.Catacomb
}

// NewWorker returns a new CharmDownloader worker.
func NewWorker(cfg Config) (*CharmDownloader, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	cd := &CharmDownloader{
		logger:             cfg.Logger,
		applicationService: cfg.ApplicationService,
	}

	err := catacomb.Invoke(catacomb.Plan{
		Site: &cd.catacomb,
		Work: cd.loop,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return cd, nil
}

func (cd *CharmDownloader) loop() error {
	ctx, cancel := cd.scopedContext()
	defer cancel()

	watcher, err := cd.applicationService.WatchApplicationsWithPendingCharms(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	if err := cd.catacomb.Add(watcher); err != nil {
		return errors.Trace(err)
	}

	cd.logger.Debugf("watching applications referencing charms that have not yet been downloaded")

	for {
		select {
		case <-cd.catacomb.Dying():
			return cd.catacomb.ErrDying()
		case changes, ok := <-watcher.Changes():
			if !ok {
				return errors.New("application watcher closed")
			}

			if len(changes) == 0 {
				continue
			}

			appIDs := make([]application.ID, len(changes))
			for i, change := range changes {
				appID, err := application.ParseID(change)
				if err != nil {
					cd.logger.Warningf("ignoring invalid application ID %q: %v", change, err)
					continue
				}
				appIDs[i] = appID
			}

			cd.logger.Debugf("triggering asynchronous download of charms for the following applications: %v", strings.Join(changes, ", "))
			if err := cd.applicationService.DownloadApplicationCharms(ctx, appIDs); err != nil {
				return errors.Trace(err)
			}
		}
	}
}

// Kill is part of the worker.Worker interface.
func (cd *CharmDownloader) Kill() {
	cd.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (cd *CharmDownloader) Wait() error {
	return cd.catacomb.Wait()
}

func (cd *CharmDownloader) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(cd.catacomb.Context(context.Background()))
}
