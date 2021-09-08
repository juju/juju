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

type secretUpdateCommand struct {
	cmd.CommandBase
	ctx Context

	id             string
	asBase64       bool
	rotateInterval time.Duration
	data           map[string]string
}

// NewSecretUpdateCommand returns a command to create a secret.
func NewSecretUpdateCommand(ctx Context) (cmd.Command, error) {
	return &secretUpdateCommand{ctx: ctx, rotateInterval: -1}, nil
}

// Info implements cmd.Command.
func (c *secretUpdateCommand) Info() *cmd.Info {
	doc := `
Update a secret with either a single value or a list of key values.
If --base64 is specified, the values are already in base64 format and no
encoding will be performed, otherwise the values will be base64 encoded
prior to being stored.
To just update the rotate interval, do not specify any secret value.
	
Examples:
    secret-update apitoken 34ae35facd4
    secret-update --base64 password AA==
    secret-update --rotate 5d password s3cret
    secret-update --rotate 10d password
`
	return jujucmd.Info(&cmd.Info{
		Name:    "secret-update",
		Args:    "<id> [value|key=value...]",
		Purpose: "update an existing secret",
		Doc:     doc,
	})
}

// SetFlags implements cmd.Command.
func (c *secretUpdateCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.asBase64, "base64", false,
		`specify the supplied values are base64 encoded strings`)
	f.DurationVar(&c.rotateInterval, "rotate", -1, "how often the secret should be rotated")
}

// Init implements cmd.Command.
func (c *secretUpdateCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.New("missing secret id")
	}
	if c.rotateInterval < -1 {
		return errors.NotValidf("rotate interval %q", c.rotateInterval)
	}
	c.id = args[0]

	var err error
	if len(args) > 1 {
		c.data, err = secrets.CreatSecretData(c.asBase64, args[1:])
	}
	return err
}

// Run implements cmd.Command.
func (c *secretUpdateCommand) Run(ctx *cmd.Context) error {
	value := secrets.NewSecretValue(c.data)
	id, err := c.ctx.UpdateSecret(c.id, &UpsertArgs{
		Value:          value,
		RotateInterval: c.rotateInterval,
	})
	if err != nil {
		return err
	}
	fmt.Fprintln(ctx.Stdout, id)
	return nil
}
