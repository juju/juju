// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"github.com/juju/cmd/v4"
	"github.com/juju/gnuflag"

	jujucmd "github.com/juju/juju/cmd"
)

type secretIdsCommand struct {
	cmd.CommandBase
	ctx Context

	out cmd.Output
}

// NewSecretIdsCommand returns a command to list the IDs and labels of secrets.
// created by this app.
func NewSecretIdsCommand(ctx Context) (cmd.Command, error) {
	return &secretIdsCommand{ctx: ctx}, nil
}

func (c *secretIdsCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters.Formatters())
}

// Info implements cmd.Command.
func (c *secretIdsCommand) Info() *cmd.Info {
	doc := `
Returns the secret ids for secrets owned by the application.
`
	examples := `
    secret-ids
`
	return jujucmd.Info(&cmd.Info{
		Name:     "secret-ids",
		Purpose:  "Print secret IDs.",
		Doc:      doc,
		Examples: examples,
	})
}

// Init implements cmd.Command.
func (c *secretIdsCommand) Init(args []string) error {
	return cmd.CheckEmpty(args)
}

// Run implements cmd.Command.
func (c *secretIdsCommand) Run(ctx *cmd.Context) error {
	result, err := c.ctx.SecretMetadata()
	if err != nil {
		return err
	}
	var out []string
	for id := range result {
		out = append(out, id)
	}
	return c.out.Write(ctx, out)
}
