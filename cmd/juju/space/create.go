// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space

import (
	"strings"

	"github.com/juju/cmd"
)

// CreateCommand calls the API to create a new network space.
type CreateCommand struct {
	SpaceCommandBase
}

const createEnvHelpDoc = `
This command will create a network space... bla bla bla
`

func (c *CreateCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "create",
		Args:    "<name> [<CIDR1> <CIDR2> ...]",
		Purpose: "create network space",
		Doc:     strings.TrimSpace(createEnvHelpDoc),
	}
}

// Run implements Command.Run.
func (c *CreateCommand) Run(ctx *cmd.Context) (err error) {
	return nil
}
