// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
)

// DestroyEnvironmentCommand destroys an environment.
type DestroyEnvironmentCommand struct {
	EnvCommandBase
}

func (c *DestroyEnvironmentCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "destroy-environment",
		Purpose: "terminate all machines and other associated resources for an environment",
	}
}

func (c *DestroyEnvironmentCommand) Run(_ *cmd.Context) error {
	environ, err := environs.NewFromName(c.EnvName)
	if err != nil {
		return err
	}
	return environ.Destroy(nil)
}
