// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workers

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/set"

	"github.com/juju/juju/process"
	"github.com/juju/juju/process/context"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
)

var workloadEventLogger = loggo.GetLogger("juju.workload.workers.event")

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

// EventHandlers orchestrates handling of events on workload processes.
type EventHandlers struct {
	events   chan []process.Event
	handlers []func([]process.Event, context.APIClient, Runner) error

	apiClient context.APIClient
	runner    *trackingRunner
}

// NewEventHandlers wraps a new EventHandler around the provided channel.
func NewEventHandlers() *EventHandlers {
	workloadEventLogger.Debugf("new event handler created")
	eh := &EventHandlers{
		events: make(chan []process.Event),
	}
	return eh
}

func (eh *EventHandlers) Init(apiClient context.APIClient, runner Runner) {
	eh.apiClient = apiClient
	eh.runner = newTrackingRunner(runner)
}

// Close cleans up the handler's resources.
func (eh *EventHandlers) Close() error {
	close(eh.events)
	if err := eh.runner.stopAll(); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// RegisterHandler adds a handler to the list of handlers used when new
// events are processed.
func (eh *EventHandlers) RegisterHandler(handler func([]process.Event, context.APIClient, Runner) error) {
	workloadEventLogger.Debugf("registering handler: %#v", handler)
	eh.handlers = append(eh.handlers, handler)
}

// AddEvents adds events to the list of events to be handled.
func (eh *EventHandlers) AddEvents(events ...process.Event) {
	eh.events <- events
}

// Manifolds returns the set of manifolds that should be added for the unit.
func (eh *EventHandlers) Manifolds() dependency.Manifolds {
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

			workloadEventLogger.Debugf("starting new worker")
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

func (eh *EventHandlers) handle(events []process.Event) error {
	workloadEventLogger.Debugf("handling %d events", len(events))
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
func InitialEvents(apiClient context.APIClient) ([]process.Event, error) {
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

	var events []process.Event
	for _, id := range ids {
		proc, err := hctx.Get(id)
		if err != nil {
			return nil, errors.Trace(err)
		}
		// TODO(wwitzel3) (Upgrade/Restart broken) during a restart of the
		// worker, the Plugin loses its absPath for the executable.
		plugin, err := hctx.Plugin(proc)
		if err != nil {
			return nil, errors.Trace(err)
		}

		events = append(events, process.Event{
			Kind:     process.EventKindTracked,
			ID:       proc.ID(),
			Plugin:   plugin,
			PluginID: proc.Details.ID,
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
