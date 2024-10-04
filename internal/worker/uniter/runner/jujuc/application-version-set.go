// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"github.com/juju/cmd/v4"
	"github.com/juju/errors"

	jujucmd "github.com/juju/juju/cmd"
)

type applicationVersionSetCommand struct {
	cmd.CommandBase
	ctx Context

	version string
}

// NewApplicationVersionSetCommand creates an application-version-set command.
func NewApplicationVersionSetCommand(ctx Context) (cmd.Command, error) {
	cmd := &applicationVersionSetCommand{ctx: ctx}
	return cmd, nil
}

// Info is part of the cmd.Command interface.
func (c *applicationVersionSetCommand) Info() *cmd.Info {
	doc := `
application-version-set tells Juju which version of the application
software is running. This could be a package version number or some
other useful identifier, such as a Git hash, that indicates the
version of the deployed software. (It shouldn't be confused with the
charm revision.) The version set will be displayed in "juju status"
output for the application.
`
	examples := `
    application-version-set 1.1.10
`
	return jujucmd.Info(&cmd.Info{
		Name:     "application-version-set",
		Args:     "<new-version>",
		Purpose:  "Specify which version of the application is deployed.",
		Doc:      doc,
		Examples: examples,
	})
}

// Init is part of the cmd.Command interface.
func (c *applicationVersionSetCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.New("no version specified")
	}
	c.version = args[0]
	return cmd.CheckEmpty(args[1:])
}

// Run is part of the cmd.Command interface.
func (c *applicationVersionSetCommand) Run(ctx *cmd.Context) error {
	return c.ctx.SetUnitWorkloadVersion(ctx, c.version)
}
