// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package process

import (
	"gopkg.in/juju/charm.v6-unstable"
)

// Status values specific to workload processes.
const (
	StatusPending Status = iota
	StatusActive
	StatusFailed
	StatusStopped
)

// Status represents the status of a worload process.
type Status string

// String implements fmt.Stringer.
func (s Status) String() string {
	switch status {
	case StatusPending:
		return "pending"
	case StatusActive:
		return "active"
	case StatusFailed:
		return "failed"
	case StatusStopped:
		return "stopped"
	}
	return "Unknown"
}

// ProcessInfo holds information about a process that Juju needs.
type ProcessInfo struct {
	charm.Process

	// Status is the overall Juju status of the workload process.
	Status Status

	// Space is the networking space with which the process was started.
	Space string

	// EnvVars is the set of environment variables with which the
	// process was started.
	EnvVars map[string]string

	// Details is the information about the process which the plugin provided.
	Details ProcessDetails
}
