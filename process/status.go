// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package process

import (
	"github.com/juju/errors"
)

// Status is the Juju-level status of a workload process.
type Status struct {
	// State is which state the process is in relative to Juju.
	State string
	// Failed identifies whether or not Juju got a failure while trying
	// to interact with the process (via its plugin).
	Failed bool
	// Message is a human-readable message describing the current status
	// of the process, why it is in the current state, or what Juju is
	// doing right now relative to the process. There may be no message.
	Message string
}

// PluginStatus represents the data returned from the Plugin.Status call.
type PluginStatus struct {
	// Label represents the human-readable label returned by the plugin
	// that represents the status of the workload process.
	Label string `json:"label"`
}

// Validate returns nil if this value is valid, and an error that satisfies
// IsValid if it is not.
func (s PluginStatus) Validate() error {
	if s.Label == "" {
		return errors.NotValidf("Label cannot be empty")
	}
	return nil
}
