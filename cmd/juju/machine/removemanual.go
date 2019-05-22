// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs/manual"
	"github.com/juju/juju/environs/manual/sshprovisioner"
)

// NewRemoveManualCommand returns a command used to remove a specified machine.
func NewRemoveManualCommand() cmd.Command {
	return modelcmd.Wrap(&removeManualCommand{})
}

// removeManualCommand causes an existing machine to be destroyed.
type removeManualCommand struct {
	baseMachinesCommand
	fs        *gnuflag.FlagSet
	Placement *instance.Placement
}

const removeManualMachineDoc = `
Removing manual provisioned machines can be cleaned up as long as they're still
accessible via SSH. Clean up of the machine removes the Juju services along with
cleaning up of lib and log directories.

Examples:

    juju remove-manual-machine ssh:user@10.10.0.3
    juju remove-manual-machine winrm:user@10.10.0.3

See also:
	add-machine
	remove-machine
`

// Info implements Command.Info.
func (c *removeManualCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "remove-manual-machine",
		Args:    "[ssh:[user@]host | winrm:[user@]host]",
		Purpose: "Removes a manual machine.",
		Doc:     removeManualMachineDoc,
	})
}

// SetFlags implements Command.SetFlags.
func (c *removeManualCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	c.fs = f
}

func (c *removeManualCommand) Init(args []string) error {
	if len(args) != 1 {
		return errors.Errorf("wrong number of arguments, expected 1")
	}

	placement, err := instance.ParsePlacement(args[0])
	if err != nil {
		return errors.Annotatef(err, "placement parse error for %q", args[0])
	}
	if placement.Scope == instance.MachineScope {
		return errors.Errorf("remove-manual-machine expects user@host argument. Instead please use remove-machine %s", args[0])
	}
	if placement.Directive == "" {
		return errors.Errorf("invalid placement directive %q", args[0])
	}
	c.Placement = placement

	return nil
}

// Run implements Command.Run.
func (c *removeManualCommand) Run(ctx *cmd.Context) error {
	err := c.removeManual(ctx)
	if err == errNonManualScope {
		return errors.Errorf("unexpected placement scope %s", c.Placement.Scope)
	}
	if err != nil {
		return err
	}
	return nil
}

var (
	sshRemover = sshprovisioner.RemoveMachine
)

func (c *removeManualCommand) removeManual(ctx *cmd.Context) error {
	var removeMachine manual.RemoveMachineFunc
	var removeMachineCommandExec manual.CommandExec
	switch c.Placement.Scope {
	case sshScope:
		removeMachine = sshRemover
		removeMachineCommandExec = sshprovisioner.DefaultCommandExec()
	case winrmScope:
		return errors.NotImplementedf("todo")
	default:
		return errNonManualScope
	}

	user, host := splitUserHost(c.Placement.Directive)
	args := manual.RemoveMachineArgs{
		Host:        host,
		User:        user,
		Stdin:       ctx.Stdin,
		Stdout:      ctx.Stdout,
		Stderr:      ctx.Stderr,
		CommandExec: removeMachineCommandExec,
	}

	if err := removeMachine(args); err != nil {
		return errors.Trace(err)
	}
	ctx.Infof("machine removed")
	return nil
}
