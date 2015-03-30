// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space

import (
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
)

// RemoveCommand calls the API to remove a new network space.
type RemoveCommand struct {
	SpaceCommandBase
	Name string
}

const removeCommandDoc = `
Removes a network space with a given name

A network space name can consist of ...
`

// Info is defined on the cmd.Command interface.
func (c *RemoveCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "remove",
		Args:    "<name>",
		Purpose: "remove a network space",
		Doc:     strings.TrimSpace(removeCommandDoc),
	}
}

// Init is defined on the cmd.Command interface. It checks the
// arguments for sanity and sets up the command to run.
func (c *RemoveCommand) Init(args []string) error {
	// Validate given name.
	if len(args) == 0 {
		return errors.New("Space name is required")
	} else if len(args) > 1 {
		return errors.New("Please only provide a single space name.")
	}
	givenName := args[0]
	if !names.IsValidSpace(givenName) {
		return errors.Errorf("%q is not a valid space name", givenName)
	}
	c.Name = givenName

	return nil
}

// Run implements Command.Run.
func (c *RemoveCommand) Run(ctx *cmd.Context) error {
	api, err := c.NewAPI()
	if err != nil {
		return errors.Annotate(err, "cannot connect to API server")
	}
	defer api.Close()

	// Remove the space.
	err = api.RemoveSpace(c.Name)
	if err != nil {
		return errors.Annotatef(err, "cannot remove space %q", c.Name)
	}
	ctx.Infof("removed space %q", c.Name)
	return nil
}
