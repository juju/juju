// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package process

const (
	EventKindNoop      = ""
	EventKindTracked   = "tracked"
	EventKindUntracked = "untracked"
)

// Event describes something that happened for a workload process.
type Event struct {
	// Kind identifies the type of event.
	Kind string
	// ID identifies the workload process, relative to the current unit.
	ID string
}

// AddEvents adds the provided events to the channel.
func AddEvents(ch chan []Event, events ...Event) {
	// TODO(ericsnow) Validate the events?
	ch <- events
}
