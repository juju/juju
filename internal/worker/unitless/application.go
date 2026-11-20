// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitless

import (
	"context"

	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/catacomb"

	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/watcher"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	domainunitless "github.com/juju/juju/domain/unitless"
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

	charmUUID    corecharm.ID
	eventWatcher watcher.StringsWatcher
	executor     Executor
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
	appWatcher, err := r.config.ScriptletService.WatchScriptletApplication(ctx, applicationUUID)
	if err != nil {
		return internalerrors.Errorf("watching scriptlet application %q: %w", applicationUUID, err)
	}
	if err := r.catacomb.Add(appWatcher); err != nil {
		return internalerrors.Capture(err)
	}

	// Wait for the subscription to be active before reading application state.
	select {
	case <-r.catacomb.Dying():
		return r.catacomb.ErrDying()
	case _, ok := <-appWatcher.Changes():
		if !ok {
			return internalerrors.New("scriptlet application watcher closed")
		}
	}

	application, stop, err := r.getScriptletApplication(ctx)
	if err != nil {
		return internalerrors.Capture(err)
	}
	if stop {
		return nil
	}

	if err := r.startScriptlet(ctx, application); err != nil {
		return internalerrors.Capture(err)
	}

	for {
		select {
		case <-r.catacomb.Dying():
			return r.catacomb.ErrDying()

		case _, ok := <-appWatcher.Changes():
			if !ok {
				return internalerrors.New("scriptlet application watcher closed")
			}

			application, stop, err := r.getScriptletApplication(ctx)
			if err != nil {
				return internalerrors.Capture(err)
			}
			if stop {
				return nil
			}
			if application.CharmUUID == r.charmUUID {
				continue
			}

			log.Infof(ctx, "refreshing scriptlet application %q", applicationUUID)
			if err := worker.Stop(r.eventWatcher); err != nil {
				return internalerrors.Errorf("stopping scriptlet events for application %q: %w", applicationUUID, err)
			}
			if err := r.startScriptlet(ctx, application); err != nil {
				return internalerrors.Capture(err)
			}

		case eventNames, ok := <-r.eventWatcher.Changes():
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

// getScriptletApplication returns the current scriptlet application and whether
// the runner should stop because it was removed, became a normal charm, or is
// no longer alive.
func (r *applicationRunner) getScriptletApplication(
	ctx context.Context,
) (domainunitless.ScriptletApplication, bool, error) {
	applicationUUID := r.config.ApplicationUUID
	log := r.config.Logger

	application, err := r.config.ScriptletService.GetScriptletApplication(ctx, applicationUUID)
	if internalerrors.Is(err, applicationerrors.ApplicationNotFound) {
		// The application was removed or its charm no longer has scriptlets,
		// so stop executing it.
		log.Infof(ctx, "scriptlet application %q not found, stopping runner", applicationUUID)
		return domainunitless.ScriptletApplication{}, true, nil
	} else if err != nil {
		return domainunitless.ScriptletApplication{}, false,
			internalerrors.Errorf("getting scriptlet application %q: %w", applicationUUID, err)
	}

	if life.IsNotAlive(application.Life) {
		log.Infof(ctx, "scriptlet application %q is no longer alive, stopping runner", applicationUUID)
		return application, true, nil
	}
	return application, false, nil
}

// startScriptlet creates a fresh executor and event watcher. Restarting the
// watcher dispatches the initial config event to the refreshed charm.
func (r *applicationRunner) startScriptlet(
	ctx context.Context, application domainunitless.ScriptletApplication,
) error {
	applicationUUID := r.config.ApplicationUUID
	eventWatcher, err := r.config.ScriptletService.WatchApplicationEvents(ctx, applicationUUID)
	if err != nil {
		return internalerrors.Errorf("watching scriptlet events for application %q: %w", applicationUUID, err)
	}

	if err := r.catacomb.Add(eventWatcher); err != nil {
		return internalerrors.Capture(err)
	}

	executor, err := r.config.NewExecutor(ctx, ExecutorConfig{
		Scriptlet: application,
		MaxAllocs: r.config.MaxAllocs,
		MaxSteps:  r.config.MaxSteps,
		Logger:    NewStarformLogAdapter(r.config.Logger),
	})
	if err != nil {
		return internalerrors.Errorf("creating scriptlet executor for application %q: %w", applicationUUID, err)
	}
	r.charmUUID = application.CharmUUID
	r.eventWatcher = eventWatcher
	r.executor = executor
	return nil
}

func (r *applicationRunner) handleEvent(ctx context.Context, eventName string) error {
	if eventName == "" {
		r.config.Logger.Warningf(ctx, "ignoring empty scriptlet event for application %q", r.config.ApplicationUUID)
		return nil
	}

	event, err := r.config.ScriptletService.GetScriptletEvent(
		ctx, r.config.ApplicationUUID, domainunitless.EventType(eventName))
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
