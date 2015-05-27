// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package process

import (
	"gopkg.in/juju/charm.v6-unstable"
)

// Status represents the status of a worload process.
type Status string

// Status values specific to workload processes.
const (
	StatusPending Status = "pending"
	StatusActive  Status = "active"
	StatusFailed  Status = "failed"
	StatusStopped Status = "stopped"
)

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
