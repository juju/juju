// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/collections/set"
	"github.com/juju/gnuflag"

	jujucmd "github.com/juju/juju/cmd"
)

type secretIdsCommand struct {
	cmd.CommandBase
	ctx Context

	out    cmd.Output
	labels set.Strings
}

// NewSecretIdsCommand returns a command to list the IDs and labels of secrets.
// created by this app.
func NewSecretIdsCommand(ctx Context) (cmd.Command, error) {
	return &secretIdsCommand{ctx: ctx}, nil
}

// Info implements cmd.Command.
func (c *secretIdsCommand) Info() *cmd.Info {
	doc := `
Returns the secret ids and labels for secrets owned by the application.

Examples:
    secret-ids
    secret-ids label1 label2
`
	return jujucmd.Info(&cmd.Info{
		Name:    "secret-ids",
		Args:    "[<label> ]...",
		Purpose: "print secret ids and their labels",
		Doc:     doc,
	})
}

// SetFlags implements cmd.Command.
func (c *secretIdsCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})
}

// Init implements cmd.Command.
func (c *secretIdsCommand) Init(args []string) error {
	c.labels = set.NewStrings(args...)
	return nil
}

// Run implements cmd.Command.
func (c *secretIdsCommand) Run(ctx *cmd.Context) error {
	result, err := c.ctx.SecretIds()
	if err != nil {
		return err
	}
	out := make(map[string]string)
	for uri, label := range result {
		if c.labels.IsEmpty() || c.labels.Contains(label) {
			out[uri.ShortString()] = label
		}
	}
	return c.out.Write(ctx, out)
}
