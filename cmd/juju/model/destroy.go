// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for infos.

package model

import (
	"fmt"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"
	"github.com/juju/romulus/api/budget"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon-bakery.v1/httpbakery"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/modelconfig"
	"github.com/juju/juju/api/modelmanager"
	"github.com/juju/juju/apiserver/params"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
)

const (
	slaUnsupported = "unsupported"
)

var logger = loggo.GetLogger("juju.cmd.juju.model")

// NewDestroyCommand returns a command used to destroy a model.
func NewDestroyCommand() cmd.Command {
	destroyCmd := &destroyCommand{}
	destroyCmd.RefreshModels = destroyCmd.ModelCommandBase.RefreshModels
	destroyCmd.sleepFunc = time.Sleep
	return modelcmd.Wrap(
		destroyCmd,
		modelcmd.WrapSkipDefaultModel,
		modelcmd.WrapSkipModelFlags,
	)
}

// destroyCommand destroys the specified model.
type destroyCommand struct {
	modelcmd.ModelCommandBase
	// RefreshModels hides the RefreshModels function defined
	// in ModelCommandBase. This allows overriding for testing.
	// NOTE: ideal solution would be to have the base implement a method
	// like store.ModelByName which auto-refreshes.
	RefreshModels func(jujuclient.ClientStore, string) error

	// sleepFunc is used when calling the timed function to get model status updates.
	sleepFunc func(time.Duration)

	envName   string
	assumeYes bool
	api       DestroyModelAPI
	configApi ModelConfigAPI
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
	ModelStatus(models ...names.ModelTag) ([]base.ModelStatus, error)
}

// ModelConfigAPI defines the methods on the modelconfig
// API that the destroy command calls. It is exported for mocking in tests.
type ModelConfigAPI interface {
	Close() error
	SLALevel() (string, error)
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
		return c.SetModelName(args[0], false)
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

func (c *destroyCommand) getModelConfigAPI() (ModelConfigAPI, error) {
	if c.configApi != nil {
		return c.configApi, nil
	}
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return modelconfig.NewClient(root), nil
}

// Run implements Command.Run
func (c *destroyCommand) Run(ctx *cmd.Context) error {
	store := c.ClientStore()
	controllerName, err := c.ControllerName()
	if err != nil {
		return errors.Trace(err)
	}
	modelName, err := c.ModelName()
	if err != nil {
		return errors.Trace(err)
	}

	controllerDetails, err := store.ControllerByName(controllerName)
	if err != nil {
		return errors.Annotate(err, "cannot read controller details")
	}
	modelDetails, err := store.ModelByName(controllerName, modelName)
	if errors.IsNotFound(err) {
		if err := c.RefreshModels(store, controllerName); err != nil {
			return errors.Annotate(err, "refreshing models cache")
		}
		// Now try again.
		modelDetails, err = store.ModelByName(controllerName, modelName)
	}
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

	configApi, err := c.getModelConfigAPI()
	if err != nil {
		return errors.Annotate(err, "cannot connect to API")
	}
	defer configApi.Close()

	// Check if the model has an SLA set.
	slaIsSet := false
	slaLevel, err := configApi.SLALevel()
	if err == nil {
		slaIsSet = slaLevel != "" && slaLevel != slaUnsupported
	} else {
		ctx.Warningf("could not determine model SLA level: %v", err)
	}

	// Attempt to destroy the model.
	ctx.Infof("Destroying model")
	err = api.DestroyModel(names.NewModelTag(modelDetails.ModelUUID))
	if err != nil {
		return c.handleError(errors.Annotate(err, "cannot destroy model"), modelName)
	}

	// Wait for model to be destroyed.
	const modelStatusPollWait = 2 * time.Second
	modelStatus := newTimedModelStatus(ctx, api, names.NewModelTag(modelDetails.ModelUUID), c.sleepFunc)
	modelData := modelStatus(0)
	for modelData != nil {
		ctx.Infof(formatDestroyModelInfo(modelData) + "...")
		modelData = modelStatus(modelStatusPollWait)
	}

	err = store.RemoveModel(controllerName, modelName)
	if err != nil && !errors.IsNotFound(err) {
		return errors.Trace(err)
	}

	// Check if the model has an sla auth.
	if slaIsSet {
		err = c.removeModelBudget(modelDetails.ModelUUID)
		if err != nil {
			ctx.Warningf("model allocation not removed: %v", err)
		}
	}

	return nil
}

func (c *destroyCommand) removeModelBudget(uuid string) error {
	bakeryClient, err := c.BakeryClient()
	if err != nil {
		return errors.Trace(err)
	}

	budgetClient := getBudgetAPIClient(bakeryClient)

	resp, err := budgetClient.DeleteBudget(uuid)
	if err != nil {
		return errors.Trace(err)
	}
	if resp != "" {
		logger.Infof(resp)
	}
	return nil
}

type modelData struct {
	machineCount     int
	applicationCount int
}

// newTimedModelStatus returns a function which waits a given period of time
// before querying the API server for the status of a model.
func newTimedModelStatus(ctx *cmd.Context, api DestroyModelAPI, tag names.ModelTag, sleepFunc func(time.Duration)) func(time.Duration) *modelData {
	return func(wait time.Duration) *modelData {
		sleepFunc(wait)
		status, err := api.ModelStatus(tag)
		if err != nil {
			if params.ErrCode(err) != params.CodeNotFound {
				ctx.Infof("Unable to get the model status from the API: %v.", err)
			}
			return nil
		}
		if l := len(status); l != 1 {
			ctx.Infof("error finding model status: expected one result, got %d", l)
			return nil
		}
		return &modelData{
			machineCount:     status[0].HostedMachineCount,
			applicationCount: status[0].ServiceCount,
		}
	}
}

func formatDestroyModelInfo(data *modelData) string {
	out := "Waiting on model to be removed"
	if data.machineCount == 0 && data.applicationCount == 0 {
		return out
	}
	if data.machineCount > 0 {
		out += fmt.Sprintf(", %d machine(s)", data.machineCount)
	}
	if data.applicationCount > 0 {
		out += fmt.Sprintf(", %d application(s)", data.applicationCount)
	}
	return out
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

var getBudgetAPIClient = getBudgetAPIClientImpl

func getBudgetAPIClientImpl(bakeryClient *httpbakery.Client) BudgetAPIClient {
	return budget.NewClient(bakeryClient)
}

// BudgetAPIClient defines the budget API client interface.
type BudgetAPIClient interface {
	DeleteBudget(string) (string, error)
}
