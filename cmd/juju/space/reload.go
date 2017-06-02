// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space

import (
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"

	"github.com/juju/juju/cmd/modelcmd"
)

// NewListCommand returns a command used to list spaces.
func NewReloadCommand() modelcmd.ModelCommand {
	return modelcmd.Wrap(&ReloadCommand{})
}

// listCommand displays a list of all spaces known to Juju.
type ReloadCommand struct {
	SpaceCommandBase
}

const ReloadCommandDoc = `
Reloades spaces and subnets from substrate
`

// Info is defined on the cmd.Command interface.
func (c *ReloadCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "reload-spaces",
		Purpose: "Reloads spaces and subnets from substrate.",
		Doc:     strings.TrimSpace(ReloadCommandDoc),
	}
}

// Run implements Command.Run.
func (c *ReloadCommand) Run(ctx *cmd.Context) error {
	return c.RunWithAPI(ctx, func(api SpaceAPI, ctx *cmd.Context) error {
		err := api.ReloadSpaces()
		if err != nil {
			if errors.IsNotSupported(err) {
				ctx.Infof("cannot reload spaces: %v", err)
			}
			return errors.Annotate(err, "cannot reload spaces")
		}
		return nil
	})
}
