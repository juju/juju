// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitless

import (
	"context"

	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/catacomb"

	"github.com/juju/juju/core/logger"
	internalerrors "github.com/juju/juju/internal/errors"
)

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
