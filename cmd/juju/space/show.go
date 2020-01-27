// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space

import (
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/apiserver/params"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/output"
)

// NewAddCommand returns a command used to add a network space.
func NewShowSpaceCommand() modelcmd.ModelCommand {
	return modelcmd.Wrap(&ShowSpaceCommand{})
}

// ShowSpaceCommand calls the API to add a new network space.
type ShowSpaceCommand struct {
	SpaceCommandBase
	Name string

	out cmd.Output
}

const ShowSpaceCommandDoc = `
Displays extended information about a given space. 
Output includes the space subnets, applications with bindings to the space,
and a count of machines connected to the space.`

// SetFlags implements part of the cmd.Command interface.
func (c *ShowSpaceCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", output.DefaultFormatters)
}

// Info is defined on the cmd.Command interface.
func (c *ShowSpaceCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "show-space",
		Args:    "<name>",
		Purpose: "Shows information about the network space.",
		Doc:     strings.TrimSpace(ShowSpaceCommandDoc),
	})
}

// Init is defined on the cmd.Command interface. It checks the
// arguments for sanity and sets up the command to run.
func (c *ShowSpaceCommand) Init(args []string) error {
	if lArgs := len(args); lArgs != 1 {
		return errors.Errorf("expected exactly 1 space name, got %d arguments", lArgs)
	}
	c.Name = args[0]
	return nil
}

// Run implements Command.Run.
func (c *ShowSpaceCommand) Run(ctx *cmd.Context) error {
	return c.RunWithAPI(ctx, func(api SpaceAPI, ctx *cmd.Context) error {
		// Add the new space.
		spaceInformation, err := api.ShowSpace(c.Name)
		if err != nil {
			if errors.IsNotSupported(err) {
				ctx.Infof("cannot retrieve space %q: %v", c.Name, err)
			}
			if params.IsCodeUnauthorized(err) {
				common.PermissionsMessage(ctx.Stderr, "retrieving space")
			}
			return block.ProcessBlockedError(errors.Annotatef(err, "cannot retrieve space %q", c.Name), block.BlockChange)
		}
		return errors.Trace(c.out.Write(ctx, spaceInformation))
	})
}
