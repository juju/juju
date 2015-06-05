// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package process

import (
	"github.com/juju/errors"
	"gopkg.in/juju/charm.v5"
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
func NewInfo(name, procType string) *Info {
	return &Info{
		Process: charm.Process{
			Name: name,
			Type: procType,
		},
	}
}

// Validate checks the process info to ensure it is correct.
func (info Info) Validate() error {
	if err := info.Process.Validate(); err != nil {
		return errors.Trace(err)
	}

	if info.Status.IsUnknown() {
		return errors.Errorf("bad status %#v", info.Status)
	}

	return nil
}
