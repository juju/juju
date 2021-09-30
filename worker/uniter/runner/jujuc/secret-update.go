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
	active         bool
	staged         bool
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
    secret-update apitoken 34ae35facd4
    secret-update password s3ke3t --staged
    secret-update password --active
    secret-update --base64 password AA==
    secret-update --rotate 24h password s3cret
    secret-update --rotate 48h password
    secret-update --tag foo=bar --tag hello=world \
        --description "my database password"
    secret-update --tag foo=baz new-s3cret 
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
	f.BoolVar(&c.staged, "staged", false,
		"specify whether the secret should be staged rather than active")
	f.BoolVar(&c.active, "active", false,
		"update a staged secret to be active")
	f.DurationVar(&c.rotateInterval, "rotate", -1, "how often the secret should be rotated")
	f.StringVar(&c.description, "description", "", "the secret description")
	f.Var(cmd.StringMap{&c.tags}, "tag", "tag to apply to the secret")
}

// Init implements cmd.Command.
func (c *secretUpdateCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.New("missing secret id")
	}
	if c.rotateInterval < -1 {
		return errors.NotValidf("rotate interval %q", c.rotateInterval)
	}
	if c.staged && c.active {
		return errors.NotValidf("specifying both --staged and --active")
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
	status := secrets.StatusActive
	if c.staged {
		status = secrets.StatusStaged
	}
	args := UpsertArgs{
		Value:  value,
		Status: &status,
		Tags:   &c.tags,
	}
	if c.rotateInterval >= 0 {
		args.RotateInterval = &c.rotateInterval
	}
	if c.description != "" {
		args.Description = &c.description
	}
	id, err := c.ctx.UpdateSecret(c.id, &args)
	if err != nil {
		return err
	}
	fmt.Fprintln(ctx.Stdout, id)
	return nil
}
