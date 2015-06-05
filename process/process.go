// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package process

import (
	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6-unstable"
)

const ComponentName = "process"

// Status represents the status of a worload process.
type Status string

// Status values specific to workload processes.
const (
	StatusPending Status = "pending"
	StatusActive  Status = "active"
	StatusFailed  Status = "failed"
	StatusStopped Status = "stopped"
)

// Info holds information about a process that Juju needs.
type Info struct {
	charm.Process

	// Status is the overall Juju status of the workload process.
	Status Status

	// Space is the networking space with which the process was started.
	Space string

	// EnvVars is the set of environment variables with which the
	// process was started.
	EnvVars map[string]string

	// Details is the information about the process which the plugin provided.
	Details LaunchDetails
}

// NewInfo builds a new Info object with the provided values.
func NewInfo(name, pType string) *Info {
	return &Info{
		Process: charm.Process{
			Name: name,
			Type: pType,
		},
	}
}

// CheckStatus ensures that the provided status is supported.
func CheckStatus(status Status) error {
	switch status {
	case StatusPending, StatusActive, StatusFailed, StatusStopped:
		return nil
	case "":
		return errors.Errorf("missing Status")
	default:
		return errors.Errorf("unknown status %q", status)
	}
}
