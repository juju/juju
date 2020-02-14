// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space

import (
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"gopkg.in/juju/names.v3"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
)

// NewRemoveCommand returns a command used to remove a space.
func NewRemoveCommand() modelcmd.ModelCommand {
	return modelcmd.Wrap(&RemoveCommand{})
}

// RemoveCommand calls the API to remove an existing network space.
type RemoveCommand struct {
	SpaceCommandBase
	name string
}

const removeCommandDoc = `
Removes an existing Juju network space with the given name. Any subnets
associated with the space will be transferred to the default space.

Examples:

Remove a space by name:
	juju remove-space db-space

See also:
	add-space
	list-spaces
	reload-spaces
	rename-space
	show-space
`

// Info is defined on the cmd.Command interface.
func (c *RemoveCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "remove-space",
		Args:    "<name>",
		Purpose: "Remove a network space",
		Doc:     strings.TrimSpace(removeCommandDoc),
	})
}

// Init is defined on the cmd.Command interface. It checks the
// arguments for sanity and sets up the command to run.
func (c *RemoveCommand) Init(args []string) (err error) {
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
func (c *RemoveCommand) Run(ctx *cmd.Context) error {
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
