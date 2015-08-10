// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workers

import (
	"github.com/juju/errors"

	"github.com/juju/juju/process"
	"github.com/juju/juju/worker"
)

// EventHandlers orchestrates handling of events on workload processes.
type EventHandlers struct {
	events   chan []process.Event
	handlers []func([]process.Event) error
}

// NewEventHandlers wraps a new EventHandler around the provided channel.
func NewEventHandlers() *EventHandlers {
	eh := &EventHandlers{
		events: make(chan []process.Event), // TODO(ericsnow) Set a size?
	}
	return eh
}

// Close cleans up the handler's resources.
func (eh *EventHandlers) Close() {
	close(eh.events)
}

// RegisterHandler adds a handler to the list of handlers used when new
// events are processed.
func (eh *EventHandlers) RegisterHandler(handler func([]process.Event) error) {
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
		if err := handleEvents(events); err != nil {
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
	return nil
}
