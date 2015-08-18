// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workers

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/set"

	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/workload"
	"github.com/juju/juju/workload/context"
)

var logger = loggo.GetLogger("juju.workload.workers")

const (
	resEvents    = "events"
	resRunner    = "runner"
	resAPIClient = "apiclient"
)

// Runner is the portion of worker.Worker needed for Event handlers.
type Runner interface {
	// Start a worker using the provided func.
	StartWorker(id string, newWorker func() (worker.Worker, error)) error
	// Stop the identified worker.
	StopWorker(id string) error
}

// TODO(ericsnow) Switch handlers to...manifests? workers?

// EventHandlers orchestrates handling of events on workloads.
type EventHandlers struct {
	events   chan []workload.Event
	handlers []func([]workload.Event, context.APIClient, Runner) error

	apiClient context.APIClient
	runner    *trackingRunner
}

// NewEventHandlers wraps a new EventHandler around the provided channel.
func NewEventHandlers() *EventHandlers {
	logger.Debugf("new event handler created")
	eh := &EventHandlers{
		events: make(chan []workload.Event),
	}
	return eh
}

// Reset resets the event handlers.
func (eh *EventHandlers) Reset(apiClient context.APIClient) {
	close(eh.events)
	eh.events = make(chan []workload.Event)

	eh.apiClient = apiClient
	eh.runner = nil
}

// Close cleans up the handler's resources.
func (eh *EventHandlers) Close() error {
	close(eh.events)
	if eh.runner != nil {
		if err := eh.runner.stopAll(); err != nil {
			return errors.Trace(err)
		}
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
	var runner worker.Runner // TODO(ericsnow) Wrap engine in a runner.
	eh.runner = newTrackingRunner(runner)

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
	return dependency.Manifolds{
		resEvents:    eh.eventsManifold(),
		resRunner:    eh.runnerManifold(),
		resAPIClient: eh.apiManifold(),
	}
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

// TODO(ericsnow) Use worker.util.ValueWorker in runnerManifold and apiManifold.

func (eh *EventHandlers) runnerManifold() dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{},
		Start: func(dependency.GetResourceFunc) (worker.Worker, error) {
			loop := func(<-chan struct{}) error { return nil }
			return worker.NewSimpleWorker(loop), nil
		},
		Output: func(in worker.Worker, out interface{}) error {
			// TODO(ericsnow) provide the runner
			return nil
		},
	}
}

func (eh *EventHandlers) apiManifold() dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{},
		Start: func(dependency.GetResourceFunc) (worker.Worker, error) {
			loop := func(<-chan struct{}) error { return nil }
			return worker.NewSimpleWorker(loop), nil
		},
		Output: func(in worker.Worker, out interface{}) error {
			// TODO(ericsnow) provide the API client
			return nil
		},
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

type trackingRunner struct {
	Runner
	running set.Strings
}

func newTrackingRunner(runner Runner) *trackingRunner {
	return &trackingRunner{
		Runner:  runner,
		running: set.NewStrings(),
	}
}

// StartWorker implements Runner.
func (r *trackingRunner) Startworker(id string, newWorker func() (worker.Worker, error)) error {
	if err := r.Runner.StartWorker(id, newWorker); err != nil {
		return errors.Trace(err)
	}
	r.running.Add(id)
	return nil
}

// StopWorker implements Runner.
func (r *trackingRunner) StopWorker(id string) error {
	if err := r.Runner.StopWorker(id); err != nil {
		return errors.Trace(err)
	}
	r.running.Remove(id) // TODO(ericsnow) Move above StopWorker?
	return nil

}

func (r *trackingRunner) stopAll() error {
	for _, id := range r.running.Values() {
		if err := r.StopWorker(id); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}
