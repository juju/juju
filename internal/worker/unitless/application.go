// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitless

import (
	"context"

	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/catacomb"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/logger"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	internalerrors "github.com/juju/juju/internal/errors"
)

type applicationRunnerConfig struct {
	ApplicationUUID  coreapplication.UUID
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
		Name: "scriptlet-" + config.ApplicationUUID.String(),
		Site: &r.catacomb,
		Work: r.loop,
	}); err != nil {
		return nil, internalerrors.Capture(err)
	}
	return r, nil
}

// Kill is part of the worker.Worker interface.
func (r *applicationRunner) Kill() {
	r.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (r *applicationRunner) Wait() error {
	return r.catacomb.Wait()
}

func (r *applicationRunner) loop() error {
	ctx := r.catacomb.Context(context.Background())
	log := r.config.Logger

	applicationUUID := r.config.ApplicationUUID

	log.Infof(ctx, "starting scriptlet application runner %q", applicationUUID)
	dyingWatcher, err := r.config.ScriptletService.WatchScriptletApplicationDying(ctx, applicationUUID)
	if internalerrors.Is(err, applicationerrors.ApplicationNotFound) {
		log.Infof(ctx, "scriptlet application %q not found, stopping runner", applicationUUID)
		return nil
	} else if err != nil {
		return internalerrors.Errorf("watching scriptlet application life %q: %w", applicationUUID, err)
	}
	if err := r.catacomb.Add(dyingWatcher); err != nil {
		return internalerrors.Capture(err)
	}

	// Wait for the subscription to be active before reading application life.
	select {
	case <-r.catacomb.Dying():
		return r.catacomb.ErrDying()
	case _, ok := <-dyingWatcher.Changes():
		if !ok {
			return internalerrors.New("scriptlet application dying watcher closed")
		}
	}

	application, err := r.config.ScriptletService.GetScriptletApplication(ctx, applicationUUID)
	if internalerrors.Is(err, applicationerrors.ApplicationNotFound) {
		// The application was removed before we could start the runner, so just
		// exit cleanly.
		log.Infof(ctx, "scriptlet application %q not found, stopping runner", applicationUUID)
		return nil
	} else if err != nil {
		return internalerrors.Errorf("getting scriptlet for application %q: %w", applicationUUID, err)
	}

	// If the application is no longer alive, exit cleanly.
	if life.IsNotAlive(application.Life) {
		log.Infof(ctx, "scriptlet application %q is no longer alive, stopping runner", applicationUUID)
		return nil
	}

	eventWatcher, err := r.config.ScriptletService.WatchApplicationEvents(ctx, applicationUUID)
	if internalerrors.Is(err, applicationerrors.ApplicationNotFound) {
		log.Infof(ctx, "scriptlet application %q was removed, stopping runner", applicationUUID)
		return nil
	} else if err != nil {
		return internalerrors.Errorf("watching scriptlet events for application %q: %w", applicationUUID, err)
	}
	if err := r.catacomb.Add(eventWatcher); err != nil {
		return internalerrors.Capture(err)
	}

	r.executor, err = r.config.NewExecutor(ctx, ExecutorConfig{
		Scriptlet: application,
		MaxAllocs: r.config.MaxAllocs,
		MaxSteps:  r.config.MaxSteps,
		Logger:    NewStarformLogAdapter(log),
	})
	if err != nil {
		return internalerrors.Errorf("creating scriptlet executor for application %q: %w", applicationUUID, err)
	}

	for {
		select {
		case <-r.catacomb.Dying():
			return r.catacomb.ErrDying()

		case _, ok := <-dyingWatcher.Changes():
			if !ok {
				return internalerrors.New("scriptlet application dying watcher closed")
			}
			application, err := r.config.ScriptletService.GetScriptletApplication(ctx, applicationUUID)
			if internalerrors.Is(err, applicationerrors.ApplicationNotFound) {
				// The application was removed before we saw the dying event, so
				// just exit cleanly.
				log.Infof(ctx, "scriptlet application %q was removed, stopping runner", applicationUUID)
				return nil
			} else if err != nil {
				return internalerrors.Errorf("getting scriptlet application life %q: %w", applicationUUID, err)
			}

			// If the application is no longer alive, exit cleanly.
			if life.IsNotAlive(application.Life) {
				log.Infof(ctx, "scriptlet application %q is no longer alive, stopping runner", applicationUUID)
				return nil
			}

		case eventNames, ok := <-eventWatcher.Changes():
			if !ok {
				return internalerrors.New("scriptlet event watcher closed")
			}
			for _, eventName := range eventNames {
				select {
				case <-r.catacomb.Dying():
					return r.catacomb.ErrDying()
				default:
				}
				if err := r.handleEvent(ctx, eventName); internalerrors.Is(err, applicationerrors.ApplicationNotFound) {
					log.Infof(ctx, "scriptlet application %q was removed, stopping runner", applicationUUID)
					return nil
				} else if err != nil {
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
