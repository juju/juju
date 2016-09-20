// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
)

func newResolvedCommand() cmd.Command {
	return modelcmd.Wrap(&resolvedCommand{})
}

// resolvedCommand marks a unit in an error state as ready to continue.
type resolvedCommand struct {
	modelcmd.ModelCommandBase
	UnitName string
	NoRetry  bool
}

func (c *resolvedCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "resolved",
		Args:    "<unit>",
		Purpose: "Marks unit errors resolved and re-executes failed hooks",
	}
}

func (c *resolvedCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.BoolVar(&c.NoRetry, "no-retry", false, "Do not re-execute failed hooks on the unit")
}

func (c *resolvedCommand) Init(args []string) error {
	if len(args) > 0 {
		c.UnitName = args[0]
		if !names.IsValidUnit(c.UnitName) {
			return errors.Errorf("invalid unit name %q", c.UnitName)
		}
		args = args[1:]
	} else {
		return errors.Errorf("no unit specified")
	}
	return cmd.CheckEmpty(args)
}

func (c *resolvedCommand) Run(_ *cmd.Context) error {
	client, err := c.NewAPIClient()
	if err != nil {
		return err
	}
	defer client.Close()
	return block.ProcessBlockedError(client.Resolved(c.UnitName, c.NoRetry), block.BlockChange)
}
