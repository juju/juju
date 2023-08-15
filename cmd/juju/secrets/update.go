// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	apisecrets "github.com/juju/juju/api/client/secrets"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/secrets"
)

type updateSecretCommand struct {
	modelcmd.ModelCommandBase

	SecretUpsertContentCommand
	secretsAPIFunc func() (UpdateSecretsAPI, error)

	secretURI *secrets.URI
	autoPrune bool
}

// UpdateSecretsAPI is the secrets client API.
type UpdateSecretsAPI interface {
	UpdateSecret(
		uri *secrets.URI, autoPrune *bool,
		label, description string, data map[string]string,
	) error
	Close() error
}

// NewUpdateSecretCommand returns a command to update a secret.
func NewUpdateSecretCommand() cmd.Command {
	c := &updateSecretCommand{}
	c.secretsAPIFunc = c.secretsAPI
	return modelcmd.Wrap(c)
}

func (c *updateSecretCommand) secretsAPI() (UpdateSecretsAPI, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return apisecrets.NewClient(root), nil
}

// Info implements cmd.Command.
func (c *updateSecretCommand) Info() *cmd.Info {
	doc := `
Update a secret with a list of key values, or info.
If a value has the '#base64' suffix, it is already in base64 format and no
encoding will be performed, otherwise the value will be base64 encoded
prior to being stored.
The --auto-prune option is used to allow Juju to automatically remove revisions 
which are no longer being tracked by any observers (see Rotation and Expiry).
This is configured per revision. This feature is opt-in because Juju 
automatically removing secret content might result in data loss.

Examples:
    update-secret secret:9m4e2mr0ui3e8a215n4g token=34ae35facd4

    update-secret secret:9m4e2mr0ui3e8a215n4g key#base64 AA==

    update-secret secret:9m4e2mr0ui3e8a215n4g token=34ae35facd4 --auto-prune

    update-secret secret:9m4e2mr0ui3e8a215n4g --label db-password \
        --info "my database password" \
        data#base64 s3cret== 

    update-secret secret:9m4e2mr0ui3e8a215n4g --label db-password \
        --info "my database password"

    update-secret secret:9m4e2mr0ui3e8a215n4g --label db-password \
        --info "my database password" \
        --file=/path/to/file
`
	return jujucmd.Info(&cmd.Info{
		Name:    "update-secret",
		Args:    "<ID> [key[#base64|#file]=value...]",
		Purpose: "Update an existing secret.",
		Doc:     doc,
	})
}

// Init implements cmd.Command.
func (c *updateSecretCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.New("missing secret URI")
	}
	var err error
	if c.secretURI, err = secrets.ParseURI(args[0]); err != nil {
		return errors.Trace(err)
	}
	return c.SecretUpsertContentCommand.Init(args[1:])
}

func (c *updateSecretCommand) SetFlags(f *gnuflag.FlagSet) {
	c.SecretUpsertContentCommand.SetFlags(f)
	f.BoolVar(
		&c.autoPrune, "auto-prune", false,
		"used to allow Juju to automatically remove revisions which are no longer being tracked by any observers",
	)
}

// Run implements cmd.Command.
func (c *updateSecretCommand) Run(ctx *cmd.Context) error {
	secretsAPI, err := c.secretsAPIFunc()
	if err != nil {
		return errors.Trace(err)
	}
	defer secretsAPI.Close()
	return secretsAPI.UpdateSecret(c.secretURI, &c.autoPrune, c.Label, c.Description, c.Data)
}
