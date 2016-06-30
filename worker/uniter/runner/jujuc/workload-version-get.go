// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"
)

type workloadVersionGetCommand struct {
	cmd.CommandBase
	ctx Context

	out cmd.Output
}

// NewWorkloadVersionGetCommand creates a workload-version-get command.
func NewWorkloadVersionGetCommand(ctx Context) (cmd.Command, error) {
	cmd := &workloadVersionGetCommand{ctx: ctx}
	return cmd, nil
}

// Info is part of the cmd.Command interface.
func (c *workloadVersionGetCommand) Info() *cmd.Info {
	doc := `
workload-version-get returns the currently-set workload version for
this unit. It takes no arguments.
`
	return &cmd.Info{
		Name:    "workload-version-get",
		Args:    "",
		Purpose: "get workload version",
		Doc:     doc,
	}
}

// SetFlags is part of the cmd.Command interface.
func (c *workloadVersionGetCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters)
}

// Run is part of the cmd.Command interface.
func (c *workloadVersionGetCommand) Run(ctx *cmd.Context) error {
	version, err := c.ctx.UnitWorkloadVersion()
	if err != nil {
		return errors.Trace(err)
	}
	return c.out.Write(ctx, version)
}
