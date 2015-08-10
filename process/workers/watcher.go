// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workers

import (
	"github.com/juju/juju/process"
	"github.com/juju/juju/worker"
)

// Watcher watches for workload process events.
type Watcher struct {
	// Handler is the event handler used by the watcher.
	Handlers *EventHandler
	events   chan []process.Event
}

// NewWatcher wraps the
func NewWatcher() *Watcher {
	events := make(chan []process.Event) // TODO(ericsnow) Set a size?
	w := &Watcher{
		Handlers: NewEventHandler(events),
		events:   events,
	}
	return w
}

// Close cleans up the watcher's resources.
func (w *Watcher) Close() {
	close(w.events)
}

// NewWorker wraps the Watcher in a worker.
func (w *Watcher) NewWorker() (worker.Worker, error) {
	defer w.Close()
	return w.Handlers.NewWorker()
}
