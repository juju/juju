// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitless

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/catacomb"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	internalerrors "github.com/juju/juju/internal/errors"
	internalworker "github.com/juju/juju/internal/worker"
)

const runnerRestartDelay = 10 * time.Second

// Config holds the dependencies for the scriptlet worker.
type Config struct {
	ScriptletService ScriptletService
	NewExecutor      ExecutorFactory
	MaxAllocs        int64
	MaxSteps         int64
	Logger           logger.Logger
}

// Validate checks that the worker can be started.
func (config Config) Validate() error {
	if config.ScriptletService == nil {
		return internalerrors.New("nil ScriptletService not valid").Add(coreerrors.NotValid)
	}
	if config.Logger == nil {
		return internalerrors.New("nil Logger not valid").Add(coreerrors.NotValid)
	}
	if config.MaxAllocs < 0 {
		return internalerrors.New("negative MaxAllocs not valid").Add(coreerrors.NotValid)
	}
	if config.MaxSteps < 0 {
		return internalerrors.New("negative MaxSteps not valid").Add(coreerrors.NotValid)
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

	// TODO (manadart 2026-06-12): Remove this fallback, validate it with incoming config.
	// Close over it with the steps and allocs numbers to we don' have to do a fallback
	// for those in NewStarformExecutor.
	if config.NewExecutor == nil {
		config.NewExecutor = NewStarformExecutor
	}

	runner, err := worker.NewRunner(worker.RunnerParams{
		Name:          "scriptlet",
		IsFatal:       func(error) bool { return false },
		ShouldRestart: internalworker.ShouldRunnerRestart,
		RestartDelay:  runnerRestartDelay,
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
				ApplicationUUID:  applicationUUID,
				ScriptletService: w.config.ScriptletService,
				NewExecutor:      w.config.NewExecutor,
				MaxAllocs:        w.config.MaxAllocs,
				MaxSteps:         w.config.MaxSteps,
				Logger:           w.config.Logger,
			})
		})
		if errors.Is(err, errors.AlreadyExists) {
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

type applicationRunnerConfig struct {
	ApplicationUUID  string
	ScriptletService ScriptletService
	NewExecutor      ExecutorFactory
	MaxAllocs        int64
	MaxSteps         int64
	Logger           logger.Logger
}

type applicationRunner struct {
	catacomb catacomb.Catacomb
	config   applicationRunnerConfig
	executor Executor
}

func newApplicationRunner(config applicationRunnerConfig) (worker.Worker, error) {
	r := &applicationRunner{
		config: config,
	}
	if err := catacomb.Invoke(catacomb.Plan{
		Name: "scriptlet-" + config.ApplicationUUID,
		Site: &r.catacomb,
		Work: r.loop,
	}); err != nil {
		return nil, internalerrors.Capture(err)
	}
	return r, nil
}

func (r *applicationRunner) loop() error {
	ctx := r.catacomb.Context(context.Background())
	log := r.config.Logger
	applicationUUID := r.config.ApplicationUUID

	log.Infof(ctx, "starting scriptlet application runner %q", applicationUUID)
	scriptlet, err := r.config.ScriptletService.GetApplicationScriptlet(ctx, applicationUUID)
	if err != nil {
		return internalerrors.Errorf("getting scriptlet for application %q: %w", applicationUUID, err)
	}

	executor, err := r.config.NewExecutor(ctx, ExecutorConfig{
		Scriptlet: scriptlet,
		MaxAllocs: r.config.MaxAllocs,
		MaxSteps:  r.config.MaxSteps,
		Logger:    NewStarformLogAdapter(log),
	})
	if err != nil {
		return internalerrors.Errorf("creating scriptlet executor for application %q: %w", applicationUUID, err)
	}
	r.executor = executor

	eventWatcher, err := r.config.ScriptletService.WatchApplicationEvents(ctx, applicationUUID)
	if err != nil {
		return internalerrors.Errorf("watching scriptlet events for application %q: %w", applicationUUID, err)
	}
	if err := r.catacomb.Add(eventWatcher); err != nil {
		return internalerrors.Capture(err)
	}

	for {
		select {
		case <-r.catacomb.Dying():
			return r.catacomb.ErrDying()
		case eventNames, ok := <-eventWatcher.Changes():
			if !ok {
				return internalerrors.New("scriptlet event watcher closed")
			}
			for _, eventName := range eventNames {
				if err := r.handleEvent(ctx, eventName); err != nil {
					return internalerrors.Errorf(
						"handling scriptlet event %q for application %q: %w",
						eventName, applicationUUID, err,
					)
				}
			}
		}
	}
}

func (r *applicationRunner) handleEvent(ctx context.Context, eventName string) error {
	if eventName == "" {
		r.config.Logger.Warningf(ctx, "ignoring empty scriptlet event for application %q", r.config.ApplicationUUID)
		return nil
	}

	event, err := r.config.ScriptletService.GetScriptletEvent(ctx, r.config.ApplicationUUID, eventName)
	if err != nil {
		return internalerrors.Errorf("getting event snapshot: %w", err)
	}

	intents, err := r.executor.Handle(ctx, event)
	if err != nil {
		r.config.Logger.Errorf(
			ctx, "executing scriptlet event %q for application %q: %v", event.Name, r.config.ApplicationUUID, err)
		return nil
	}
	for _, intent := range intents {
		r.config.Logger.Infof(
			ctx, "scriptlet application %q event %q intent: %s", r.config.ApplicationUUID, event.Name, intent.Type)
	}
	return nil
}

// Kill is part of the worker.Worker interface.
func (r *applicationRunner) Kill() {
	r.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (r *applicationRunner) Wait() error {
	return r.catacomb.Wait()
}
