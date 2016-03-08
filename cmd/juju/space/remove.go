// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space

import (
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/cmd/modelcmd"
)

func newRemoveCommand() cmd.Command {
	return modelcmd.Wrap(&removeCommand{})
}

// removeCommand calls the API to remove an existing network space.
type removeCommand struct {
	SpaceCommandBase
	name string
}

const removeCommandDoc = `
Removes an existing Juju network space with the given name. Any subnets
associated with the space will be transfered to the default space.
`

// Info is defined on the cmd.Command interface.
func (c *removeCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "remove",
		Args:    "<name>",
		Purpose: "remove a network space",
		Doc:     strings.TrimSpace(removeCommandDoc),
	}
}

// Init is defined on the cmd.Command interface. It checks the
// arguments for sanity and sets up the command to run.
func (c *removeCommand) Init(args []string) (err error) {
	defer errors.DeferredAnnotatef(&err, "invalid arguments specified")

	// Validate given name.
	if len(args) == 0 {
		return errors.New("space name is required")
	}
	givenName := args[0]
	if !names.IsValidSpace(givenName) {
		return errors.Errorf("%q is not a valid space name", givenName)
	}
	c.name = givenName

	return cmd.CheckEmpty(args[1:])
}

// Run implements Command.Run.
func (c *removeCommand) Run(ctx *cmd.Context) error {
	return c.RunWithAPI(ctx, func(api SpaceAPI, ctx *cmd.Context) error {
		// Remove the space.
		err := api.RemoveSpace(c.name)
		if err != nil {
			return errors.Annotatef(err, "cannot remove space %q", c.name)
		}
		ctx.Infof("removed space %q", c.name)
		return nil
	})
}
