// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationflag

import (
	"github.com/juju/errors"
	"github.com/juju/utils"
	"github.com/juju/worker/v2/catacomb"

	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/watcher"
)

// ErrChanged indicates that a Worker has stopped because its
// Check result is no longer valid.
var ErrChanged = errors.New("migration flag value changed")

// Facade exposes controller functionality required by a Worker.
type Facade interface {
	Watch(uuid string) (watcher.NotifyWatcher, error)
	Phase(uuid string) (migration.Phase, error)
}

// Predicate defines a predicate.
type Predicate func(migration.Phase) bool

// IsTerminal returns true when the given phase means a migration has
// finished (successfully or otherwise).
func IsTerminal(phase migration.Phase) bool {
	return phase.IsTerminal()
}

// Config holds the dependencies and configuration for a Worker.
type Config struct {
	Facade Facade
	Model  string
	Check  Predicate
}

// Validate returns an error if the config cannot be expected to
// drive a functional Worker.
func (config Config) Validate() error {
	if config.Facade == nil {
		return errors.NotValidf("nil Facade")
	}
	if !utils.IsValidUUIDString(config.Model) {
		return errors.NotValidf("Model %q", config.Model)
	}
	if config.Check == nil {
		return errors.NotValidf("nil Check")
	}
	return nil
}

// New returns a Worker that tracks the result of the configured
// Check on the Model's migration phase, as exposed by the Facade.
func New(config Config) (*Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	phase, err := config.Facade.Phase(config.Model)
	if err != nil {
		return nil, errors.Trace(err)
	}

	w := &Worker{
		config: config,
		phase:  phase,
	}
	err = catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

// Worker implements worker.Worker and util.Flag, and exits
// with ErrChanged whenever the result of its configured Check of
// the Model's migration phase changes.
type Worker struct {
	catacomb catacomb.Catacomb
	config   Config
	phase    migration.Phase
}

// Kill is part of the worker.Worker interface.
func (w *Worker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *Worker) Wait() error {
	return w.catacomb.Wait()
}

// Check is part of the util.Flag interface.
func (w *Worker) Check() bool {
	return w.config.Check(w.phase)
}

func (w *Worker) loop() error {
	model := w.config.Model
	facade := w.config.Facade
	watcher, err := facade.Watch(model)
	if err != nil {
		return errors.Trace(err)
	}
	if err := w.catacomb.Add(watcher); err != nil {
		return errors.Trace(err)
	}
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case <-watcher.Changes():
			phase, err := facade.Phase(model)
			if err != nil {
				return errors.Trace(err)
			}
			if w.Check() != w.config.Check(phase) {
				return ErrChanged
			}
		}
	}
}
