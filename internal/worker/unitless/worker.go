// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitless

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/catacomb"

	coreapplication "github.com/juju/juju/core/application"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	internalerrors "github.com/juju/juju/internal/errors"
	internalworker "github.com/juju/juju/internal/worker"
)

const runnerRestartDelay = 10 * time.Second

// Config holds the dependencies for the scriptlet worker.
type Config struct {
	// ScriptletService is used by the worker to
	// schedule scriptlets and dispatch events.
	ScriptletService ScriptletService

	// NewExecutor is a factory for creating scriptlet execution environments.
	NewExecutor ExecutorFactory

	// Clock supplies timing services to the worker runner.
	Clock clock.Clock

	// MaxAllocs limits the allocations that can be made
	// during a single scriptlet function execution.
	MaxAllocs int64

	// MaxSteps limits the execution steps that can be made
	// during a single scriptlet function execution.
	MaxSteps int64

	// Logger logs worker output.
	Logger logger.Logger
}

// Validate checks that the worker can be started.
func (cfg Config) Validate() error {
	if cfg.ScriptletService == nil {
		return internalerrors.New("nil ScriptletService not valid").Add(coreerrors.NotValid)
	}
	if cfg.NewExecutor == nil {
		return internalerrors.New("nil NewExecutor not valid").Add(coreerrors.NotValid)
	}
	if cfg.Clock == nil {
		return internalerrors.New("nil Clock not valid").Add(coreerrors.NotValid)
	}
	if cfg.MaxAllocs < 0 {
		return internalerrors.New("negative MaxAllocs not valid").Add(coreerrors.NotValid)
	}
	if cfg.MaxSteps < 0 {
		return internalerrors.New("negative MaxSteps not valid").Add(coreerrors.NotValid)
	}
	if cfg.Logger == nil {
		return internalerrors.New("nil Logger not valid").Add(coreerrors.NotValid)
	}
	return nil
}

type scriptletWorker struct {
	catacomb catacomb.Catacomb
	config   Config
	runner   *worker.Runner
}

// NewWorker returns a worker that dispatches events to scriptlet applications.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, internalerrors.Capture(err)
	}

	runner, err := worker.NewRunner(worker.RunnerParams{
		Name:          "scriptlet",
		IsFatal:       func(error) bool { return false },
		ShouldRestart: internalworker.ShouldRunnerRestart,
		RestartDelay:  runnerRestartDelay,
		Clock:         config.Clock,
		Logger:        internalworker.WrapLogger(config.Logger),
	})
	if err != nil {
		return nil, internalerrors.Capture(err)
	}

	w := &scriptletWorker{
		config: config,
		runner: runner,
	}
	if err := catacomb.Invoke(catacomb.Plan{
		Name: "scriptlet",
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{w.runner},
	}); err != nil {
		return nil, internalerrors.Capture(err)
	}
	return w, nil
}

func (w *scriptletWorker) loop() error {
	ctx := w.catacomb.Context(context.Background())
	log := w.config.Logger

	log.Infof(ctx, "starting scriptlet worker")

	appWatcher, err := w.config.ScriptletService.WatchScriptletApplications(ctx)
	if err != nil {
		return internalerrors.Errorf("watching scriptlet applications: %w", err)
	}
	if err := w.catacomb.Add(appWatcher); err != nil {
		return internalerrors.Capture(err)
	}

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case applicationUUIDs, ok := <-appWatcher.Changes():
			if !ok {
				return internalerrors.New("scriptlet application watcher closed")
			}
			if err := w.handleApplicationChanges(ctx, applicationUUIDs); err != nil {
				return internalerrors.Capture(err)
			}
		}
	}
}

func (w *scriptletWorker) handleApplicationChanges(ctx context.Context, applicationUUIDs []string) error {
	for _, applicationUUID := range applicationUUIDs {
		if applicationUUID == "" {
			w.config.Logger.Warningf(ctx, "ignoring empty scriptlet application UUID")
			continue
		}

		err := w.runner.StartWorker(ctx, applicationUUID, func(context.Context) (worker.Worker, error) {
			return newApplicationRunner(applicationRunnerConfig{
				ApplicationUUID:  coreapplication.UUID(applicationUUID),
				ScriptletService: w.config.ScriptletService,
				NewExecutor:      w.config.NewExecutor,
				MaxAllocs:        w.config.MaxAllocs,
				MaxSteps:         w.config.MaxSteps,
				Logger:           w.config.Logger,
			})
		})
		if internalerrors.Is(err, coreerrors.AlreadyExists) {
			continue
		}
		if err != nil {
			return internalerrors.Errorf("starting scriptlet application runner %q: %w", applicationUUID, err)
		}
	}
	return nil
}

// Report returns information about running application workers.
func (w *scriptletWorker) Report(ctx context.Context) map[string]any {
	return w.runner.Report(ctx)
}

// Kill is part of the worker.Worker interface.
func (w *scriptletWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *scriptletWorker) Wait() error {
	return w.catacomb.Wait()
}
