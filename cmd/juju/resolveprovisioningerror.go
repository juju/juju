// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/names"
)

// ResolveProvisioningErrorCommand marks a machine in an error state as able to attempt to
// be provisioned again.
type ResolveProvisioningErrorCommand struct {
	cmd.EnvCommandBase
	MachineId string
}

func (c *ResolveProvisioningErrorCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "resolve-provisioning-error",
		Args:    "<machine>",
		Purpose: "marks provisioning errors resolved",
	}
}

func (c *ResolveProvisioningErrorCommand) Init(args []string) error {
	if len(args) > 0 {
		c.MachineId = args[0]
		if !names.IsMachine(c.MachineId) {
			return fmt.Errorf("invalid machine %q", c.MachineId)
		}
		args = args[1:]
	} else {
		return fmt.Errorf("no machine specified")
	}
	return cmd.CheckEmpty(args)
}

func (c *ResolveProvisioningErrorCommand) Run(_ *cmd.Context) error {
	client, err := juju.NewAPIClientFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer client.Close()
	return client.ResolveProvisioningError(names.MachineTag(c.MachineId))
}
