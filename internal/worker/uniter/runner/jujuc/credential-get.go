// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"github.com/juju/cmd/v4"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	jujucmd "github.com/juju/juju/cmd"
)

// CredentialGetCommand implements the leader-get command.
type CredentialGetCommand struct {
	cmd.CommandBase
	ctx Context
	out cmd.Output
}

// NewCredentialGetCommand returns a new CredentialGetCommand with the given context.
func NewCredentialGetCommand(ctx Context) (cmd.Command, error) {
	return &CredentialGetCommand{ctx: ctx}, nil
}

// Info is part of the cmd.Command interface.
func (c *CredentialGetCommand) Info() *cmd.Info {
	doc := `
credential-get returns the cloud specification used by the unit's model.
`
	return jujucmd.Info(&cmd.Info{
		Name:    "credential-get",
		Args:    "",
		Purpose: "access cloud credentials",
		Doc:     doc,
	})
}

// SetFlags is part of the cmd.Command interface.
func (c *CredentialGetCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters.Formatters())
}

// Init is part of the cmd.Command interface.
func (c *CredentialGetCommand) Init(args []string) error {
	return cmd.CheckEmpty(args)
}

// Run is part of the cmd.Command interface.
func (c *CredentialGetCommand) Run(ctx *cmd.Context) error {
	credential, err := c.ctx.CloudSpec(ctx)
	if err != nil {
		return errors.Annotatef(err, "cannot access cloud credentials")
	}
	return c.out.Write(ctx, credential)
}
