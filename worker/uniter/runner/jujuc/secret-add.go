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

type secretUpsertCommand struct {
	cmd.CommandBase
	ctx Context

	rotatePolicy string
	description  string
	label        string
	fileName     string

	expireSpec string
	expireTime time.Time

	data map[string]string
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
If a value has the '#base64' suffix, it is already in base64 format and no
encoding will be performed, otherwise the value will be base64 encoded
prior to being stored.

Examples:
    secret-add token=34ae35facd4
    secret-add key#base64 AA==
    secret-add --rotate monthly token=s3cret 
    secret-add --expire 24h token=s3cret 
    secret-add --expire 2025-01-01T06:06:06 token=s3cret 
    secret-add --label db-password \
        --description "my database password" \
        data#base64 s3cret== 
    secret-add --label db-password \
        --description "my database password" \
        --file=/path/to/file
`
	return jujucmd.Info(&cmd.Info{
		Name:    "secret-add",
		Args:    "[key[#base64]=value...]",
		Purpose: "add a new secret",
		Doc:     doc,
	})
}

// SetFlags implements cmd.Command.
func (c *secretUpsertCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.expireSpec, "expire", "", "either a duration or time when the secret should expire")
	f.StringVar(&c.rotatePolicy, "rotate", "", "the secret rotation policy")
	f.StringVar(&c.description, "description", "", "the secret description")
	f.StringVar(&c.label, "label", "", "a label used to identify the secret in hooks")
	f.StringVar(&c.fileName, "file", "", "a YAML file containing secret key values")
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
	var err error
	c.data, err = secrets.CreateSecretData(args)
	if err != nil || c.fileName == "" {
		return errors.Trace(err)
	}
	dataFromFile, err := secrets.ReadSecretData(c.fileName)
	if err != nil {
		return errors.Trace(err)
	}
	for k, v := range dataFromFile {
		c.data[k] = v
	}
	return nil
}

func (c *secretUpsertCommand) marshallArg() *SecretUpsertArgs {
	value := secrets.NewSecretValue(c.data)
	arg := &SecretUpsertArgs{
		Value: value,
	}
	if c.rotatePolicy != "" {
		p := secrets.RotatePolicy(c.rotatePolicy)
		arg.RotatePolicy = &p
	}
	if !c.expireTime.IsZero() {
		arg.ExpireTime = &c.expireTime
	}
	if c.description != "" {
		arg.Description = &c.description
	}
	if c.label != "" {
		arg.Label = &c.label
	}
	return arg
}

// Init implements cmd.Command.
func (c *secretAddCommand) Init(args []string) error {
	if len(args) < 1 && c.fileName == "" {
		return errors.New("missing secret value or filename")
	}
	return c.secretUpsertCommand.Init(args)
}

// Run implements cmd.Command.
func (c *secretAddCommand) Run(ctx *cmd.Context) error {
	id, err := c.ctx.CreateSecret(c.marshallArg())
	if err != nil {
		return err
	}
	fmt.Fprintln(ctx.Stdout, id)
	return nil
}
