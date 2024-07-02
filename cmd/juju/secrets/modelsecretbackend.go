// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"context"
	"fmt"

	"github.com/juju/cmd/v4"
	"github.com/juju/errors"

	api "github.com/juju/juju/api/client/secrets"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
)

type modelSecretBackendCommand struct {
	modelcmd.ModelCommandBase

	getAPIFunc func() (ModelSecretBackendAPI, error)

	secretBackendName *string
}

// ModelSecretBackendAPI is the mdoel secret backend client API.
type ModelSecretBackendAPI interface {
	GetModelSecretBackend(ctx context.Context) (string, error)
	SetModelSecretBackend(ctx context.Context, secretBackendName string) error
	Close() error
}

// NewmoMelSecretBackendCommand returns a command to get or set secret backend config for the model.
func NewModelSecretBackendCommand() cmd.Command {
	c := &modelSecretBackendCommand{}
	c.getAPIFunc = c.secretBackendAPI
	return modelcmd.Wrap(c)
}

func (c *modelSecretBackendCommand) secretBackendAPI() (ModelSecretBackendAPI, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return api.NewClient(root), nil
}

const (
	modelSecretBackendDoc = `
Sets or displays the secret backend config for the current model.
`
	modelSecretBackendExamples = `
   juju model-secret-backend
   juju model-secret-backend myVault
`
)

// Info implements cmd.Command.
func (c *modelSecretBackendCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "model-secret-backend",
		Args:     "<secret-backend-name>",
		Purpose:  "Displays or sets the secret backend on a model.",
		Doc:      modelSecretBackendDoc,
		Examples: modelSecretBackendExamples,
		SeeAlso: []string{
			"add-secret-backend",
			"secret-backends",
			"list-secret-backends",
			"remove-secret-backend",
			"show-secret-backend",
			"update-secret-backend",
		},
	})
}

// Init implements cmd.Command.
func (c *modelSecretBackendCommand) Init(args []string) error {
	if len(args) == 0 {
		return nil
	}
	if len(args) != 1 {
		return errors.New("cannot specify multiple secret backends")
	}
	c.secretBackendName = &args[0]
	return nil
}

// Run implements cmd.Command.
func (c *modelSecretBackendCommand) Run(ctx *cmd.Context) error {
	api, err := c.getAPIFunc()
	if err != nil {
		return errors.Trace(err)
	}
	defer func() { _ = api.Close() }()

	if c.secretBackendName == nil {
		secretBackendName, err := api.GetModelSecretBackend(ctx.Context)
		if err != nil {
			return errors.Trace(err)
		}
		fmt.Fprintln(ctx.Stdout, secretBackendName)
		return nil
	}
	return api.SetModelSecretBackend(ctx.Context, *c.secretBackendName)
}
