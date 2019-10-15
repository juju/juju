// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradedatabase

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/tomb.v2"
)

// Config is the configuration needed to construct an upgradeDB worker.
type Config struct {
	// Tag is the current machine tag.
	Tag names.Tag

	// Logger is the logger for this worker.
	Logger Logger

	// Open state is a function pointer for returning a state pool indirection.
	OpenState func() (Pool, error)
}

// Validate returns an error if the worker config is not valid.
func (cfg Config) Validate() error {
	if cfg.Tag == nil {
		return errors.NotValidf("nil machine tag")
	}
	k := cfg.Tag.Kind()
	if k != names.MachineTagKind {
		return errors.NotValidf("%q tag kind", k)
	}
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if cfg.OpenState == nil {
		return errors.NotValidf("nil OpenState function")
	}
	return nil
}

// upgradeDB is a worker that will run on a controller machine.
// It is responsible for running upgrade steps of type `DatabaseMaster` on the
// primary MongoDB instance.
type upgradeDB struct {
	tomb tomb.Tomb

	tag    names.Tag
	logger Logger
	pool   Pool
}

// NewWorker validates the input configuration, then uses it to create,
// start and return an upgradeDB worker.
func NewWorker(cfg Config) (worker.Worker, error) {
	var err error

	if err = cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &upgradeDB{
		tag:    cfg.Tag,
		logger: cfg.Logger,
	}
	if w.pool, err = cfg.OpenState(); err != nil {
		return nil, err
	}

	w.tomb.Go(w.run)
	return w, nil
}

func (w *upgradeDB) run() error {
	defer func() { _ = w.pool.Close() }()

	// If this controller is not running the Mongo primary,
	// we have no work to do and can just exit cleanly.
	isPrimary, err := w.pool.IsPrimary(w.tag.Id())
	if err != nil {
		return errors.Trace(err)
	}
	if !isPrimary {
		w.logger.Debugf("not running the Mongo primary; exiting")
		return nil
	}

	return nil
}

// Kill is part of the worker.Worker interface.
func (w *upgradeDB) Kill() {
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *upgradeDB) Wait() error {
	return w.tomb.Wait()
}
