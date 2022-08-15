// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/core/secrets"
)

type secretGetCommand struct {
	cmd.CommandBase
	ctx Context
	out cmd.Output

	secretUri *secrets.URI
	label     string
	key       string
	peek      bool
	update    bool
}

// NewSecretGetCommand returns a command to get a secret value.
func NewSecretGetCommand(ctx Context) (cmd.Command, error) {
	return &secretGetCommand{ctx: ctx}, nil
}

// Info implements cmd.Command.
func (c *secretGetCommand) Info() *cmd.Info {
	doc := `
Get the value of a secret with a given secret ID.
The first time the value is fetched, the latest revision is used.
Subsequent calls will always return this same revision unless
--peek or --update are used.
Using --peek will fetch the latest revision just this time.
Using --update will fetch the latest revision and continue to
return the same revision next time unless --peek or --update is used.


Examples
    secret-get secret:9m4e2mr0ui3e8a215n4g
    secret-get secret:9m4e2mr0ui3e8a215n4g token
    secret-get secret:9m4e2mr0ui3e8a215n4g token#base64
    secret-get secret:9m4e2mr0ui3e8a215n4g --format json
    secret-get secret:9m4e2mr0ui3e8a215n4g --peek
    secret-get secret:9m4e2mr0ui3e8a215n4g --update
    secret-get secret:9m4e2mr0ui3e8a215n4g --label db-password
`
	return jujucmd.Info(&cmd.Info{
		Name:    "secret-get [key[#base64]]",
		Args:    "<ID>",
		Purpose: "get the value of a secret",
		Doc:     doc,
	})
}

// SetFlags implements cmd.Command.
func (c *secretGetCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})
	f.StringVar(&c.label, "label", "", "a label used to identify the secret in hooks")
	f.BoolVar(&c.peek, "peek", false,
		`get the latest revision just this time`)
	f.BoolVar(&c.update, "update", false,
		`get the latest revision and also get this same revision for subsequent calls`)
}

// Init implements cmd.Command.
func (c *secretGetCommand) Init(args []string) (err error) {
	if len(args) < 1 {
		return errors.New("missing secret URI")
	}
	c.secretUri, err = secrets.ParseURI(args[0])
	if err != nil {
		return errors.NotValidf("secret URI %q", args[0])
	}
	if c.peek && c.update {
		return errors.New("specify one of --peek or --update but not both")
	}
	if len(args) > 1 {
		c.key = args[1]
		return cmd.CheckEmpty(args[2:])
	}
	return cmd.CheckEmpty(args[1:])
}

// Run implements cmd.Command.
func (c *secretGetCommand) Run(ctx *cmd.Context) error {
	value, err := c.ctx.GetSecret(c.secretUri.String(), c.label, c.update, c.peek)
	if err != nil {
		return err
	}

	var val interface{}
	val, err = value.Values()
	if err != nil {
		return err
	}
	if c.key == "" {
		return c.out.Write(ctx, val)
	}

	val, err = value.KeyValue(c.key)
	if err != nil {
		return err
	}
	return c.out.Write(ctx, val)
}
