// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretspruner

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/watcher"
)

// SecretsFacade instances provide a set of API for the worker to deal with secret prune.
type SecretsFacade interface {
	WatchRevisionsToPrune(context.Context) (watcher.NotifyWatcher, error)
	DeleteObsoleteUserSecretRevisions(context.Context) error
}

// Config defines the operation of the Worker.
type Config struct {
	SecretsFacade
	Logger logger.Logger
}

// Validate returns an error if config cannot drive the Worker.
func (config Config) Validate() error {
	if config.SecretsFacade == nil {
		return errors.NotValidf("nil SecretsFacade")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

// NewWorker returns a secretspruner Worker backed by config, or an error.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &Worker{config: config}
	err := catacomb.Invoke(catacomb.Plan{
		Name: "secrets-pruner",
		Site: &w.catacomb,
		Work: w.loop,
	})
	return w, errors.Trace(err)
}

// Worker prunes the user supplied secret revisions.
type Worker struct {
	catacomb catacomb.Catacomb
	config   Config
}

// Kill is defined on worker.Worker.
func (w *Worker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *Worker) Wait() error {
	return w.catacomb.Wait()
}

func (w *Worker) processChanges(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	return w.config.SecretsFacade.DeleteObsoleteUserSecretRevisions(ctx)
}

func (w *Worker) loop() (err error) {
	ctx, cancel := w.scopeContext()
	defer cancel()

	watcher, err := w.config.SecretsFacade.WatchRevisionsToPrune(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	if err := w.catacomb.Add(watcher); err != nil {
		return errors.Trace(err)
	}

	for {
		select {
		case <-w.catacomb.Dying():
			return errors.Trace(w.catacomb.ErrDying())
		// TODO: watch for secret's auto-prune config changes.
		// then delete any obsolete revisions.
		case _, ok := <-watcher.Changes():
			if !ok {
				return errors.New("secret prune changed watch closed")
			}
			w.config.Logger.Debugf(ctx, "maybe have user secret revisions to prune")
			if err := w.processChanges(ctx); err != nil {
				return errors.Trace(err)
			}
		}
	}
}

func (w *Worker) scopeContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(w.catacomb.Context(context.Background()))
}
