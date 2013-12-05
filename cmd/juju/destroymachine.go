// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"strings"

	"launchpad.net/gnuflag"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/rpc"
	"launchpad.net/juju-core/state"
)

// DestroyMachineCommand causes an existing machine to be destroyed.
type DestroyMachineCommand struct {
	cmd.EnvCommandBase
	MachineIds []string
	Force      bool
}

const destroyMachineDoc = `
Machines that are responsible for the environment cannot be destroyed. Machines
running units or containers can only be destroyed with the --force flag; doing
so will also destroy all those units and containers without giving them any
opportunity to shut down cleanly.
`

func (c *DestroyMachineCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "destroy-machine",
		Args:    "<machine> ...",
		Purpose: "destroy machines",
		Doc:     destroyMachineDoc,
		Aliases: []string{"terminate-machine"},
	}
}

func (c *DestroyMachineCommand) SetFlags(f *gnuflag.FlagSet) {
	c.EnvCommandBase.SetFlags(f)
	f.BoolVar(&c.Force, "force", false, "completely remove machine and all dependencies")
}

func (c *DestroyMachineCommand) Init(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no machines specified")
	}
	for _, id := range args {
		if !names.IsMachine(id) {
			return fmt.Errorf("invalid machine id %q", id)
		}
	}
	c.MachineIds = args
	return nil
}

// destroyMachines destroys the machines with the specified ids.
// This is copied from the 1.16.3 code to enable compatibility. It should be
// removed when we release a version that goes via the API only (whatever is
// after 1.18)
func destroyMachines(st *state.State, ids ...string) (err error) {
	var errs []string
	for _, id := range ids {
		machine, err := st.Machine(id)
		switch {
		case errors.IsNotFoundError(err):
			err = fmt.Errorf("machine %s does not exist", id)
		case err != nil:
		case machine.Life() != state.Alive:
			continue
		default:
			err = machine.Destroy()
		}
		if err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) == 0 {
		return nil
	}
	msg := "some machines were not destroyed"
	if len(errs) == len(ids) {
		msg = "no machines were destroyed"
	}
	return fmt.Errorf("%s: %s", msg, strings.Join(errs, "; "))
}

func (c *DestroyMachineCommand) run1dot16() error {
	if c.Force {
		return fmt.Errorf("destroy-machine --force is not supported in Juju servers older than 1.16.4")
	}
	conn, err := juju.NewConnFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer conn.Close()
	return destroyMachines(conn.State, c.MachineIds...)
}

func (c *DestroyMachineCommand) Run(_ *cmd.Context) error {
	apiclient, err := juju.NewAPIClientFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer apiclient.Close()
	if c.Force {
		err = apiclient.ForceDestroyMachines(c.MachineIds...)
	} else {
		err = apiclient.DestroyMachines(c.MachineIds...)
	}
	// Juju 1.16.3 and older did not have DestroyMachines as an API command.
	if rpc.IsNoSuchRequest(err) {
		logger.Infof("DestroyMachines not supported by the API server, " +
			"falling back to <=1.16.3 compatibility")
		return c.run1dot16()
	}
	return err
}
