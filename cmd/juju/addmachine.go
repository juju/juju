// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"strings"

	"launchpad.net/gnuflag"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs/manual"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/state"
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

See Also:
   juju help constraints
`

// AddMachineCommand starts a new machine and registers it in the environment.
type AddMachineCommand struct {
	cmd.EnvCommandBase
	// If specified, use this series, else use the environment default-series
	Series string
	// If specified, these constraints are merged with those already in the environment.
	Constraints   constraints.Value
	MachineId     string
	ContainerType instance.ContainerType
	SSHHost       string
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
	c.EnvCommandBase.SetFlags(f)
	f.StringVar(&c.Series, "series", "", "the charm series")
	f.Var(constraints.ConstraintsValue{&c.Constraints}, "constraints", "additional machine constraints")
}

func (c *AddMachineCommand) Init(args []string) error {
	if c.Constraints.Container != nil {
		return fmt.Errorf("container constraint %q not allowed when adding a machine", *c.Constraints.Container)
	}
	containerSpec, err := cmd.ZeroOrOneArgs(args)
	if err != nil {
		return err
	}
	if containerSpec == "" {
		return nil
	}
	if strings.HasPrefix(containerSpec, sshHostPrefix) {
		c.SSHHost = containerSpec[len(sshHostPrefix):]
	} else {
		// container arg can either be 'type:machine' or 'type'
		if c.ContainerType, err = instance.ParseContainerType(containerSpec); err != nil {
			if names.IsMachine(containerSpec) || !cmd.IsMachineOrNewContainer(containerSpec) {
				return fmt.Errorf("malformed container argument %q", containerSpec)
			}
			sep := strings.Index(containerSpec, ":")
			c.MachineId = containerSpec[sep+1:]
			c.ContainerType, err = instance.ParseContainerType(containerSpec[:sep])
		}
	}
	return err
}

// addMachine1dot16 runs Client.AddMachines using a direct DB connection to maintain
// compatibility with an API server running 1.16 or older (when AddMachines
// was not available). This fallback can be removed when we no longer maintain
// 1.16 compatibility.
// This was copied directly from the code in AddMachineCommand.Run in 1.16
func (c *AddMachineCommand) addMachine1dot16() (string, error) {
	conn, err := juju.NewConnFromName(c.EnvName)
	if err != nil {
		return "", err
	}
	defer conn.Close()

	series := c.Series
	if series == "" {
		conf, err := conn.State.EnvironConfig()
		if err != nil {
			return "", err
		}
		series = conf.DefaultSeries()
	}
	template := state.MachineTemplate{
		Series:      series,
		Constraints: c.Constraints,
		Jobs:        []state.MachineJob{state.JobHostUnits},
	}
	var m *state.Machine
	switch {
	case c.ContainerType == "":
		m, err = conn.State.AddOneMachine(template)
	case c.MachineId != "":
		m, err = conn.State.AddMachineInsideMachine(template, c.MachineId, c.ContainerType)
	default:
		m, err = conn.State.AddMachineInsideNewMachine(template, template, c.ContainerType)
	}
	if err != nil {
		return "", err
	}
	return m.String(), err
}

func (c *AddMachineCommand) Run(ctx *cmd.Context) error {
	if c.SSHHost != "" {
		args := manual.ProvisionMachineArgs{
			Host:    c.SSHHost,
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

	machineParams := params.AddMachineParams{
		ParentId:      c.MachineId,
		ContainerType: c.ContainerType,
		Series:        c.Series,
		Constraints:   c.Constraints,
		Jobs:          []params.MachineJob{params.JobHostUnits},
	}
	results, err := client.AddMachines([]params.AddMachineParams{machineParams})
	var machineId string
	if params.IsCodeNotImplemented(err) {
		logger.Infof("AddMachines not supported by the API server, " +
			"falling back to 1.16 compatibility mode (direct DB access)")
		machineId, err = c.addMachine1dot16()
	} else if err != nil {
		return err
	} else {
		// Currently, only one machine is added, but in future there may be several added in one call.
		machineInfo := results[0]
		var machineErr *params.Error
		machineId, machineErr = machineInfo.Machine, machineInfo.Error
		if machineErr != nil {
			err = machineErr
		}
	}
	if err != nil {
		return err
	}
	if c.ContainerType == "" {
		logger.Infof("created machine %v", machineId)
	} else {
		logger.Infof("created %q container on machine %v", c.ContainerType, machineId)
	}
	return nil
}
