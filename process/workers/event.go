// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workers

import (
	"github.com/juju/errors"

	"github.com/juju/juju/process"
	"github.com/juju/juju/worker"
)

// EventHandler orchestrates handling of events on workload processes.
type EventHandler struct {
	events   chan []process.Event
	handlers []func([]process.Event) error
}

// NewEventHandler wraps a new EventHandler around the provided channel.
func NewEventHandler(events chan []process.Event) *EventHandler {
	eh := &EventHandler{
		events: events,
	}
	return eh
}

// RegisterHandler adds a handler to the list of handlers used when new
// events are processed.
func (eh *EventHandler) RegisterHandler(handler func([]process.Event) error) {
	eh.handlers = append(eh.handlers, handler)
}

// AddEvents adds events to the list of events to be handled.
func (eh *EventHandler) AddEvents(events ...process.Event) {
	eh.events <- events
}

// NewWorker wraps the EventHandler in a worker.
func (eh *EventHandler) NewWorker() (worker.Worker, error) {
	return worker.NewSimpleWorker(eh.loop), nil
}

func (eh *EventHandler) handle(events []process.Event) error {
	for _, handleEvents := range eh.handlers {
		if err := handleEvents(events); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (eh *EventHandler) loop(stopCh <-chan struct{}) error {
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
