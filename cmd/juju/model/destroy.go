// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for infos.

package model

import (
	"bytes"
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
)

const (
	slaUnsupported = "unsupported"
)

var logger = loggo.GetLogger("juju.cmd.juju.model")

// NewDestroyCommand returns a command used to destroy a model.
func NewDestroyCommand() cmd.Command {
	destroyCmd := &destroyCommand{}
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

	// sleepFunc is used when calling the timed function to get model status updates.
	sleepFunc func(time.Duration)

	envName        string
	assumeYes      bool
	destroyStorage bool
	releaseStorage bool
	api            DestroyModelAPI
	configApi      ModelConfigAPI
}

var destroyDoc = `
Destroys the specified model. This will result in the non-recoverable
removal of all the units operating in the model and any resources stored
there. Due to the irreversible nature of the command, it will prompt for
confirmation (unless overridden with the '-y' option) before taking any
action.

If there is persistent storage in any of the models managed by the
controller, then you must choose to either destroy or release the
storage, using --destroy-storage or --release-storage respectively.

Examples:

    juju destroy-model test
    juju destroy-model -y mymodel
    juju destroy-model -y mymodel --destroy-storage
    juju destroy-model -y mymodel --release-storage

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
	BestAPIVersion() int
	DestroyModel(tag names.ModelTag, destroyStorage *bool) error
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
	f.BoolVar(&c.destroyStorage, "destroy-storage", false, "Destroy all storage instances in the model")
	f.BoolVar(&c.releaseStorage, "release-storage", false, "Release all storage instances from the model, and management of the controller, without destroying them")
}

// Init implements Command.Init.
func (c *destroyCommand) Init(args []string) error {
	if c.destroyStorage && c.releaseStorage {
		return errors.New("--destroy-storage and --release-storage cannot both be specified")
	}
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

	controllerDetails, err := store.ControllerByName(controllerName)
	if err != nil {
		return errors.Annotate(err, "cannot read controller details")
	}
	modelName, modelDetails, err := c.ModelDetails()
	if err != nil {
		return errors.Trace(err)
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
		logger.Debugf("could not determine model SLA level: %v", err)
	}

	if api.BestAPIVersion() < 4 {
		// Versions before 4 support only destroying the storage,
		// and will not raise an error if there is storage in the
		// controller. Force the user to specify up-front.
		if c.releaseStorage {
			return errors.New("this juju controller only supports destroying storage")
		}
		if !c.destroyStorage {
			ctx.Infof(`this juju controller only supports destroying storage

Please run the the command again with --destroy-storage,
to confirm that you want to destroy the storage along
with the model.

If instead you want to keep the storage, you must first
upgrade the controller to version 2.3 or greater.

`)
			return cmd.ErrSilent
		}
	}

	// Attempt to destroy the model.
	ctx.Infof("Destroying model")
	var destroyStorage *bool
	if c.destroyStorage || c.releaseStorage {
		destroyStorage = &c.destroyStorage
	}
	modelTag := names.NewModelTag(modelDetails.ModelUUID)
	if err := api.DestroyModel(modelTag, destroyStorage); err != nil {
		return c.handleError(
			modelTag, modelName, api,
			errors.Annotate(err, "cannot destroy model"),
		)
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
	volumeCount      int
	filesystemCount  int
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
			volumeCount:      len(status[0].Volumes),
			filesystemCount:  len(status[0].Filesystems),
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
	if data.volumeCount > 0 {
		out += fmt.Sprintf(", %d volume(s)", data.volumeCount)
	}
	if data.filesystemCount > 0 {
		out += fmt.Sprintf(", %d filesystems(s)", data.filesystemCount)
	}
	return out
}

func (c *destroyCommand) handleError(
	modelTag names.ModelTag,
	modelName string,
	api DestroyModelAPI,
	err error,
) error {
	if params.IsCodeOperationBlocked(err) {
		return block.ProcessBlockedError(err, block.BlockDestroy)
	}
	if params.IsCodeHasPersistentStorage(err) {
		return handlePersistentStorageError(modelTag, modelName, api)
	}
	logger.Errorf(`failed to destroy model %q`, modelName)
	return err
}

func handlePersistentStorageError(
	modelTag names.ModelTag,
	modelName string,
	api DestroyModelAPI,
) error {
	modelStatuses, err := api.ModelStatus(modelTag)
	if err != nil {
		return errors.Annotate(err, "getting model status")
	}
	modelStatus := modelStatuses[0]

	var buf bytes.Buffer
	var persistentVolumes, persistentFilesystems int
	for _, v := range modelStatus.Volumes {
		if v.Detachable {
			persistentVolumes++
		}
	}
	for _, f := range modelStatus.Filesystems {
		if f.Detachable {
			persistentFilesystems++
		}
	}
	if n := persistentVolumes; n > 0 {
		fmt.Fprintf(&buf, "%d volume", n)
		if n > 1 {
			buf.WriteRune('s')
		}
		if persistentFilesystems > 0 {
			buf.WriteString(" and ")
		}
	}
	if n := persistentFilesystems; n > 0 {
		fmt.Fprintf(&buf, "%d filesystem", n)
		if n > 1 {
			buf.WriteRune('s')
		}
	}

	return errors.Errorf(`cannot destroy model %q

The model has persistent storage remaining:
	%s

To destroy the storage, run the destroy-model
command again with the "--destroy-storage" flag.

To release the storage from Juju's management
without destroying it, use the "--release-storage"
flag instead. The storage can then be imported
into another Juju model.

`, modelName, buf.String())
}

var getBudgetAPIClient = getBudgetAPIClientImpl

func getBudgetAPIClientImpl(bakeryClient *httpbakery.Client) BudgetAPIClient {
	return budget.NewClient(bakeryClient)
}

// BudgetAPIClient defines the budget API client interface.
type BudgetAPIClient interface {
	DeleteBudget(string) (string, error)
}
