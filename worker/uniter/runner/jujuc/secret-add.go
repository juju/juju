// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"fmt"
	"time"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v4"

	jujucmd "github.com/juju/juju/cmd"
	secretscmd "github.com/juju/juju/cmd/juju/secrets"
	"github.com/juju/juju/core/secrets"
)

type secretUpsertCommand struct {
	cmd.CommandBase
	ctx Context

	owner        string
	rotatePolicy string

	expireSpec string
	expireTime time.Time

	secretscmd.SecretUpsertContentCommand
}

type secretAddCommand struct {
	secretUpsertCommand
}

// NewSecretAddCommand returns a command to add a secret.
func NewSecretAddCommand(ctx Context) (cmd.Command, error) {
	return &secretAddCommand{
		secretUpsertCommand{ctx: ctx},
	}, nil
}

// Info implements cmd.Command.
func (c *secretAddCommand) Info() *cmd.Info {
	doc := `
Add a secret with a list of key values.

If a key has the '#base64' suffix, the value is already in base64 format and no
encoding will be performed, otherwise the value will be base64 encoded
prior to being stored.

If a key has the '#file' suffix, the value is read from the corresponding file.

By default, a secret is owned by the application, meaning only the unit
leader can manage it. Use "--owner unit" to create a secret owned by the
specific unit which created it.

Examples:
    secret-add token=34ae35facd4
    secret-add key#base64=AA==
    secret-add key#file=/path/to/file another-key=s3cret
    secret-add --owner unit token=s3cret 
    secret-add --rotate monthly token=s3cret 
    secret-add --expire 24h token=s3cret 
    secret-add --expire 2025-01-01T06:06:06 token=s3cret 
    secret-add --label db-password \
        --description "my database password" \
        data#base64=s3cret== 
    secret-add --label db-password \
        --description "my database password" \
        --file=/path/to/file
`
	return jujucmd.Info(&cmd.Info{
		Name:    "secret-add",
		Args:    "[key[#base64|#file]=value...]",
		Purpose: "add a new secret",
		Doc:     doc,
	})
}

// SetFlags implements cmd.Command.
func (c *secretUpsertCommand) SetFlags(f *gnuflag.FlagSet) {
	c.SecretUpsertContentCommand.SetFlags(f)
	f.StringVar(&c.expireSpec, "expire", "", "either a duration or time when the secret should expire")
	f.StringVar(&c.rotatePolicy, "rotate", "", "the secret rotation policy")
	f.StringVar(&c.owner, "owner", "application", "the owner of the secret, either the application or unit")
}

const rcf3339NoTZ = "2006-01-02T15:04:05"

// Init implements cmd.Command.
func (c *secretUpsertCommand) Init(args []string) error {
	if c.expireSpec != "" {
		expireTime, err := time.Parse(time.RFC3339, c.expireSpec)
		if err != nil {
			expireTime, err = time.Parse(rcf3339NoTZ, c.expireSpec)
		}
		if err != nil {
			d, err := time.ParseDuration(c.expireSpec)
			if err != nil {
				return errors.NotValidf("expire time or duration %q", c.expireSpec)
			}
			if d <= 0 {
				return errors.NotValidf("negative expire duration %q", c.expireSpec)
			}
			expireTime = time.Now().Add(d)
		}
		c.expireTime = expireTime.UTC()
	}
	if c.rotatePolicy != "" && !secrets.RotatePolicy(c.rotatePolicy).IsValid() {
		return errors.NotValidf("rotate policy %q", c.rotatePolicy)
	}
	if c.owner != "application" && c.owner != "unit" {
		return errors.NotValidf("secret owner %q", c.owner)
	}

	return c.SecretUpsertContentCommand.Init(args)
}

func (c *secretUpsertCommand) marshallArg() *SecretUpdateArgs {
	value := secrets.NewSecretValue(c.Data)
	arg := &SecretUpdateArgs{
		Value: value,
	}
	if c.rotatePolicy != "" {
		p := secrets.RotatePolicy(c.rotatePolicy)
		arg.RotatePolicy = &p
	}
	if !c.expireTime.IsZero() {
		arg.ExpireTime = &c.expireTime
	}
	if c.Description != "" {
		arg.Description = &c.Description
	}
	if c.Label != "" {
		arg.Label = &c.Label
	}
	return arg
}

// Init implements cmd.Command.
func (c *secretAddCommand) Init(args []string) error {
	if len(args) < 1 && c.FileName == "" {
		return errors.New("missing secret value or filename")
	}
	return c.secretUpsertCommand.Init(args)
}

// Run implements cmd.Command.
func (c *secretAddCommand) Run(ctx *cmd.Context) error {
	unitName := c.ctx.UnitName()
	var ownerTag names.Tag
	appName, _ := names.UnitApplication(unitName)
	ownerTag = names.NewApplicationTag(appName)
	if c.owner == "unit" {
		ownerTag = names.NewUnitTag(unitName)
	}
	updateArgs := c.marshallArg()
	if updateArgs.Value.IsEmpty() {
		return errors.NotValidf("empty secret value")
	}
	arg := &SecretCreateArgs{
		SecretUpdateArgs: *updateArgs,
		OwnerTag:         ownerTag,
	}
	uri, err := c.ctx.CreateSecret(arg)
	if err != nil {
		return err
	}
	fmt.Fprintln(ctx.Stdout, uri.String())
	return nil
}
