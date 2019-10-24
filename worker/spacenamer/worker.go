// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spacenamer

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v3"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/catacomb"

	"github.com/juju/juju/core/watcher"
)

// SpaceNamerAPI represents the API calls the logger makes.
type SpaceNamerAPI interface {
	SetDefaultSpaceName() error
	WatchDefaultSpaceConfig() (watcher.NotifyWatcher, error)
}

// WorkerConfig contains the information required for the Logger worker
// to operate.
type WorkerConfig struct {
	API    SpaceNamerAPI
	Tag    names.Tag
	Logger Logger
}

// Validate ensures all the necessary fields have values.
func (c *WorkerConfig) Validate() error {
	if c.API == nil {
		return errors.NotValidf("missing api")
	}
	if c.Logger == nil {
		return errors.NotValidf("missing logger")
	}
	return nil
}

// spaceNamerWorker is responsible for updating the default space name
// model config watcher tells the agent that the value has changed.
type spaceNamerWorker struct {
	SpaceNamerAPI
	catacomb catacomb.Catacomb
	logger   Logger
}

func NewWorker(config WorkerConfig) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	w := &spaceNamerWorker{
		SpaceNamerAPI: config.API,
		logger:        config.Logger,
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
	}); err != nil {
		return nil, errors.Trace(err)
	}

	return w, nil
}

func (w *spaceNamerWorker) loop() error {
	uw, err := w.WatchDefaultSpaceConfig()
	if err != nil {
		return errors.Trace(err)
	}
	err = w.catacomb.Add(uw)
	if err != nil {
		return errors.Trace(err)
	}
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case <-uw.Changes():
			if err := w.SetDefaultSpaceName(); err != nil {
				// log the error, but not a good reason to restart the worker.
				w.logger.Errorf("Received error setting Default Space Name: %s", err.Error())
			}
		}
	}
}

// Kill implements worker.Worker.Kill.
func (w *spaceNamerWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait implements worker.Worker.Wait.
func (w *spaceNamerWorker) Wait() error {
	return w.catacomb.Wait()
}

// Stop stops the upgrade-series worker and returns any
// error it encountered when running.
func (w *spaceNamerWorker) Stop() error {
	w.Kill()
	return w.Wait()
}
