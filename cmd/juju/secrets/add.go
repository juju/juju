// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"fmt"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"

	apisecrets "github.com/juju/juju/api/client/secrets"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
)

type addSecretCommand struct {
	modelcmd.ModelCommandBase

	SecretUpsertContentCommand
	name           string
	secretsAPIFunc func() (AddSecretsAPI, error)
}

// AddSecretsAPI is the secrets client API.
type AddSecretsAPI interface {
	CreateSecret(name, description string, data map[string]string) (string, error)
	Close() error
}

// NewAddSecretCommand returns a command to add a secret.
func NewAddSecretCommand() cmd.Command {
	c := &addSecretCommand{}
	c.secretsAPIFunc = c.secretsAPI
	return modelcmd.Wrap(c)
}

func (c *addSecretCommand) secretsAPI() (AddSecretsAPI, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return apisecrets.NewClient(root), nil
}

const (
	addSecretDoc = `
Add a secret with a list of key values.

If a key has the ` + "`#base64` " + `suffix, the value is already in base64 format and no
encoding will be performed, otherwise the value will be base64 encoded
prior to being stored.

If a key has the ` + "`#file` " + `suffix, the value is read from the corresponding file.

A secret is owned by the model, meaning only the model admin
can manage it, ie grant/revoke access, update, remove etc.
`
	addSecretExamples = `
    juju add-secret my-apitoken token=34ae35facd4
    juju add-secret my-secret key#base64=AA==
    juju add-secret my-secret key#file=/path/to/file another-key=s3cret
    juju add-secret db-password \
        --info "my database password" \
        data#base64=s3cret==
    juju add-secret db-password \
        --info "my database password" \
        --file=/path/to/file
`
)

// Info implements cmd.Command.
func (c *addSecretCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "add-secret",
		Args:     "<name> [key[#base64|#file]=value...]",
		Purpose:  "Add a new secret.",
		Doc:      addSecretDoc,
		Examples: addSecretExamples,
	})
}

// Init implements cmd.Command.
func (c *addSecretCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.New("secret name needs to be supplied as the first argument")
	}
	c.name = args[0]
	args = args[1:]
	if err := c.SecretUpsertContentCommand.Init(args); err != nil {
		return err
	}
	if len(c.Data) == 0 {
		return errors.New("missing secret value or filename")
	}
	return nil
}

// Run implements cmd.Command.
func (c *addSecretCommand) Run(ctx *cmd.Context) error {
	secretsAPI, err := c.secretsAPIFunc()
	if err != nil {
		return errors.Trace(err)
	}
	defer secretsAPI.Close()

	uri, err := secretsAPI.CreateSecret(c.name, c.Description, c.Data)
	if err != nil {
		return err
	}
	fmt.Fprintln(ctx.Stdout, uri)
	return nil
}
