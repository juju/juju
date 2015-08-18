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
)

var workloadEventLogger = loggo.GetLogger("juju.workload.workers.event")

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

// NewWorker wraps the EventHandler in a worker.
func (eh *EventHandlers) NewWorker() (worker.Worker, error) {
	workloadEventLogger.Debugf("starting new worker")
	return worker.NewSimpleWorker(eh.loop), nil
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
