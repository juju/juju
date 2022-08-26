// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/errors"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/core/secrets"
)

type secretUpdateCommand struct {
	secretUpsertCommand

	secretURI *secrets.URI
}

// NewSecretUpdateCommand returns a command to create a secret.
func NewSecretUpdateCommand(ctx Context) (cmd.Command, error) {
	return &secretUpdateCommand{
		secretUpsertCommand: secretUpsertCommand{ctx: ctx},
	}, nil
}

// Info implements cmd.Command.
func (c *secretUpdateCommand) Info() *cmd.Info {
	doc := `
Update a secret with a list of key values.
If a value has the '#base64' suffix, it is already in base64 format and no
encoding will be performed, otherwise the value will be base64 encoded
prior to being stored.
To just update selected metadata like rotate policy, do not specify any secret value.

Examples:
    secret-update secret:9m4e2mr0ui3e8a215n4g token=34ae35facd4
    secret-update secret:9m4e2mr0ui3e8a215n4g key#base64 AA==
    secret-update secret:9m4e2mr0ui3e8a215n4g --rotate monthly token=s3cret 
    secret-update secret:9m4e2mr0ui3e8a215n4g --expire 24h
    secret-update secret:9m4e2mr0ui3e8a215n4g --expire 24h token=s3cret 
    secret-update secret:9m4e2mr0ui3e8a215n4g --expire 2025-01-01T06:06:06 token=s3cret 
    secret-update secret:9m4e2mr0ui3e8a215n4g --label db-password \
        --description "my database password" \
        data#base64 s3cret== 
    secret-update secret:9m4e2mr0ui3e8a215n4g --label db-password \
        --description "my database password"
    secret-update secret:9m4e2mr0ui3e8a215n4g --label db-password \
        --description "my database password" \
        --file=/path/to/file
`
	return jujucmd.Info(&cmd.Info{
		Name:    "secret-update",
		Args:    "<ID> [key[#base64]=value...]",
		Purpose: "update an existing secret",
		Doc:     doc,
	})
}

// Init implements cmd.Command.
func (c *secretUpdateCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.New("missing secret URI")
	}
	var err error
	if c.secretURI, err = secrets.ParseURI(args[0]); err != nil {
		return errors.Trace(err)
	}
	return c.secretUpsertCommand.Init(args[1:])
}

// Run implements cmd.Command.
func (c *secretUpdateCommand) Run(ctx *cmd.Context) error {
	return c.ctx.UpdateSecret(c.secretURI, c.marshallArg())
}
