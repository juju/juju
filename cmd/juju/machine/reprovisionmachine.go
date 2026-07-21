// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v6"

	"github.com/juju/juju/api/client/machinemanager"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/rpc/params"
)

// NewReprovisionMachineCommand returns a command that reprovisions a machine.
func NewReprovisionMachineCommand() cmd.Command {
	return modelcmd.Wrap(&reprovisionMachineCommand{})
}

type reprovisionMachineCommand struct {
	modelcmd.ModelCommandBase
	modelcmd.IAASOnlyCommand
	machine string
	force   bool
	api     ReprovisionMachineAPI
}

// ReprovisionMachineAPI defines the methods on the client API that the
// reprovision-machine command calls.
type ReprovisionMachineAPI interface {
	Close() error
	ReprovisionMachine(ctx context.Context, machine string, force bool) (params.ErrorResult, error)
}

const reprovisionMachineDoc = `
Reprovision a machine whose backing cloud instance is operator-declared lost.
This preserves the Juju machine identity and unit assignment and creates a
replacement cloud instance through the normal provisioning path.

Root disk, ephemeral disk, charm-local state, and machine-scoped storage
data are NOT recovered. The replacement instance will have empty storage.

The --force flag is required by the server to acknowledge this data loss.
It does not bypass safety checks.

This command is only supported for top-level, non-controller, IaaS
provider-backed machines without child container machines or attached
model-scoped storage.
`

const reprovisionMachineExamples = `
    juju reprovision-machine 3 --force
`

// Info implements Command.Info.
func (c *reprovisionMachineCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "reprovision-machine",
		Args:     "<machine>",
		Purpose:  "Reprovision a machine whose cloud instance has been lost.",
		Doc:      reprovisionMachineDoc,
		Examples: reprovisionMachineExamples,
	})
}

// SetFlags implements Command.SetFlags.
func (c *reprovisionMachineCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.BoolVar(&c.force, "force", false, "Acknowledge that root disk, ephemeral disk, charm-local state, and machine-scoped storage data will be lost")
}

// Init implements Command.Init.
func (c *reprovisionMachineCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.Errorf("no machine specified")
	}
	if len(args) > 1 {
		return errors.Errorf("expected exactly one machine, got %d", len(args))
	}

	machine := args[0]
	if !names.IsValidMachine(machine) {
		return errors.Errorf("invalid machine %q", machine)
	}
	if names.IsContainerMachine(machine) {
		return errors.Errorf("invalid machine %q reprovision-machine does not support containers", machine)
	}

	c.machine = machine
	return nil
}

func (c *reprovisionMachineCommand) getAPI(ctx context.Context) (ReprovisionMachineAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	root, err := c.NewAPIRoot(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	client := machinemanager.NewClient(root)
	return client, nil
}

// Run implements Command.Run.
func (c *reprovisionMachineCommand) Run(ctx *cmd.Context) error {
	client, err := c.getAPI(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	result, err := client.ReprovisionMachine(ctx, c.machine, c.force)
	if err != nil {
		if params.IsCodeNotImplemented(err) {
			return errors.Errorf(
				"reprovision-machine is not supported by this controller; " +
					"the controller must be upgraded to a version that supports reprovisioning",
			)
		}
		return block.ProcessBlockedError(err, block.BlockChange)
	}
	if result.Error != nil {
		fmt.Fprintf(ctx.Stderr, "%v\n", result.Error)
		return nil
	}

	fmt.Fprintf(ctx.Stdout, "reprovisioning machine %s\n", c.machine)
	return nil
}
