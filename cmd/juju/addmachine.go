// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/names"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/manual"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju"
	"github.com/juju/juju/state/api/params"
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
   juju add-machine -n 2                 (starts 2 new machines)
   juju add-machine lxc                  (starts a new machine with an lxc container)
   juju add-machine lxc -n 2             (starts 2 new machines with an lxc container)
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

	NumMachines int
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
	f.IntVar(&c.NumMachines, "n", 1, "The number of machines to add")
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
	if c.NumMachines > 1 && c.Placement != nil && c.Placement.Directive != "" {
		return fmt.Errorf("cannot use -n when specifying a placement directive")
	}
	return nil
}

type AddMachineAPI interface {
	Close() error
	AddMachines([]params.AddMachineParams) ([]params.AddMachinesResult, error)
	AddMachines1dot18([]params.AddMachineParams) ([]params.AddMachinesResult, error)
}

var getAddMachineAPI = func(envname string) (AddMachineAPI, error) {
	return juju.NewAPIClientFromName(envname)
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

	client, err := getAddMachineAPI(c.EnvName)
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
	machines := make([]params.AddMachineParams, c.NumMachines)
	for i := 0; i < c.NumMachines; i++ {
		machines[i] = machineParams
	}

	results, err := client.AddMachines(machines)
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

	errs := []error{}
	for _, machineInfo := range results {
		if machineInfo.Error != nil {
			errs = append(errs, machineInfo.Error)
			continue
		}
		machineId := machineInfo.Machine

		if names.IsContainerMachine(machineId) {
			ctx.Infof("created container %v", machineId)
		} else {
			ctx.Infof("created machine %v", machineId)
		}
	}
	if len(errs) == 1 {
		fmt.Fprintf(ctx.Stderr, "failed to create 1 machine\n")
		return errs[0]
	}
	if len(errs) > 1 {
		fmt.Fprintf(ctx.Stderr, "failed to create %d machines\n", len(errs))
		returnErr := []string{}
		for _, e := range errs {
			returnErr = append(returnErr, fmt.Sprintf("%s", e))
		}
		return errors.New(strings.Join(returnErr, ", "))
	}
	return nil
}
