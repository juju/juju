// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"
)

// WorkloadVersionSetCommand implements the workload-version-set command.
type WorkloadVersionSetCommand struct {
	cmd.CommandBase
	ctx Context

	version string
}

func NewWorkloadVersionSetCommand(ctx Context) (cmd.Command, error) {
	cmd := &WorkloadVersionSetCommand{ctx: ctx}
	return cmd, nil
}

// Info is part of the cmd.Command interface.
func (c *WorkloadVersionSetCommand) Info() *cmd.Info {
	doc := `
workload-version-set updates the workload version for the current unit
to the value passed to the command.
`
	return &cmd.Info{
		Name:    "workload-version-set",
		Args:    "<new-version>",
		Purpose: "set workload version",
		Doc:     doc,
	}
}

// SetFlags is part of the cmd.Command interface.
func (c *WorkloadVersionSetCommand) SetFlags(f *gnuflag.FlagSet) {
}

// Init is part of the cmd.Command interface.
func (c *WorkloadVersionSetCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.New("no version specified")
	}
	c.version = args[0]
	return cmd.CheckEmpty(args[1:])
}

// Run is part of the cmd.Command interface.
func (c *WorkloadVersionSetCommand) Run(ctx *cmd.Context) error {
	return c.ctx.SetUnitWorkloadVersion(c.version)
}
