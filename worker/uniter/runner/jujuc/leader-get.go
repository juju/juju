// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"
)

// LeaderGetCommand implements the leader-get command.
type LeaderGetCommand struct {
	cmd.CommandBase
	ctx Context
	Key string
	out cmd.Output
}

// NewLeaderGetCommand returns a new LeaderGetCommand with the given context.
func NewLeaderGetCommand(ctx Context) cmd.Command {
	return &LeaderGetCommand{ctx: ctx}
}

// Info is part of the cmd.Command interface.
func (c *LeaderGetCommand) Info() *cmd.Info {
	doc := `
leader-get prints the value of a leadership setting specified by key. If no key
is given, or if the key is "-", all keys and values will be printed.
`
	return &cmd.Info{
		Name:    "leader-get",
		Args:    "[<key>]",
		Purpose: "print service leadership settings",
		Doc:     doc,
	}
}

// SetFlags is part of the cmd.Command interface.
func (c *LeaderGetCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters)
}

// Init is part of the cmd.Command interface.
func (c *LeaderGetCommand) Init(args []string) error {
	c.Key = ""
	if len(args) == 0 {
		return nil
	}
	key := args[0]
	if key == "-" {
		key = ""
	} else if strings.Contains(key, "=") {
		return errors.Errorf("invalid key %q", key)
	}
	c.Key = key
	return cmd.CheckEmpty(args[1:])
}

// Run is part of the cmd.Command interface.
func (c *LeaderGetCommand) Run(ctx *cmd.Context) error {
	settings, err := c.ctx.LeaderSettings()
	if err != nil {
		return errors.Annotatef(err, "cannot read leadership settings")
	}
	if c.Key == "" {
		return c.out.Write(ctx, settings)
	}
	if value, ok := settings[c.Key]; ok {
		return c.out.Write(ctx, value)
	}
	return c.out.Write(ctx, nil)
}
