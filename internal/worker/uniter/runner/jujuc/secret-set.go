// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"github.com/juju/cmd/v4"
	"github.com/juju/errors"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/core/secrets"
)

type secretUpdateCommand struct {
	secretUpsertCommand

	secretURI *secrets.URI
}

// NewSecretSetCommand returns a command to create a secret.
func NewSecretSetCommand(ctx Context) (cmd.Command, error) {
	return &secretUpdateCommand{
		secretUpsertCommand: secretUpsertCommand{ctx: ctx},
	}, nil
}

// Info implements cmd.Command.
func (c *secretUpdateCommand) Info() *cmd.Info {
	doc := `
Update a secret with a list of key values, or set new metadata.
If a value has the '#base64' suffix, it is already in base64 format and no
encoding will be performed, otherwise the value will be base64 encoded
prior to being stored.
To just update selected metadata like rotate policy, do not specify any secret value.
`
	examples := `
    secret-set secret:9m4e2mr0ui3e8a215n4g token=34ae35facd4
    secret-set secret:9m4e2mr0ui3e8a215n4g key#base64 AA==
    secret-set secret:9m4e2mr0ui3e8a215n4g --rotate monthly token=s3cret 
    secret-set secret:9m4e2mr0ui3e8a215n4g --expire 24h
    secret-set secret:9m4e2mr0ui3e8a215n4g --expire 24h token=s3cret 
    secret-set secret:9m4e2mr0ui3e8a215n4g --expire 2025-01-01T06:06:06 token=s3cret 
    secret-set secret:9m4e2mr0ui3e8a215n4g --label db-password \
        --description "my database password" \
        data#base64 s3cret== 
    secret-set secret:9m4e2mr0ui3e8a215n4g --label db-password \
        --description "my database password"
    secret-set secret:9m4e2mr0ui3e8a215n4g --label db-password \
        --description "my database password" \
        --file=/path/to/file
`
	return jujucmd.Info(&cmd.Info{
		Name:     "secret-set",
		Args:     "<ID> [key[#base64]=value...]",
		Purpose:  "Update an existing secret.",
		Doc:      doc,
		Examples: examples,
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
