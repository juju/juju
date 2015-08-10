// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package process

import (
	"gopkg.in/juju/charm.v5"
)

// The kinds of events.
const (
	EventKindNoop      = ""
	EventKindTracked   = "tracked"
	EventKindUntracked = "untracked"
)

// Plugin represents the functionality of a workload process plugin.
type Plugin interface {
	// Launch runs the plugin's "launch" command, passing the provided
	// process definition. The output is converted to a Details.
	Launch(definition charm.Process) (Details, error)
	// Destroy runs the plugin's "destroy" command for the given ID.
	Destroy(id string) error
	// Status runs the plugin's "status" command. The output is
	// converted to a PluginStatus.
	Status(id string) (PluginStatus, error)
}

// Event describes something that happened for a workload process.
type Event struct {
	// Kind identifies the type of event.
	Kind string
	// ID identifies the workload process, relative to the current unit.
	ID string
	// Plugin is the plugin to use for this event.
	Plugin Plugin
}
