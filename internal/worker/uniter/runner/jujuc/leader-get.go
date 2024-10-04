// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"strings"

	"github.com/juju/cmd/v4"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	jujucmd "github.com/juju/juju/cmd"
)

// leaderGetCommand implements the leader-get command.
type leaderGetCommand struct {
	cmd.CommandBase
	ctx Context
	key string
	out cmd.Output
}

// NewLeaderGetCommand returns a new leaderGetCommand with the given context.
func NewLeaderGetCommand(ctx Context) (cmd.Command, error) {
	return &leaderGetCommand{ctx: ctx}, nil
}

// Info is part of the cmd.Command interface.
func (c *leaderGetCommand) Info() *cmd.Info {
	doc := `
leader-get prints the value of a leadership setting specified by key. If no key
is given, or if the key is "-", all keys and values will be printed.
`
	examples := `
    ADDRESSS=$(leader-get cluster-leader-address)
`
	return jujucmd.Info(&cmd.Info{
		Name:     "leader-get",
		Args:     "[<key>]",
		Purpose:  "Print application leadership settings.",
		Doc:      doc,
		Examples: examples,
	})
}

// SetFlags is part of the cmd.Command interface.
func (c *leaderGetCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters.Formatters())
}

// Init is part of the cmd.Command interface.
func (c *leaderGetCommand) Init(args []string) error {
	c.key = ""
	if len(args) == 0 {
		return nil
	}
	key := args[0]
	if key == "-" {
		key = ""
	} else if strings.Contains(key, "=") {
		return errors.Errorf("invalid key %q", key)
	}
	c.key = key
	return cmd.CheckEmpty(args[1:])
}

// Run is part of the cmd.Command interface.
func (c *leaderGetCommand) Run(ctx *cmd.Context) error {
	settings, err := c.ctx.LeaderSettings(ctx)
	if err != nil {
		return errors.Annotatef(err, "cannot read leadership settings")
	}
	if c.key == "" {
		return c.out.Write(ctx, settings)
	}
	if value, ok := settings[c.key]; ok {
		return c.out.Write(ctx, value)
	}
	return c.out.Write(ctx, nil)
}
