// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lifeflag

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v4/catacomb"

	apilifeflag "github.com/juju/juju/api/common/lifeflag"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/watcher"
)

// Facade exposes capabilities required by the worker.
type Facade interface {
	Watch(context.Context, names.Tag) (watcher.NotifyWatcher, error)
	Life(context.Context, names.Tag) (life.Value, error)
}

// Config holds the configuration and dependencies for a worker.
type Config struct {
	Facade         Facade
	Entity         names.Tag
	Result         life.Predicate
	NotFoundIsDead bool
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

const (
	// ErrNotFound indicates that the worker cannot run because
	// the configured entity does not exist.
	ErrNotFound = apilifeflag.ErrEntityNotFound

	// ErrValueChanged indicates that the result of Check is
	// outdated, and the worker should be restarted.
	ErrValueChanged = errors.ConstError("flag value changed")
)

// New returns a worker that exposes the result of the configured
// predicate when applied to the configured entity's life value,
// and fails with ErrValueChanged when the result changes.
func New(ctx context.Context, config Config) (*Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &Worker{
		config: config,
	}
	plan := catacomb.Plan{
		Name: "life-flag",
		Site: &w.catacomb,
		Work: w.loop,
	}

	var err error
	// Read it before the worker starts, so that we have a value
	// guaranteed before we return the worker. Because we read this
	// before we start the internal watcher, we'll need an additional
	// read triggered by the first change event; this will *probably*
	// be the same value, but we can't assume it.
	w.life, err = config.Facade.Life(ctx, config.Entity)
	if config.NotFoundIsDead && errors.Is(err, ErrNotFound) {
		// If we handle notfound as dead, we will always be dead.
		w.life = life.Dead
		plan.Work = w.alwaysDead
	} else if err != nil {
		return nil, errors.Trace(err)
	}

	err = catacomb.Invoke(plan)
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
	return w.catacomb.Wait()
}

// Check is part of the util.Flag interface.
func (w *Worker) Check() bool {
	return w.config.Result(w.life)
}

func (w *Worker) alwaysDead() error {
	if w.config.Result(life.Dead) != w.Check() {
		return ErrValueChanged
	}
	<-w.catacomb.Dying()
	return w.catacomb.ErrDying()
}

func (w *Worker) loop() error {
	ctx, cancel := w.scopedContext()
	defer cancel()

	watcher, err := w.config.Facade.Watch(ctx, w.config.Entity)
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
			l, err := w.config.Facade.Life(ctx, w.config.Entity)
			if w.config.NotFoundIsDead && errors.Is(err, ErrNotFound) {
				l = life.Dead
			} else if err != nil {
				return errors.Trace(err)
			}
			if w.config.Result(l) != w.Check() {
				return ErrValueChanged
			}
		}
	}
}

func (w *Worker) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(w.catacomb.Context(context.Background()))
}
