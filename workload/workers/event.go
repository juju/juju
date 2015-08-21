// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workers

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/util"
	"github.com/juju/juju/workload"
	"github.com/juju/juju/workload/context"
)

var logger = loggo.GetLogger("juju.workload.workers")

const (
	resEvents    = "events"
	resRunner    = "runner"
	resAPIClient = "apiclient"
)

// TODO(ericsnow) Switch handlers to...manifests? workers?

// TODO(ericsnow) Implement ManifoldConfig and Manifold() here?

// EventHandlers orchestrates handling of events on workloads.
type EventHandlers struct {
	events   chan []workload.Event
	handlers []func([]workload.Event, context.APIClient, Runner) error

	apiClient context.APIClient
	// runner is used in lieu of the engine due to support for stopping.
	runner worker.Runner
}

// NewEventHandlers creates a new EventHandlers.
func NewEventHandlers() *EventHandlers {
	logger.Debugf("new event handler created")
	var eh EventHandlers
	eh.init(nil)
	return &eh
}

func (eh *EventHandlers) init(apiClient context.APIClient) {
	eh.events = make(chan []workload.Event)
	eh.apiClient = apiClient
}

// Reset resets the event handlers.
func (eh *EventHandlers) Reset(apiClient context.APIClient) error {
	if err := eh.Close(); err != nil {
		return errors.Trace(err)
	}
	eh.init(apiClient)
	return nil
}

// Close cleans up the handler's resources.
func (eh *EventHandlers) Close() error {
	close(eh.events)
	if eh.runner != nil {
		eh.runner.Kill()
		if err := eh.runner.Wait(); err != nil {
			return errors.Trace(err)
		}
		eh.runner = nil
	}
	return nil
}

// RegisterHandler adds a handler to the list of handlers used when new
// events are processed.
func (eh *EventHandlers) RegisterHandler(handler func([]workload.Event, context.APIClient, Runner) error) {
	logger.Debugf("registering handler: %#v", handler)
	eh.handlers = append(eh.handlers, handler)
}

// AddEvents adds events to the list of events to be handled.
func (eh *EventHandlers) AddEvents(events ...workload.Event) {
	if len(events) == 0 {
		return
	}
	eh.events <- events
}

// StartEngine creates a new dependency engine and starts it.
func (eh *EventHandlers) StartEngine() (worker.Worker, error) {
	if eh.runner != nil {
		return nil, errors.Errorf("engine already started")
	}
	engine, err := newEngine()
	if err != nil {
		return nil, errors.Trace(err)
	}
	eh.runner = newRunner()

	manifolds := eh.manifolds()
	// TODO(ericsnow) Move the following to a helper in worker/dependency or worker/util?
	if err := dependency.Install(engine, manifolds); err != nil {
		if err := worker.Stop(engine); err != nil {
			logger.Errorf("while stopping engine with bad manifolds: %v", err)
		}
		eh.runner = nil
		return nil, errors.Trace(err)
	}
	return engine, nil
}

// manifolds returns the set of manifolds that should be added for the unit.
func (eh *EventHandlers) manifolds() dependency.Manifolds {
	manifolds := dependency.Manifolds{
		resEvents:    eh.eventsManifold(),
		resAPIClient: eh.apiManifold(),
	}
	if eh.runner != nil {
		manifolds[resRunner] = eh.runnerManifold()
	}
	return manifolds
}

func (eh *EventHandlers) eventsManifold() dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{},
		Start: func(dependency.GetResourceFunc) (worker.Worker, error) {
			// Pull all existing from State (via API) and add an event for each.
			events, err := InitialEvents(eh.apiClient)
			if err != nil {
				return nil, errors.Trace(err)
			}

			logger.Debugf("starting new worker")
			w := worker.NewSimpleWorker(eh.loop)
			// These must be added *after* the worker is started.
			eh.AddEvents(events...)
			return w, nil
		},
	}
}

func (eh *EventHandlers) runnerManifold() dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{},
		Start: func(dependency.GetResourceFunc) (worker.Worker, error) {
			return util.NewValueWorker(eh.runner)
		},
		Output: util.ValueWorkerOutput,
	}
}

func (eh *EventHandlers) apiManifold() dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{},
		Start: func(dependency.GetResourceFunc) (worker.Worker, error) {
			return util.NewValueWorker(eh.apiClient)
		},
		Output: util.ValueWorkerOutput,
	}
}

func (eh *EventHandlers) handle(events []workload.Event) error {
	logger.Debugf("handling %d events", len(events))
	for _, handleEvents := range eh.handlers {
		if err := handleEvents(events, eh.apiClient, eh.runner); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (eh *EventHandlers) loop(stopCh <-chan struct{}) error {
	done := false
	for !done {
		select {
		case <-stopCh:
			done = true
		case events, alive := <-eh.events:
			if !alive {
				done = true
			} else if err := eh.handle(events); err != nil {
				return errors.Trace(err)
			}
		}
	}
	// TODO(ericsnow) Call eh.Close() here?
	return nil
}

// InitialEvents returns the events that correspond to the current Juju state.
func InitialEvents(apiClient context.APIClient) ([]workload.Event, error) {
	// TODO(ericsnow) Use an API call that returns all of them at once,
	// rather than using a Get call for each?

	hctx, err := context.NewContextAPI(apiClient, nil)
	if err != nil {
		return nil, errors.Trace(err)
	}

	ids, err := hctx.List()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var events []workload.Event
	for _, id := range ids {
		info, err := hctx.Get(id)
		if err != nil {
			return nil, errors.Trace(err)
		}
		// TODO(wwitzel3) (Upgrade/Restart broken) during a restart of the
		// worker, the Plugin loses its absPath for the executable.
		plugin, err := hctx.Plugin(info)
		if err != nil {
			return nil, errors.Trace(err)
		}

		events = append(events, workload.Event{
			Kind:     workload.EventKindTracked,
			ID:       info.ID(),
			Plugin:   plugin,
			PluginID: info.Details.ID,
		})
	}
	return events, nil
}
