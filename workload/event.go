// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workload

// The kinds of events.
const (
	EventKindNoop      = ""
	EventKindTracked   = "tracked"
	EventKindUntracked = "untracked"
)

// Event describes something that happened for a workload.
type Event struct {
	// Kind identifies the type of event.
	Kind string
	// ID identifies the workload, relative to the current unit.
	ID string
	// Plugin is the plugin to use for this event.
	Plugin Plugin
	// PluginID is the ID that the plugin uses to identify a workload.
	PluginID string
}
