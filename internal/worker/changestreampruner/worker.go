// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package changestreampruner

import (
	"context"
	"sync"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	internalworker "github.com/juju/juju/internal/worker"
)

const (
	// defaultPruneMinInterval is the default interval at which the
	// pruner will run.
	defaultPruneInterval = time.Second * 10

	// defaultWindowDuration is the default duration of the window in which
	// the pruner will select the lower bound of the watermark. If any
	// watermarks are outside of this window, they will not be selected and the
	// pruner will discard those watermarks.
	defaultWindowDuration = time.Minute * 10
)

// DBGetter describes the ability to supply a sql.DB
// reference for a particular database.
type DBGetter = coredatabase.DBGetter

// WorkerConfig encapsulates the configuration options for the
// changestream worker.
type WorkerConfig struct {
	DBGetter       DBGetter
	Clock          clock.Clock
	Logger         logger.Logger
	NewModelPruner NewModelPrunerFunc
}

// Validate ensures that the config values are valid.
func (c *WorkerConfig) Validate() error {
	if c.DBGetter == nil {
		return errors.NotValidf("missing DBGetter")
	}
	if c.NewModelPruner == nil {
		return errors.NotValidf("missing NewModelPruner")
	}
	if c.Clock == nil {
		return errors.NotValidf("missing clock")
	}
	if c.Logger == nil {
		return errors.NotValidf("missing logger")
	}
	return nil
}

// Pruner defines a worker that will truncate the change log.
type Pruner struct {
	catacomb catacomb.Catacomb

	cfg WorkerConfig

	runner *worker.Runner

	// windows holds the last window for each namespace. This is used to
	// determine if the change stream is keeping up with the pruner. If the
	// watermark is outside of the window, we should log a warning message.
	windows map[string]Window
	mutex   sync.Mutex
}

// NewWorker creates a new Pruner.
func NewWorker(cfg WorkerConfig) (worker.Worker, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	runner, err := worker.NewRunner(worker.RunnerParams{
		Name:          "changestream-pruner",
		IsFatal:       func(err error) bool { return false },
		ShouldRestart: func(err error) bool { return false },
		Clock:         cfg.Clock,
		Logger:        internalworker.WrapLogger(cfg.Logger),
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	w := &Pruner{
		cfg:     cfg,
		runner:  runner,
		windows: make(map[string]Window),
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Name: "changestream-pruner",
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{runner},
	}); err != nil {
		return nil, errors.Trace(err)
	}

	return w, nil
}

// Kill is part of the worker.Worker interface.
func (w *Pruner) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *Pruner) Wait() error {
	return w.catacomb.Wait()
}

// Report returns a map of internal state for debugging purposes.
func (w *Pruner) Report() map[string]any {
	return w.runner.Report()
}

func (w *Pruner) loop() error {
	ctx := w.catacomb.Context(context.Background())

	timer := w.cfg.Clock.NewTimer(defaultPruneInterval)
	defer timer.Stop()

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()

		case <-timer.Chan():
			// Attempt to prune, if there is any critical error, kill the
			// worker, which should force a restart.
			if err := w.prune(ctx); err != nil {
				return errors.Trace(err)
			}
			timer.Reset(defaultPruneInterval)
		}
	}
}

func (w *Pruner) prune(ctx context.Context) error {
	modelNamespaces, err := w.getModelNamespaces(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	traceEnabled := w.cfg.Logger.IsLevelEnabled(logger.TRACE)
	if traceEnabled {
		w.cfg.Logger.Tracef(ctx, "pruning model namespaces: %v", modelNamespaces)
	}

	for _, mn := range modelNamespaces {
		err := w.runner.StartWorker(ctx, mn.Namespace, func(ctx context.Context) (worker.Worker, error) {
			db, err := w.cfg.DBGetter.GetDB(ctx, mn.Namespace)
			if err != nil {
				return nil, errors.Trace(err)
			}

			return w.cfg.NewModelPruner(
				db,
				&namespaceWindow{
					namespace: mn.Namespace,
					pruner:    w,
				},
				w.cfg.Clock,
				w.cfg.Logger.Child(mn.Namespace),
			), nil
		})
		if err != nil && !errors.Is(err, errors.AlreadyExists) {
			return errors.Trace(err)
		}
	}

	return nil
}

func (w *Pruner) getModelNamespaces(ctx context.Context) ([]ModelNamespace, error) {
	db, err := w.cfg.DBGetter.GetDB(ctx, coredatabase.ControllerNS)
	if err != nil {
		return nil, errors.Trace(err)
	}

	query, err := sqlair.Prepare(`
SELECT namespace AS &ModelNamespace.namespace
FROM model_namespace;
	`, ModelNamespace{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	var modelNamespaces []ModelNamespace
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return errors.Trace(tx.Query(ctx, query).GetAll(&modelNamespaces))
	})
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Trace(err)
	}

	// To ensure we always prune the change log for the controller, we add it
	// to the list of models to prune.
	modelNamespaces = append([]ModelNamespace{
		{Namespace: coredatabase.ControllerNS},
	}, modelNamespaces...)

	return modelNamespaces, nil
}

type namespaceWindow struct {
	namespace string
	pruner    *Pruner
}

// CurrentWindow returns the current window for the namespace.
func (w *namespaceWindow) CurrentWindow() Window {
	w.pruner.mutex.Lock()
	defer w.pruner.mutex.Unlock()
	return w.pruner.windows[w.namespace]
}

// UpdateWindow updates the current window for the namespace.
func (w *namespaceWindow) UpdateWindow(newWindow Window) {
	w.pruner.mutex.Lock()
	defer w.pruner.mutex.Unlock()
	w.pruner.windows[w.namespace] = newWindow
}

// Namespace returns the namespace for this window.
func (w *namespaceWindow) Namespace() string {
	return w.namespace
}
