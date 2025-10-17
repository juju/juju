// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackends

import (
	"context"
	"fmt"

	"github.com/juju/errors"

	api "github.com/juju/juju/api/client/modelconfig"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	secretbackenderrors "github.com/juju/juju/domain/secretbackend/errors"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/secrets/provider"
)

type modelSecretBackendCommand struct {
	modelcmd.ModelCommandBase

	getAPIFunc func(ctx context.Context) (ModelSecretBackendAPI, error)

	secretBackendName *string
}

// ModelSecretBackendAPI is the mdoel secret backend client API.
type ModelSecretBackendAPI interface {
	GetModelSecretBackend(ctx context.Context) (string, error)
	SetModelSecretBackend(ctx context.Context, secretBackendName string) error
	Close() error
}

// NewModelSecretBackendCommand returns a command to get or set secret backend config for the current model.
func NewModelSecretBackendCommand() cmd.Command {
	c := &modelSecretBackendCommand{}
	c.getAPIFunc = c.secretBackendAPI
	return modelcmd.Wrap(c)
}

func (c *modelSecretBackendCommand) secretBackendAPI(ctx context.Context) (ModelSecretBackendAPI, error) {
	root, err := c.NewAPIRoot(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return api.NewClient(root), nil
}

const (
	modelSecretBackendDoc = `
Sets or displays the secret backend for the current model.
`
	modelSecretBackendExamples = `
Display the secret backend for the current model:

    juju model-secret-backend

Set the secret backend to myVault for the current model:

    juju model-secret-backend myVault
`
)

// Info implements cmd.Command.
func (c *modelSecretBackendCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "model-secret-backend",
		Args:     "[<secret-backend-name>]",
		Purpose:  "Displays or sets the secret backend for a model.",
		Doc:      modelSecretBackendDoc,
		Examples: modelSecretBackendExamples,
		SeeAlso: []string{
			"add-secret-backend",
			"secret-backends",
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
		return errors.New("cannot specify multiple secret backend names")
	}
	if args[0] == "" {
		return errors.New("cannot specify an empty secret backend name")
	}
	c.secretBackendName = &args[0]
	return nil
}

// Run implements cmd.Command.
func (c *modelSecretBackendCommand) Run(ctx *cmd.Context) error {
	api, err := c.getAPIFunc(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	defer func() { _ = api.Close() }()

	if c.secretBackendName == nil {
		secretBackendName, err := api.GetModelSecretBackend(ctx.Context)
		if errors.Is(err, errors.NotSupported) {
			return modelSecretBackendNotSupportedError
		}
		if err != nil {
			return errors.Trace(err)
		}
		fmt.Fprintln(ctx.Stdout, secretBackendName)
		return nil
	}
	err = api.SetModelSecretBackend(ctx.Context, *c.secretBackendName)
	if errors.Is(err, errors.NotSupported) {
		return modelSecretBackendNotSupportedError
	} else if errors.Is(err, secretbackenderrors.NotFound) {
		return fmt.Errorf(`%w: please use "add-secret-backend" to add %q to the controller first`, err, *c.secretBackendName)
	} else if errors.Is(err, secretbackenderrors.NotValid) {
		return fmt.Errorf(`%w: please use %q instead`, err, provider.Auto)
	}
	return errors.Trace(err)
}

const modelSecretBackendNotSupportedError = errors.ConstError(
	`"model-secret-backend" has not been implemented on the controller, use the "model-config" command instead`,
)
