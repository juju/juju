// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"fmt"
	"time"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/core/secrets"
)

type secretUpdateCommand struct {
	cmd.CommandBase
	ctx Context

	uri            string
	asBase64       bool
	rotateInterval time.Duration
	description    string
	tags           map[string]string
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
    secret-update secret:9m4e2mr0ui3e8a215n4g 34ae35facd4
    secret-update secret:9m4e2mr0ui3e8a215n4g s3ke3t --staged
    secret-update --base64 secret:9m4e2mr0ui3e8a215n4g AA==
    secret-update --rotate 24h secret:9m4e2mr0ui3e8a215n4g s3cret
    secret-update --rotate 48h secret:9m4e2mr0ui3e8a215n4g
    secret-update --tag foo=bar --tag hello=world \
        --description "my database secret:9m4e2mr0ui3e8a215n4g"
`
	return jujucmd.Info(&cmd.Info{
		Name:    "secret-update",
		Args:    "<ID> [value|key=value...]",
		Purpose: "update an existing secret",
		Doc:     doc,
	})
}

// SetFlags implements cmd.Command.
func (c *secretUpdateCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.asBase64, "base64", false,
		`specify the supplied values are base64 encoded strings`)
	f.DurationVar(&c.rotateInterval, "rotate", -1, "how often the secret should be rotated")
	f.StringVar(&c.description, "description", "", "the secret description")
	f.Var(cmd.StringMap{&c.tags}, "tag", "tag to apply to the secret")
}

// Init implements cmd.Command.
func (c *secretUpdateCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.New("missing secret URI")
	}
	if c.rotateInterval < -1 {
		return errors.NotValidf("rotate interval %q", c.rotateInterval)
	}
	c.uri = args[0]
	if _, err := secrets.ParseURI(c.uri); err != nil {
		return errors.NewNotValid(err, fmt.Sprintf("secret URI %q not valid", c.uri))
	}

	var err error
	if len(args) > 1 {
		c.data, err = secrets.CreatSecretData(c.asBase64, args[1:])
	}
	return err
}

// Run implements cmd.Command.
func (c *secretUpdateCommand) Run(ctx *cmd.Context) error {
	value := secrets.NewSecretValue(c.data)
	args := SecretUpsertArgs{
		Value: value,
		Tags:  &c.tags,
	}
	if c.rotateInterval >= 0 {
		args.RotateInterval = &c.rotateInterval
	}
	if c.description != "" {
		args.Description = &c.description
	}
	return c.ctx.UpdateSecret(c.uri, &args)
}
