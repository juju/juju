// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for infos.

package model

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/modelmanager"
	"github.com/juju/juju/apiserver/params"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
)

var logger = loggo.GetLogger("juju.cmd.juju.model")

// NewDestroyCommand returns a command used to destroy a model.
func NewDestroyCommand() cmd.Command {
	return modelcmd.Wrap(
		&destroyCommand{},
		modelcmd.WrapSkipDefaultModel,
		modelcmd.WrapSkipModelFlags,
	)
}

// destroyCommand destroys the specified model.
type destroyCommand struct {
	modelcmd.ModelCommandBase
	envName   string
	assumeYes bool
	api       DestroyModelAPI
}

var destroyDoc = `
Destroys the specified model. This will result in the non-recoverable
removal of all the units operating in the model and any resources stored
there. Due to the irreversible nature of the command, it will prompt for
confirmation (unless overridden with the '-y' option) before taking any
action.

Examples:

    juju destroy-model test
    juju destroy-model -y mymodel

See also:
    destroy-controller
`
var destroyEnvMsg = `
WARNING! This command will destroy the %q model.
This includes all machines, applications, data and other resources.

Continue [y/N]? `[1:]

// DestroyModelAPI defines the methods on the modelmanager
// API that the destroy command calls. It is exported for mocking in tests.
type DestroyModelAPI interface {
	Close() error
	DestroyModel(names.ModelTag) error
}

// Info implements Command.Info.
func (c *destroyCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "destroy-model",
		Args:    "[<controller name>:]<model name>",
		Purpose: "Terminate all machines and resources for a non-controller model.",
		Doc:     destroyDoc,
	}
}

// SetFlags implements Command.SetFlags.
func (c *destroyCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.BoolVar(&c.assumeYes, "y", false, "Do not prompt for confirmation")
	f.BoolVar(&c.assumeYes, "yes", false, "")
}

// Init implements Command.Init.
func (c *destroyCommand) Init(args []string) error {
	switch len(args) {
	case 0:
		return errors.New("no model specified")
	case 1:
		return c.SetModelName(args[0])
	default:
		return cmd.CheckEmpty(args[1:])
	}
}

func (c *destroyCommand) getAPI() (DestroyModelAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	root, err := c.NewControllerAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return modelmanager.NewClient(root), nil
}

// Run implements Command.Run
func (c *destroyCommand) Run(ctx *cmd.Context) error {
	store := c.ClientStore()
	controllerName := c.ControllerName()
	modelName := c.ModelName()

	controllerDetails, err := store.ControllerByName(controllerName)
	if err != nil {
		return errors.Annotate(err, "cannot read controller details")
	}
	modelDetails, err := store.ModelByName(controllerName, modelName)
	if err != nil {
		return errors.Annotate(err, "cannot read model info")
	}
	if modelDetails.ModelUUID == controllerDetails.ControllerUUID {
		return errors.Errorf("%q is a controller; use 'juju destroy-controller' to destroy it", modelName)
	}

	if !c.assumeYes {
		fmt.Fprintf(ctx.Stdout, destroyEnvMsg, modelName)

		if err := jujucmd.UserConfirmYes(ctx); err != nil {
			return errors.Annotate(err, "model destruction")
		}
	}

	// Attempt to connect to the API.  If we can't, fail the destroy.
	api, err := c.getAPI()
	if err != nil {
		return errors.Annotate(err, "cannot connect to API")
	}
	defer api.Close()

	// Attempt to destroy the model.
	err = api.DestroyModel(names.NewModelTag(modelDetails.ModelUUID))
	if err != nil {
		return c.handleError(errors.Annotate(err, "cannot destroy model"), modelName)
	}

	err = store.RemoveModel(controllerName, modelName)
	if err != nil && !errors.IsNotFound(err) {
		return errors.Trace(err)
	}
	return nil
}

func (c *destroyCommand) handleError(err error, modelName string) error {
	if err == nil {
		return nil
	}
	if params.IsCodeOperationBlocked(err) {
		return block.ProcessBlockedError(err, block.BlockDestroy)
	}
	logger.Errorf(`failed to destroy model %q`, modelName)
	return err
}
