// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for infos.

package model

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"time"

	jujuclock "github.com/juju/clock"
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/romulus/api/budget"
	"gopkg.in/macaroon-bakery.v2/httpbakery"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/modelconfig"
	"github.com/juju/juju/api/modelmanager"
	"github.com/juju/juju/api/storage"
	"github.com/juju/juju/apiserver/params"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	rcmd "github.com/juju/juju/cmd/juju/romulus"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/output"
	"github.com/juju/juju/core/model"
	corestatus "github.com/juju/juju/core/status"
)

const (
	slaUnsupported = "unsupported"
)

var logger = loggo.GetLogger("juju.cmd.juju.model")

// NewDestroyCommand returns a command used to destroy a model.
func NewDestroyCommand() cmd.Command {
	destroyCmd := &destroyCommand{
		clock: jujuclock.WallClock,
	}
	destroyCmd.CanClearCurrentModel = true
	return modelcmd.Wrap(
		destroyCmd,
		modelcmd.WrapSkipDefaultModel,
		modelcmd.WrapSkipModelFlags,
	)
}

// destroyCommand destroys the specified model.
type destroyCommand struct {
	modelcmd.ModelCommandBase

	clock jujuclock.Clock

	assumeYes      bool
	timeout        time.Duration
	destroyStorage bool
	releaseStorage bool
	api            DestroyModelAPI
	configAPI      ModelConfigAPI
	storageAPI     StorageAPI

	Force  bool
	NoWait bool
	fs     *gnuflag.FlagSet
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

Sometimes, the destruction of the model may fail as Juju encounters errors
and failures that need to be dealt with before a model can be destroyed.
However, at times, there is a need to destroy a model ignoring
all operational errors. In these rare cases, use --force option but note 
that --force will also remove all units of the application, its subordinates
and, potentially, machines without given them the opportunity to shutdown cleanly.

Model destruction is a multi-step process. Under normal circumstances, Juju will not
proceed to a next step until the current step has finished. 
However, when using --force, users can also specify --no-wait to progress through steps 
without delay waiting for each step to complete.

Examples:

    juju destroy-model test
    juju destroy-model -y mymodel
    juju destroy-model -y mymodel --destroy-storage
    juju destroy-model -y mymodel --release-storage
    juju destroy-model -y mymodel --force
    juju destroy-model -y mymodel --force --no-wait

See also:
    destroy-controller
`
var destroyIAASModelMsg = `
WARNING! This command will destroy the %q model.
This includes all machines, applications, data and other resources.

Continue [y/N]? `[1:]

var destroyCAASModelMsg = `
WARNING! This command will destroy the %q model.
This includes all containers, applications, data and other resources.

Continue [y/N]? `[1:]

// DestroyModelAPI defines the methods on the modelmanager
// API that the destroy command calls. It is exported for mocking in tests.
type DestroyModelAPI interface {
	Close() error
	BestAPIVersion() int
	DestroyModel(tag names.ModelTag, destroyStorage, force *bool, maxWait *time.Duration) error
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
	return jujucmd.Info(&cmd.Info{
		Name:    "destroy-model",
		Args:    "[<controller name>:]<model name>",
		Purpose: "Terminate all machines/containers and resources for a non-controller model.",
		Doc:     destroyDoc,
	})
}

// SetFlags implements Command.SetFlags.
func (c *destroyCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.BoolVar(&c.assumeYes, "y", false, "Do not prompt for confirmation")
	f.BoolVar(&c.assumeYes, "yes", false, "")
	f.DurationVar(&c.timeout, "t", 30*time.Minute, "Timeout before model destruction is aborted")
	f.DurationVar(&c.timeout, "timeout", 30*time.Minute, "")
	f.BoolVar(&c.destroyStorage, "destroy-storage", false, "Destroy all storage instances in the model")
	f.BoolVar(&c.releaseStorage, "release-storage", false, "Release all storage instances from the model, and management of the controller, without destroying them")
	f.BoolVar(&c.Force, "force", false, "Force destroy model ignoring any errors")
	f.BoolVar(&c.NoWait, "no-wait", false, "Rush through model destruction without waiting for each individual step to complete")
	c.fs = f
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
		return c.SetModelIdentifier(args[0], false)
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
	if c.configAPI != nil {
		return c.configAPI, nil
	}
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return modelconfig.NewClient(root), nil
}

func (c *destroyCommand) getStorageAPI() (StorageAPI, error) {
	if c.storageAPI != nil {
		return c.storageAPI, nil
	}
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return storage.NewClient(root), nil
}

// Run implements Command.Run
func (c *destroyCommand) Run(ctx *cmd.Context) error {
	noWaitSet := false
	forceSet := false
	c.fs.Visit(func(flag *gnuflag.Flag) {
		if flag.Name == "no-wait" {
			noWaitSet = true
		} else if flag.Name == "force" {
			forceSet = true
		}
	})
	if !forceSet && noWaitSet {
		return errors.NotValidf("--no-wait without --force")
	}

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
		modelType, err := c.ModelType()
		if err != nil {
			return errors.Trace(err)
		}
		msg := destroyIAASModelMsg
		if modelType == model.CAAS {
			msg = destroyCAASModelMsg
		}
		fmt.Fprintf(ctx.Stdout, msg, modelName)

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

	configAPI, err := c.getModelConfigAPI()
	if err != nil {
		return errors.Annotate(err, "cannot connect to API")
	}
	defer configAPI.Close()

	// Check if the model has an SLA set.
	slaIsSet := false
	slaLevel, err := configAPI.SLALevel()
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
			storageAPI, err := c.getStorageAPI()
			if err != nil {
				return errors.Trace(err)
			}
			defer storageAPI.Close()

			storageDetails, err := storageAPI.ListStorageDetails()
			if err != nil {
				return errors.Trace(err)
			}
			if len(storageDetails) > 0 {
				return errors.Errorf(`cannot destroy model %q

Destroying this model will destroy the storage, but you
have not indicated that you want to do that.

Please run the the command again with --destroy-storage
to confirm that you want to destroy the storage along
with the model.

If instead you want to keep the storage, you must first
upgrade the controller to version 2.3 or greater.

`, modelName)
			}
			c.destroyStorage = true
		}
	}

	// Attempt to destroy the model.
	fmt.Fprint(ctx.Stderr, "Destroying model")
	var destroyStorage *bool
	if c.destroyStorage || c.releaseStorage {
		destroyStorage = &c.destroyStorage
	}
	var force *bool
	var maxWait *time.Duration
	if c.Force {
		force = &c.Force
		if c.NoWait {
			zeroSec := 0 * time.Second
			maxWait = &zeroSec
		}
	}
	modelTag := names.NewModelTag(modelDetails.ModelUUID)
	if err := api.DestroyModel(modelTag, destroyStorage, force, maxWait); err != nil {
		return c.handleError(
			modelTag, modelName, api,
			errors.Annotate(err, "cannot destroy model"),
		)
	}

	// Wait for model to be destroyed.
	if err := waitForModelDestroyed(
		ctx, api,
		names.NewModelTag(modelDetails.ModelUUID),
		c.timeout,
		c.clock,
	); err != nil {
		return err
	}

	// Check if the model has an sla auth.
	if slaIsSet {
		err = c.removeModelBudget(modelDetails.ModelUUID)
		if err != nil {
			ctx.Warningf("model allocation not removed: %v", err)
		}
	}

	c.RemoveModelFromClientStore(store, controllerName, modelName)
	return nil
}

func (c *destroyCommand) removeModelBudget(uuid string) error {
	bakeryClient, err := c.BakeryClient()
	if err != nil {
		return errors.Trace(err)
	}

	budgetAPIRoot, err := rcmd.GetMeteringURLForModelCmd(&c.ModelCommandBase)
	if err != nil {
		return errors.Trace(err)
	}
	budgetClient, err := getBudgetAPIClient(budgetAPIRoot, bakeryClient)
	if err != nil {
		return errors.Trace(err)
	}

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
	errorCount       int
}

func (data *modelData) isEmpty() bool {
	return data.errorCount == 0 &&
		data.machineCount == 0 &&
		data.applicationCount == 0 &&
		data.volumeCount == 0 &&
		data.filesystemCount == 0
}

func waitForModelDestroyed(
	ctx *cmd.Context,
	api DestroyModelAPI,
	tag names.ModelTag,
	timeout time.Duration,
	clock jujuclock.Clock,
) error {

	interrupted := make(chan os.Signal, 1)
	defer close(interrupted)
	ctx.InterruptNotify(interrupted)
	defer ctx.StopInterruptNotify(interrupted)

	var data *modelData
	var erroredStatuses modelResourceErrorStatusSummary

	printErrors := func() {
		erroredStatuses.PrettyPrint(ctx.Stdout)
	}

	// no wait for 1st time.
	intervalSeconds := 0 * time.Second
	timeoutAfter := clock.After(timeout)
	reported := ""
	lineLength := 0
	const perLineLength = 80
	for {
		select {
		case <-interrupted:
			ctx.Infof("ctrl+c detected, aborting...")
			printErrors()
			return cmd.ErrSilent
		case <-timeoutAfter:
			printErrors()
			return errors.Timeoutf("timeout after %v", timeout)
		case <-clock.After(intervalSeconds):
			data, erroredStatuses = getModelStatus(ctx, api, tag)
			if data == nil {
				// model has been destroyed successfully.
				return nil
			}
			msg := formatDestroyModelInfo(data)
			if reported == msg {
				if lineLength == perLineLength {
					// Time to break to the next line.
					fmt.Fprintln(ctx.Stderr)
					lineLength = 0
				}
				fmt.Fprint(ctx.Stderr, ".")
				lineLength++
			} else {
				fmt.Fprint(ctx.Stderr, fmt.Sprintf("\n%v...", msg))
				reported = msg
				lineLength = len(msg) + 3
			}
			intervalSeconds = 2 * time.Second
		}
	}
}

type modelResourceErrorStatus struct {
	ID, Message string
}

type modelResourceErrorStatusSummary struct {
	Machines    []modelResourceErrorStatus
	Filesystems []modelResourceErrorStatus
	Volumes     []modelResourceErrorStatus
}

func (s modelResourceErrorStatusSummary) Count() int {
	return len(s.Machines) + len(s.Filesystems) + len(s.Volumes)
}

func (s modelResourceErrorStatusSummary) PrettyPrint(writer io.Writer) error {
	if s.Count() == 0 {
		return nil
	}

	tw := output.TabWriter(writer)
	w := output.Wrapper{tw}
	w.Println(`
The following errors were encountered during destroying the model.
You can fix the problem causing the errors and run destroy-model again.
`)
	w.Println("Resource", "Id", "Message")
	for _, resources := range []map[string][]modelResourceErrorStatus{
		{"Machine": s.Machines},
		{"Filesystem": s.Filesystems},
		{"Volume": s.Volumes},
	} {
		for k, v := range resources {
			resourceType := k
			for _, r := range v {
				w.Println(resourceType, r.ID, r.Message)
				resourceType = ""
			}
		}
	}
	tw.Flush()
	return nil
}

func getModelStatus(ctx *cmd.Context, api DestroyModelAPI, tag names.ModelTag) (*modelData, modelResourceErrorStatusSummary) {
	var erroredStatuses modelResourceErrorStatusSummary

	status, err := api.ModelStatus(tag)
	if err == nil && len(status) == 1 && status[0].Error != nil {
		// In 2.2 an error of one model generate an error for the entire request,
		// in 2.3 this was corrected to just be an error for the requested model.
		err = status[0].Error
	}
	if err != nil {
		if params.IsCodeNotFound(err) {
			ctx.Infof("\nModel destroyed.")
		} else {
			ctx.Infof("Unable to get the model status from the API: %v.", err)
		}
		return nil, erroredStatuses
	}
	isError := func(s string) bool {
		return corestatus.Error.Matches(corestatus.Status(s))
	}
	for _, s := range status {
		for _, v := range s.Machines {
			if isError(v.Status) {
				erroredStatuses.Machines = append(erroredStatuses.Machines, modelResourceErrorStatus{
					ID:      v.Id,
					Message: v.Message,
				})
			}
		}
		for _, v := range s.Filesystems {
			if isError(v.Status) {
				erroredStatuses.Filesystems = append(erroredStatuses.Filesystems, modelResourceErrorStatus{
					ID:      v.Id,
					Message: v.Message,
				})
			}
		}
		for _, v := range s.Volumes {
			if isError(v.Status) {
				erroredStatuses.Volumes = append(erroredStatuses.Volumes, modelResourceErrorStatus{
					ID:      v.Id,
					Message: v.Message,
				})
			}
		}
	}

	if l := len(status); l != 1 {
		ctx.Infof("error finding model status: expected one result, got %d", l)
		return nil, erroredStatuses
	}
	return &modelData{
		machineCount:     status[0].HostedMachineCount,
		applicationCount: status[0].ApplicationCount,
		volumeCount:      len(status[0].Volumes),
		filesystemCount:  len(status[0].Filesystems),
		errorCount:       erroredStatuses.Count(),
	}, erroredStatuses
}

func formatDestroyModelInfo(data *modelData) string {
	out := "Waiting for model to be removed"
	if data.errorCount > 0 {
		// always shows errorCount even if no machines and applications left.
		out += fmt.Sprintf(", %d error(s)", data.errorCount)
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
	if l := len(modelStatuses); l != 1 {
		return errors.Errorf("error finding model status: expected one result, got %d", l)
	}
	modelStatus := modelStatuses[0]
	if modelStatus.Error != nil {
		if errors.IsNotFound(modelStatus.Error) {
			// This most likely occurred because a model was
			// destroyed half-way through the call.
			return nil
		}
		return errors.Annotate(err, "getting model status")
	}

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
command again with the "--destroy-storage" option.

To release the storage from Juju's management
without destroying it, use the "--release-storage"
option instead. The storage can then be imported
into another Juju model.

`, modelName, buf.String())
}

var getBudgetAPIClient = getBudgetAPIClientImpl

func getBudgetAPIClientImpl(apiRoot string, bakeryClient *httpbakery.Client) (BudgetAPIClient, error) {
	return budget.NewClient(budget.APIRoot(apiRoot), budget.HTTPClient(bakeryClient))
}

// BudgetAPIClient defines the budget API client interface.
type BudgetAPIClient interface {
	DeleteBudget(string) (string, error)
}

// StorageAPI defines the storage client API interface.
type StorageAPI interface {
	Close() error
	ListStorageDetails() ([]params.StorageDetails, error)
}
