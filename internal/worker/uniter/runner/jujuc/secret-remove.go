// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"github.com/juju/cmd/v4"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/core/secrets"
)

type secretRemoveCommand struct {
	cmd.CommandBase
	ctx Context

	secretURI *secrets.URI
	revision  int
}

// NewSecretRemoveCommand returns a command to remove a secret.
func NewSecretRemoveCommand(ctx Context) (cmd.Command, error) {
	return &secretRemoveCommand{ctx: ctx}, nil
}

// Info implements cmd.Command.
func (c *secretRemoveCommand) Info() *cmd.Info {
	doc := `
Remove a secret with the specified URI.
`
	examples := `
    secret-remove secret:9m4e2mr0ui3e8a215n4g
`
	return jujucmd.Info(&cmd.Info{
		Name:     "secret-remove",
		Args:     "<ID>",
		Purpose:  "Remove an existing secret.",
		Doc:      doc,
		Examples: examples,
	})
}

// SetFlags implements cmd.Command.
func (c *secretRemoveCommand) SetFlags(f *gnuflag.FlagSet) {
	f.IntVar(&c.revision, "revision", 0, "remove the specified revision")
}

// Init implements cmd.Command.
func (c *secretRemoveCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.New("missing secret URI")
	}
	var err error
	if c.secretURI, err = secrets.ParseURI(args[0]); err != nil {
		return errors.Trace(err)
	}
	return cmd.CheckEmpty(args[1:])
}

// Run implements cmd.Command.
func (c *secretRemoveCommand) Run(ctx *cmd.Context) error {
	var rev *int
	if c.revision > 0 {
		rev = &c.revision
	}
	return c.ctx.RemoveSecret(c.secretURI, rev)
}
