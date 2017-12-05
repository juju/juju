// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/juju/names.v2"
)

// ContainerspecSetCommand implements the container-spec-set command.
type ContainerspecSetCommand struct {
	cmd.CommandBase
	ctx Context

	specFile cmd.FileVar
	unitName string
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
		Args:    "--file <container spec file> [--unit <unit-name>]",
		Purpose: "set container spec information",
		Doc:     doc,
	}
}

func (c *ContainerspecSetCommand) SetFlags(f *gnuflag.FlagSet) {
	c.specFile.SetStdin()
	c.specFile.Path = "-"
	f.Var(&c.specFile, "file", "file containing container spec")
	f.StringVar(&c.unitName, "unit", "", "set this spec for the specified unit")
}

func (c *ContainerspecSetCommand) Init(args []string) error {
	if c.unitName != "" && !names.IsValidUnit(c.unitName) {
		return errors.NotValidf("unit name %q", c.unitName)
	}
	return cmd.CheckEmpty(args)
}

func (c *ContainerspecSetCommand) Run(ctx *cmd.Context) error {
	specData, err := c.handleSpecFile(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	return c.ctx.SetContainerSpec(c.unitName, specData)
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
