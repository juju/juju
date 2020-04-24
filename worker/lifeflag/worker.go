// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lifeflag

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/worker/v2/catacomb"

	"github.com/juju/juju/api/lifeflag"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/watcher"
)

// Facade exposes capabilities required by the worker.
type Facade interface {
	Watch(names.Tag) (watcher.NotifyWatcher, error)
	Life(names.Tag) (life.Value, error)
}

// Config holds the configuration and dependencies for a worker.
type Config struct {
	Facade Facade
	Entity names.Tag
	Result life.Predicate
}

// Validate returns an error if the config cannot be expected
// to drive a functional worker.
func (config Config) Validate() error {
	if config.Facade == nil {
		return errors.NotValidf("nil Facade")
	}
	if config.Entity == nil {
		return errors.NotValidf("nil Entity")
	}
	if config.Result == nil {
		return errors.NotValidf("nil Result")
	}
	return nil
}

var (
	// ErrNotFound indicates that the worker cannot run because
	// the configured entity does not exist.
	ErrNotFound = errors.New("entity not found")

	// ErrValueChanged indicates that the result of Check is
	// outdated, and the worker should be restarted.
	ErrValueChanged = errors.New("flag value changed")
)

// filter is used to wrap errors that might have come from the api,
// so that we can return an error appropriate to our level. Was
// tempted to make it a manifold-level thing, but that'd be even
// worse (because the worker should not be emitting api errors for
// conditions it knows about, full stop).
func filter(err error) error {
	if cause := errors.Cause(err); cause == lifeflag.ErrNotFound {
		return ErrNotFound
	}
	return err
}

// New returns a worker that exposes the result of the configured
// predicate when applied to the configured entity's life value,
// and fails with ErrValueChanged when the result changes.
func New(config Config) (*Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	// Read it before the worker starts, so that we have a value
	// guaranteed before we return the worker. Because we read this
	// before we start the internal watcher, we'll need an additional
	// read triggered by the first change event; this will *probably*
	// be the same value, but we can't assume it.
	life, err := config.Facade.Life(config.Entity)
	if err != nil {
		return nil, filter(errors.Trace(err))
	}

	w := &Worker{
		config: config,
		life:   life,
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

// Worker holds the result of some predicate regarding an entity's life,
// and fails with ErrValueChanged when the result of the predicate changes.
type Worker struct {
	catacomb catacomb.Catacomb
	config   Config
	life     life.Value
}

// Kill is part of the worker.Worker interface.
func (w *Worker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *Worker) Wait() error {
	return filter(w.catacomb.Wait())
}

// Check is part of the util.Flag interface.
func (w *Worker) Check() bool {
	return w.config.Result(w.life)
}

func (w *Worker) loop() error {
	watcher, err := w.config.Facade.Watch(w.config.Entity)
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
			life, err := w.config.Facade.Life(w.config.Entity)
			if err != nil {
				return errors.Trace(err)
			}
			if w.config.Result(life) != w.Check() {
				return ErrValueChanged
			}
		}
	}
}
