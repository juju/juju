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

type secretGetCommand struct {
	cmd.CommandBase
	ctx Context
	out cmd.Output

	secretUri *secrets.URI
	label     string
	key       string
	peek      bool
	refresh   bool
}

// NewSecretGetCommand returns a command to get a secret value.
func NewSecretGetCommand(ctx Context) (cmd.Command, error) {
	return &secretGetCommand{ctx: ctx}, nil
}

// Info implements cmd.Command.
func (c *secretGetCommand) Info() *cmd.Info {
	doc := `
Get the content of a secret with a given secret ID.
The first time the value is fetched, the latest revision is used.
Subsequent calls will always return this same revision unless
--peek or --refresh are used.
Using --peek will fetch the latest revision just this time.
Using --refresh will fetch the latest revision and continue to
return the same revision next time unless --peek or --refresh is used.

Either the ID or label can be used to identify the secret.
`
	examples := `
    secret-get secret:9m4e2mr0ui3e8a215n4g
    secret-get secret:9m4e2mr0ui3e8a215n4g token
    secret-get secret:9m4e2mr0ui3e8a215n4g token#base64
    secret-get secret:9m4e2mr0ui3e8a215n4g --format json
    secret-get secret:9m4e2mr0ui3e8a215n4g --peek
    secret-get secret:9m4e2mr0ui3e8a215n4g --refresh
    secret-get secret:9m4e2mr0ui3e8a215n4g --label db-password
`
	return jujucmd.Info(&cmd.Info{
		Name:     "secret-get",
		Args:     "<ID> [key[#base64]]",
		Purpose:  "Get the content of a secret.",
		Doc:      doc,
		Examples: examples,
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
	f.BoolVar(&c.refresh, "refresh", false,
		`get the latest revision and also get this same revision for subsequent calls`)
}

// Init implements cmd.Command.
func (c *secretGetCommand) Init(args []string) (err error) {
	if len(args) > 0 {
		c.secretUri, err = secrets.ParseURI(args[0])
		if err != nil {
			return errors.NotValidf("secret URI %q", args[0])
		}
		args = args[1:]
	}

	if c.secretUri == nil && c.label == "" {
		return errors.New("require either a secret URI or label")
	}

	if c.peek && c.refresh {
		return errors.New("specify one of --peek or --refresh but not both")
	}
	if len(args) > 0 {
		c.key = args[0]
		return cmd.CheckEmpty(args[1:])
	}
	return cmd.CheckEmpty(args)
}

// Run implements cmd.Command.
func (c *secretGetCommand) Run(ctx *cmd.Context) error {
	value, err := c.ctx.GetSecret(ctx, c.secretUri, c.label, c.refresh, c.peek)
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
