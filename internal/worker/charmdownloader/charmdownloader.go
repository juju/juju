// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmdownloader

import (
	"context"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/watcher"
)

// Config defines the operation of a Worker.
type Config struct {
	Logger             logger.Logger
	CharmDownloaderAPI CharmDownloaderAPI
}

// Validate returns an error if cfg cannot drive a Worker.
func (cfg Config) Validate() error {
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if cfg.CharmDownloaderAPI == nil {
		return errors.NotValidf("nil CharmDownloader API")
	}
	return nil
}

// CharmDownloader watches applications that reference charms that have not
// yet been downloaded and triggers an asynchronous download request for each
// one.
type CharmDownloader struct {
	charmDownloaderAPI CharmDownloaderAPI
	logger             logger.Logger

	catacomb   catacomb.Catacomb
	appWatcher watcher.StringsWatcher
}

// NewCharmDownloader returns a new CharmDownloader worker.
func NewCharmDownloader(cfg Config) (worker.Worker, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	cd := &CharmDownloader{
		logger:             cfg.Logger,
		charmDownloaderAPI: cfg.CharmDownloaderAPI,
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

func (cd *CharmDownloader) setup(ctx context.Context) error {
	var err error
	cd.appWatcher, err = cd.charmDownloaderAPI.WatchApplicationsWithPendingCharms(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	if err := cd.catacomb.Add(cd.appWatcher); err != nil {
		return errors.Trace(err)
	}

	cd.logger.Debugf(context.TODO(), "started watching applications referencing charms that have not yet been downloaded")
	return nil
}

func (cd *CharmDownloader) loop() error {
	ctx, cancel := cd.scopedContext()
	defer cancel()

	if err := cd.setup(ctx); err != nil {
		return errors.Trace(err)
	}

	for {
		select {
		case <-cd.catacomb.Dying():
			return cd.catacomb.ErrDying()
		case changes, ok := <-cd.appWatcher.Changes():
			if !ok {
				return errors.New("application watcher closed")
			}

			if len(changes) == 0 {
				continue
			}

			appTags := make([]names.ApplicationTag, len(changes))
			for i, appName := range changes {
				appTags[i] = names.NewApplicationTag(appName)
			}

			cd.logger.Debugf(context.TODO(), "triggering asynchronous download of charms for the following applications: %v", strings.Join(changes, ", "))
			if err := cd.charmDownloaderAPI.DownloadApplicationCharms(ctx, appTags); err != nil {
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
	ctx, cancel := context.WithCancel(cd.catacomb.Context(context.Background()))
	return cd.catacomb.Context(ctx), cancel
}
