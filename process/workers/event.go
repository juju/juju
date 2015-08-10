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

// AddEvents adds events to the list of events to be handled.
func (eh *EventHandler) AddEvents(events ...process.Event) {
	eh.events <- events
}

// NewWorker wraps the EventHandler in a worker.
func (eh *EventHandler) NewWorker() (worker.Worker, error) {
	return worker.NewSimpleWorker(eh.loop), nil
}

func (eh *EventHandler) next() (bool, error) {
	events, closed := <-eh.events
	if closed {
		return true, nil
	}

	for _, handleEvents := range eh.handlers {
		if err := handleEvents(events); err != nil {
			return false, errors.Trace(err)
		}
	}
	return false, nil
}

func (eh *EventHandler) loop(stopCh <-chan struct{}) error {
	for {
		select {
		case <-stopCh:
			break
		default:
		}
		done, err := eh.next()
		if err != nil {
			return errors.Trace(err)
		}
		if done {
			break
		}
	}
	return nil
}
