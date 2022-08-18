// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/errors"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/core/secrets"
)

type secretRemoveCommand struct {
	cmd.CommandBase
	ctx Context

	uri string
}

// NewSecretRemoveCommand returns a command to remove a secret.
func NewSecretRemoveCommand(ctx Context) (cmd.Command, error) {
	return &secretRemoveCommand{ctx: ctx}, nil
}

// Info implements cmd.Command.
func (c *secretRemoveCommand) Info() *cmd.Info {
	doc := `
Remove a secret with the specified URI.

Examples:
    secret-remove secret:9m4e2mr0ui3e8a215n4g
`
	return jujucmd.Info(&cmd.Info{
		Name:    "secret-remove",
		Args:    "<ID>",
		Purpose: "remove a existing secret",
		Doc:     doc,
	})
}

// Init implements cmd.Command.
func (c *secretRemoveCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.New("missing secret URI")
	}
	c.uri = args[0]
	if _, err := secrets.ParseURI(c.uri); err != nil {
		return errors.Trace(err)
	}
	return cmd.CheckEmpty(args[1:])
}

// Run implements cmd.Command.
func (c *secretRemoveCommand) Run(ctx *cmd.Context) error {
	return c.ctx.RemoveSecret(c.uri)
}
