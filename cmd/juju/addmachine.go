// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"

	"launchpad.net/gnuflag"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/cmd/envcmd"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs/manual"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/state/api/params"
)

// sshHostPrefix is the prefix for a machine to be "manually provisioned".
const sshHostPrefix = "ssh:"

var addMachineDoc = `

If no container is specified, a new machine will be
provisioned.  If a container is specified, a new machine will be provisioned
with that container.

To add a container to an existing machine, use the <container>:<machinenumber>
format.

When adding a new machine, you may specify constraints for the machine to be
provisioned.  Constraints cannot be combined with deploying a container to an
existing machine.

Currently, the only supported container type is lxc.

Machines are created in a clean state and ready to have units deployed.

This command also supports manual provisioning of existing machines via SSH. The
target machine must be able to communicate with the API server, and be able to
access the environment storage.

Examples:
   juju add-machine                      (starts a new machine)
   juju add-machine lxc                  (starts a new machine with an lxc container)
   juju add-machine lxc:4                (starts a new lxc container on machine 4)
   juju add-machine --constraints mem=8G (starts a machine with at least 8GB RAM)
   juju add-machine ssh:user@10.10.0.3   (manually provisions a machine with ssh)

See Also:
   juju help constraints
`

// AddMachineCommand starts a new machine and registers it in the environment.
type AddMachineCommand struct {
	envcmd.EnvCommandBase
	// If specified, use this series, else use the environment default-series
	Series string
	// If specified, these constraints are merged with those already in the environment.
	Constraints constraints.Value
	// Placement is passed verbatim to the API, to be parsed and evaluated server-side.
	Placement *instance.Placement
}

func (c *AddMachineCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "add-machine",
		Args:    "[<container>:machine | <container> | ssh:[user@]host]",
		Purpose: "start a new, empty machine and optionally a container, or add a container to a machine",
		Doc:     addMachineDoc,
	}
}

func (c *AddMachineCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.Series, "series", "", "the charm series")
	f.Var(constraints.ConstraintsValue{Target: &c.Constraints}, "constraints", "additional machine constraints")
}

func (c *AddMachineCommand) Init(args []string) error {
	if c.Constraints.Container != nil {
		return fmt.Errorf("container constraint %q not allowed when adding a machine", *c.Constraints.Container)
	}
	placement, err := cmd.ZeroOrOneArgs(args)
	if err != nil {
		return err
	}
	c.Placement, err = instance.ParsePlacement(placement)
	if err == instance.ErrPlacementScopeMissing {
		placement = c.EnvName + ":" + placement
		c.Placement, err = instance.ParsePlacement(placement)
	}
	if err != nil {
		return err
	}
	return nil
}

func (c *AddMachineCommand) Run(ctx *cmd.Context) error {
	if c.Placement != nil && c.Placement.Scope == "ssh" {
		args := manual.ProvisionMachineArgs{
			Host:    c.Placement.Directive,
			EnvName: c.EnvName,
			Stdin:   ctx.Stdin,
			Stdout:  ctx.Stdout,
			Stderr:  ctx.Stderr,
		}
		_, err := manual.ProvisionMachine(args)
		return err
	}

	client, err := juju.NewAPIClientFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer client.Close()

	if c.Placement != nil && c.Placement.Scope == instance.MachineScope {
		// It does not make sense to add-machine <id>.
		return fmt.Errorf("machine-id cannot be specified when adding machines")
	}

	machineParams := params.AddMachineParams{
		Placement:   c.Placement,
		Series:      c.Series,
		Constraints: c.Constraints,
		Jobs:        []params.MachineJob{params.JobHostUnits},
	}
	results, err := client.AddMachines([]params.AddMachineParams{machineParams})
	if params.IsCodeNotImplemented(err) {
		if c.Placement != nil {
			containerType, parseErr := instance.ParseContainerType(c.Placement.Scope)
			if parseErr != nil {
				// The user specified a non-container placement directive:
				// return original API not implemented error.
				return err
			}
			machineParams.ContainerType = containerType
			machineParams.ParentId = c.Placement.Directive
			machineParams.Placement = nil
		}
		logger.Infof(
			"AddMachinesWithPlacement not supported by the API server, " +
				"falling back to 1.18 compatibility mode",
		)
		results, err = client.AddMachines1dot18([]params.AddMachineParams{machineParams})
	}
	if err != nil {
		return err
	}

	// Currently, only one machine is added, but in future there may be several added in one call.
	machineInfo := results[0]
	if machineInfo.Error != nil {
		return machineInfo.Error
	}
	machineId := machineInfo.Machine

	if names.IsContainerMachine(machineId) {
		ctx.Infof("created container %v", machineId)
	} else {
		ctx.Infof("created machine %v", machineId)
	}
	return nil
}
