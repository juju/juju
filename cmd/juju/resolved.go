// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"

	"launchpad.net/gnuflag"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/names"
)

// ResolvedCommand marks a unit in an error state as ready to continue.
type ResolvedCommand struct {
	cmd.EnvCommandBase
	UnitName string
	Retry    bool
}

func (c *ResolvedCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "resolved",
		Args:    "<unit>",
		Purpose: "marks unit errors resolved",
	}
}

func (c *ResolvedCommand) SetFlags(f *gnuflag.FlagSet) {
	c.EnvCommandBase.SetFlags(f)
	f.BoolVar(&c.Retry, "r", false, "re-execute failed hooks")
	f.BoolVar(&c.Retry, "retry", false, "")
}

func (c *ResolvedCommand) Init(args []string) error {
	if len(args) > 0 {
		c.UnitName = args[0]
		if !names.IsUnit(c.UnitName) {
			return fmt.Errorf("invalid unit name %q", c.UnitName)
		}
		args = args[1:]
	} else {
		return fmt.Errorf("no unit specified")
	}
	return cmd.CheckEmpty(args)
}

func (c *ResolvedCommand) Run(_ *cmd.Context) error {
	client, err := juju.NewAPIClientFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer client.Close()
	return client.Resolved(c.UnitName, c.Retry)
}
