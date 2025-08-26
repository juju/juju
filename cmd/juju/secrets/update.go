// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	apisecrets "github.com/juju/juju/api/client/secrets"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/secrets"
)

type updateSecretCommand struct {
	modelcmd.ModelCommandBase

	SecretUpsertContentCommand
	secretsAPIFunc func() (UpdateSecretsAPI, error)

	secretURI *secrets.URI
	autoPrune common.AutoBoolValue

	name    string
	newName string
}

// UpdateSecretsAPI is the secrets client API.
type UpdateSecretsAPI interface {
	UpdateSecret(
		uri *secrets.URI, name string, autoPrune *bool,
		newName, description string, data map[string]string,
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

const (
	updateSecretDoc = `
Update a secret with a list of key values, or info.

If a value has the ` + "`#base64`" + ` suffix, it is already in ` + "`base64`" + ` format and no
encoding will be performed, otherwise the value will be base64 encoded
prior to being stored.

The ` + "`--auto-prune`" + ` option is used to allow Juju to automatically remove revisions
which are no longer being tracked by any observers (see Rotation and Expiry).
This is configured per revision. This feature is opt-in because Juju
automatically removing secret content might result in data loss.

`
	updateSecretExamples = `
    juju update-secret secret:9m4e2mr0ui3e8a215n4g token=34ae35facd4
    juju update-secret secret:9m4e2mr0ui3e8a215n4g key#base64 AA==
    juju update-secret secret:9m4e2mr0ui3e8a215n4g token=34ae35facd4 --auto-prune
    juju update-secret secret:9m4e2mr0ui3e8a215n4g --name db-password \
        --info "my database password" \
        data#base64 s3cret==
    juju update-secret db-pass --name db-password \
        --info "my database password"
    juju update-secret secret:9m4e2mr0ui3e8a215n4g --name db-password \
        --info "my database password" \
        --file=/path/to/file
`
)

// Info implements cmd.Command.
func (c *updateSecretCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "update-secret",
		Args:     "<ID>|<name> [key[#base64|#file]=value...]",
		Purpose:  "Update an existing secret.",
		Doc:      updateSecretDoc,
		Examples: updateSecretExamples,
	})
}

// Init implements cmd.Command.
func (c *updateSecretCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.New("missing secret URI")
	}
	var err error
	if c.secretURI, err = secrets.ParseURI(args[0]); err != nil {
		c.name = args[0]
	}
	return c.SecretUpsertContentCommand.Init(args[1:])
}

func (c *updateSecretCommand) SetFlags(f *gnuflag.FlagSet) {
	c.SecretUpsertContentCommand.SetFlags(f)
	f.StringVar(&c.newName, "name", "", "The new secret name")
	f.Var(&c.autoPrune, "auto-prune", "Used to allow Juju to automatically remove revisions which are no longer being tracked by any observers")
}

// Run implements cmd.Command.
func (c *updateSecretCommand) Run(ctx *cmd.Context) error {
	secretsAPI, err := c.secretsAPIFunc()
	if err != nil {
		return errors.Trace(err)
	}
	defer func() { _ = secretsAPI.Close() }()
	return secretsAPI.UpdateSecret(c.secretURI, c.name, c.autoPrune.Get(), c.newName, c.Description, c.Data)
}
