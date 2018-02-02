// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
)

// ContainerspecSetCommand implements the container-spec-set command.
type ContainerspecSetCommand struct {
	cmd.CommandBase
	ctx Context

	specFile    cmd.FileVar
	application bool
}

// NewContainerspecSetCommand makes a container-spec-set command.
func NewContainerspecSetCommand(ctx Context) (cmd.Command, error) {
	return &ContainerspecSetCommand{ctx: ctx}, nil
}

func (c *ContainerspecSetCommand) Info() *cmd.Info {
	doc := `
Sets configuration data to use for a container.
By default, the spec applies to all units for the
application. However, if a unit name is specified,
the spec is used for just that unit.
`
	return &cmd.Info{
		Name:    "container-spec-set",
		Args:    "--file <container spec file> [--application]",
		Purpose: "set container spec information",
		Doc:     doc,
	}
}

func (c *ContainerspecSetCommand) SetFlags(f *gnuflag.FlagSet) {
	c.specFile.SetStdin()
	c.specFile.Path = "-"
	f.Var(&c.specFile, "file", "file containing container spec")
	f.BoolVar(&c.application, "application", false, "set the spec for the application to which the unit belongs if the unit is the leader")
}

func (c *ContainerspecSetCommand) Init(args []string) error {
	return cmd.CheckEmpty(args)
}

func (c *ContainerspecSetCommand) Run(ctx *cmd.Context) error {
	specData, err := c.handleSpecFile(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	return c.ctx.SetContainerSpec(specData, c.application)
}

func (c *ContainerspecSetCommand) handleSpecFile(ctx *cmd.Context) (string, error) {
	specData, err := c.specFile.Read(ctx)
	if err != nil {
		return "", errors.Trace(err)
	}
	if len(specData) == 0 {
		return "", errors.New("no container spec specified: pipe container spec to command, or specify a file with --file")
	}
	return string(specData), nil
}
