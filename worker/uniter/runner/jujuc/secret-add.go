// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"fmt"
	"time"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/core/secrets"

	jujucmd "github.com/juju/juju/cmd"
)

type secretAddCommand struct {
	cmd.CommandBase
	ctx Context

	asBase64       bool
	rotateInterval time.Duration
	description    string
	tags           map[string]string
	data           map[string]string
}

// NewSecretAddCommand returns a command to add a secret.
func NewSecretAddCommand(ctx Context) (cmd.Command, error) {
	return &secretAddCommand{
		ctx:            ctx,
		rotateInterval: -1,
	}, nil
}

// Info implements cmd.Command.
func (c *secretAddCommand) Info() *cmd.Info {
	doc := `
Add a secret with either a single value or a list of key values.
If --base64 is specified, the values are already in base64 format and no
encoding will be performed, otherwise the values will be base64 encoded
prior to being stored.

Examples:
    secret-add 34ae35facd4
    secret-add --base64 AA==
    secret-add --rotate 24h s3cret 
    secret-add --tag foo=bar --tag hello=world \
        --description "my database password" \
        s3cret 
`
	return jujucmd.Info(&cmd.Info{
		Name:    "secret-add",
		Args:    "[value|key=value...]",
		Purpose: "add a new secret",
		Doc:     doc,
	})
}

// SetFlags implements cmd.Command.
func (c *secretAddCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.asBase64, "base64", false,
		`specify the supplied values are base64 encoded strings`)
	f.DurationVar(&c.rotateInterval, "rotate", 0, "how often the secret should be rotated")
	f.StringVar(&c.description, "description", "", "the secret description")
	f.Var(cmd.StringMap{&c.tags}, "tag", "tag to apply to the secret")
}

// Init implements cmd.Command.
func (c *secretAddCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.New("missing secret value")
	}
	if c.rotateInterval < 0 {
		return errors.NotValidf("rotate interval %q", c.rotateInterval)
	}

	var err error
	c.data, err = secrets.CreatSecretData(c.asBase64, args)
	return err
}

// Run implements cmd.Command.
func (c *secretAddCommand) Run(ctx *cmd.Context) error {
	value := secrets.NewSecretValue(c.data)
	id, err := c.ctx.CreateSecret(&SecretUpsertArgs{
		Value:          value,
		RotateInterval: &c.rotateInterval,
		Description:    &c.description,
		Tags:           &c.tags,
	})
	if err != nil {
		return err
	}
	fmt.Fprintln(ctx.Stdout, id)
	return nil
}
