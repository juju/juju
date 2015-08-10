// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workers

import (
	"github.com/juju/errors"

	"github.com/juju/juju/process"
	"github.com/juju/juju/process/context"
	"github.com/juju/juju/worker"
)

// TODO(ericsnow) Wrap runners so that handler-started workers are
// stopped when the EventHandlers is stopped/closed.

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
	runner    Runner
}

// NewEventHandlers wraps a new EventHandler around the provided channel.
func NewEventHandlers(apiClient context.APIClient, runner Runner) *EventHandlers {
	eh := &EventHandlers{
		events:    make(chan []process.Event), // TODO(ericsnow) Set a size?
		apiClient: apiClient,
		runner:    runner,
	}
	return eh
}

// Close cleans up the handler's resources.
func (eh *EventHandlers) Close() {
	close(eh.events)
}

// RegisterHandler adds a handler to the list of handlers used when new
// events are processed.
func (eh *EventHandlers) RegisterHandler(handler func([]process.Event, context.APIClient, Runner) error) {
	eh.handlers = append(eh.handlers, handler)
}

// AddEvents adds events to the list of events to be handled.
func (eh *EventHandlers) AddEvents(events ...process.Event) {
	eh.events <- events
}

// NewWorker wraps the EventHandler in a worker.
func (eh *EventHandlers) NewWorker() (worker.Worker, error) {
	return worker.NewSimpleWorker(eh.loop), nil
}

func (eh *EventHandlers) handle(events []process.Event) error {
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
