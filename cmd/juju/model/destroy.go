// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for infos.

package model

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"text/template"
	"time"

	jujuclock "github.com/juju/clock"
	"github.com/juju/collections/transform"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v6"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/client/modelmanager"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/output"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/cmd"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/rpc/params"
)

var logger = internallogger.GetLogger("juju.cmd.juju.model")

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
	modelcmd.DestroyConfirmationCommandBase

	clock jujuclock.Clock

	timeout        time.Duration
	destroyStorage bool
	releaseStorage bool
	api            DestroyModelAPI

	Force  bool
	NoWait bool
	fs     *gnuflag.FlagSet
}

var destroyDoc = `
Destroys the specified model. This will result in the non-recoverable
removal of all the units operating in the model and any resources stored
there. Due to the irreversible nature of the command, it will prompt for
confirmation (unless overridden with the '--no-prompt' option) before taking any
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
proceed to the next step until the current step has finished. 
However, when using --force, users can also specify --no-wait to progress through steps 
without delay waiting for each step to complete.

WARNING: Passing --force with --timeout will continue the final destruction without
consideration or respect for clean shutdown or resource cleanup. If timeout 
elapses with --force, you may have resources left behind that will require
manual cleanup. If --force --timeout 0 is passed, the model is brutally
removed with haste. It is recommended to use graceful destroy (without --force or --no-wait).
`

const destroyExamples = `
    juju destroy-model --no-prompt mymodel
    juju destroy-model --no-prompt mymodel --destroy-storage
    juju destroy-model --no-prompt mymodel --release-storage
    juju destroy-model --no-prompt mymodel --force
    juju destroy-model --no-prompt mymodel --force --no-wait
`

var destroyModelMsg = `
This command will destroy the %q model and affect the following resources. It cannot be stopped.`[1:]

var destroyModelMsgDetails = `
{{- if gt .MachineCount 0}}
 - {{.MachineCount}} machine{{if gt .MachineCount 1}}s{{end}} will be destroyed
  - machine list:{{range .MachineIds}} "{{.}}"{{end}}
{{- end}}
 - {{.ApplicationCount}} application{{if gt .ApplicationCount 1}}s{{end}} will be removed
 {{- if gt (len .ApplicationNames) 0}}
  - application list:{{range .ApplicationNames}} "{{.}}"{{end}}
 {{- end}}
 - {{.FilesystemCount}} filesystem{{if gt .FilesystemCount 1}}s{{end}} and {{.VolumeCount}} volume{{if gt .VolumeCount 1}}s{{end}} will be {{if .ReleaseStorage}}released{{else}}destroyed{{end}}
`

var persistentStorageErrorMsg = `cannot destroy model "%s"

The model has persistent storage remaining:%s

To destroy the storage, run the destroy-model
command again with the "--destroy-storage" option.

To release the storage from Juju's management
without destroying it, use the "--release-storage"
option instead. The storage can then be imported
into another Juju model.

`

// DestroyModelAPI defines the methods on the modelmanager
// API that the destroy command calls. It is exported for mocking in tests.
type DestroyModelAPI interface {
	Close() error
	DestroyModel(ctx context.Context, tag names.ModelTag, destroyStorage, force *bool, maxWait *time.Duration, timeout *time.Duration) error
	ModelStatus(ctx context.Context, models ...names.ModelTag) ([]base.ModelStatus, error)
}

// Info implements Command.Info.
func (c *destroyCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "destroy-model",
		Args:     "[<controller name>:]<model name>",
		Purpose:  "Terminate all machines/containers and resources for a non-controller model.",
		Doc:      destroyDoc,
		Examples: destroyExamples,
		SeeAlso: []string{
			"destroy-controller",
		},
	})
}

const unsetTimeout = -1 * time.Second

// SetFlags implements Command.SetFlags.
func (c *destroyCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	c.DestroyConfirmationCommandBase.SetFlags(f)
	f.DurationVar(&c.timeout, "t", unsetTimeout, "Timeout for each step of force model destruction")
	f.DurationVar(&c.timeout, "timeout", unsetTimeout, "")
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
	if c.timeout < 0 && c.timeout != unsetTimeout {
		return errors.New("--timeout must be zero or greater")
	}
	if !c.Force && c.timeout >= 0 {
		return errors.New("--timeout can only be used with --force (dangerous)")
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

func (c *destroyCommand) getAPI(ctx context.Context) (DestroyModelAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	root, err := c.NewControllerAPIRoot(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return modelmanager.NewClient(root), nil
}

// getMachineIds gets slice of machine ids from modelData.
func getMachineIds(data base.ModelStatus) []string {
	return transform.Slice(data.Machines, func(f base.Machine) string {
		return fmt.Sprintf("%s (%s)", f.Id, f.InstanceId)
	})
}

// getApplicationNames gets slice of application names from modelData.
func getApplicationNames(data base.ModelStatus) []string {
	return transform.Slice(data.Applications, func(app base.Application) string {
		return fmt.Sprintf("%s", app.Name)
	})
}

// printDestroyWarningDetails prints to stderr the warning with additional info about destroying model.
func printDestroyWarningDetails(ctx *cmd.Context, modelStatus *base.ModelStatus, modelName string, modelType model.ModelType, releaseStorage bool) error {
	destroyMsgDetailsTmpl := template.New("destroyMsdDetails")
	destroyMsgDetailsTmpl, err := destroyMsgDetailsTmpl.Parse(destroyModelMsgDetails)
	if err != nil {
		return errors.Annotate(err, "Destroy controller message template parsing error.")
	}
	_ = destroyMsgDetailsTmpl.Execute(ctx.Stderr, map[string]any{
		"MachineCount":     modelStatus.HostedMachineCount,
		"MachineIds":       getMachineIds(*modelStatus),
		"ApplicationCount": modelStatus.ApplicationCount,
		"ApplicationNames": getApplicationNames(*modelStatus),
		"FilesystemCount":  len(modelStatus.Filesystems),
		"VolumeCount":      len(modelStatus.Volumes),
		"ReleaseStorage":   releaseStorage,
	})
	return nil
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
	modelName, modelDetails, err := c.ModelDetails(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	if modelDetails.ModelUUID == controllerDetails.ControllerUUID {
		return errors.Errorf("%q is a controller; use 'juju destroy-controller' to destroy it", modelName)
	}

	// Attempt to connect to the API.  If we can't, fail the destroy.
	api, err := c.getAPI(ctx)
	if err != nil {
		return errors.Annotate(err, "cannot connect to API")
	}
	defer func() { _ = api.Close() }()

	modelTag := names.NewModelTag(modelDetails.ModelUUID)
	modelStatus, err := getModelStatus(ctx, modelTag, api)
	if err != nil {
		return err
	}

	// Error out if the model has detachable storage and is not authorized to destroy or release it.
	detachableVolumes, detachableFilesystems := countDetachableStorage(modelStatus)
	if (detachableVolumes > 0 || detachableFilesystems > 0) && !(c.destroyStorage || c.releaseStorage) {
		return generatePersistentStorageErrorMsg(modelName, detachableVolumes, detachableFilesystems)
	}

	if c.DestroyConfirmationCommandBase.NeedsConfirmation() {
		ctx.Warningf(destroyModelMsg, modelName)
		if err := printDestroyWarningDetails(ctx, modelStatus, modelName, modelDetails.ModelType, c.releaseStorage); err != nil {
			return errors.Trace(err)
		}
		if err := jujucmd.UserConfirmName(modelName, "model", ctx); err != nil {
			return errors.Annotate(err, "model destruction")
		}
	}

	// Attempt to destroy the model.
	_, _ = fmt.Fprintln(ctx.Stderr, "Destroying model")
	var destroyStorage *bool
	if c.destroyStorage || c.releaseStorage {
		destroyStorage = &c.destroyStorage
	}
	var force *bool
	var maxWait *time.Duration
	if c.Force {
		force = &c.Force
		if c.NoWait {
			zero := 0 * time.Second
			maxWait = &zero
		}
	}

	var timeout *time.Duration
	if c.timeout >= 0 {
		timeout = &c.timeout
	}
	if err := api.DestroyModel(ctx, modelTag, destroyStorage, force, maxWait, timeout); err != nil {
		err = errors.Annotate(err, "cannot destroy model")

		if params.IsCodeOperationBlocked(err) {
			return block.ProcessBlockedError(err, block.BlockDestroy)
		}
		if params.IsCodeHasPersistentStorage(err) {
			modelStatus, err := getModelStatus(ctx, modelTag, api)
			if err != nil {
				return err
			}

			persistentVolumes, persistentFilesystems := countDetachableStorage(modelStatus)
			return generatePersistentStorageErrorMsg(modelName, persistentVolumes, persistentFilesystems)
		}
		logger.Errorf(context.TODO(), `failed to destroy model %q`, modelName)
		return err
	}

	// Wait for model to be destroyed.
	if err := waitForModelDestroyed(
		ctx, api,
		names.NewModelTag(modelDetails.ModelUUID),
		c.clock,
		c.Force,
	); err != nil {
		return err
	}

	c.RemoveModelFromClientStore(store, controllerName, modelName)
	return nil
}

type modelData struct {
	machineCount     int
	applicationCount int
	volumeCount      int
	filesystemCount  int
	errorCount       int
}

func waitForModelDestroyed(
	ctx *cmd.Context,
	api DestroyModelAPI,
	tag names.ModelTag,
	clock jujuclock.Clock,
	force bool,
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

	// wait a little bit for the model to be destroyed, backoff
	// exponentially, but no longer than 2s
	intervalSeconds := 10 * time.Millisecond
	const maxIntervalSeconds = 2 * time.Second
	reported := ""
	lineLength := 0
	const perLineLength = 80
	for {
		select {
		case <-interrupted:
			fmt.Fprint(ctx.Stderr, "\n\ndestroy model is still running in the background...")
			printErrors()
			msg := formatDestroyModelAbortInfo(data, false, force)
			fmt.Fprintln(ctx.Stderr, msg)
			return cmd.ErrSilent
		case <-clock.After(intervalSeconds):
			data, erroredStatuses = summarizeModelStatus(ctx, api, tag)
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
			intervalSeconds *= 2
			if intervalSeconds > maxIntervalSeconds {
				intervalSeconds = maxIntervalSeconds
			}
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
	w.Println("Resource", "ID", "Message")
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

func summarizeModelStatus(ctx *cmd.Context, api DestroyModelAPI, tag names.ModelTag) (*modelData, modelResourceErrorStatusSummary) {
	var erroredStatuses modelResourceErrorStatusSummary

	status, err := api.ModelStatus(ctx, tag)
	if err == nil && len(status) == 1 && status[0].Error != nil {
		// In 2.2 an error of one model generate an error for the entire request,
		// in 2.3 this was corrected to just be an error for the requested model.
		err = status[0].Error
	}
	if err != nil {
		if errors.Is(err, errors.NotFound) {
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
		out += fmt.Sprintf(", %d filesystem(s)", data.filesystemCount)
	}
	return out
}

func formatDestroyModelAbortInfo(data *modelData, timeout, force bool) string {
	out := ""
	if data != nil && (data.machineCount > 0 || data.applicationCount > 0 || data.volumeCount > 0 || data.filesystemCount > 0) {
		out = "\nThe following resources have not yet been removed:"
		if data.machineCount > 0 {
			out += fmt.Sprintf("\n - %d machine(s)", data.machineCount)
		}
		if data.applicationCount > 0 {
			out += fmt.Sprintf("\n - %d application(s)", data.applicationCount)
		}
		if data.volumeCount > 0 {
			out += fmt.Sprintf("\n - %d volume(s)", data.volumeCount)
		}
		if data.filesystemCount > 0 {
			out += fmt.Sprintf("\n - %d filesystem(s)", data.filesystemCount)
		}
	}
	if !timeout {
		return out
	}
	out += "\nBecause the destroy model operation did not finish, there may be cloud resources left behind."
	if !force {
		out += "\nRun 'destroy-model <model-name> --timeout=0 --force' to clean up the Juju model database records\neven with potentially orphaned cloud resources."
	}
	return out
}

func getModelStatus(ctx context.Context, modelTag names.ModelTag, api DestroyModelAPI) (*base.ModelStatus, error) {
	modelStatuses, err := api.ModelStatus(ctx, modelTag)
	if err != nil {
		return nil, errors.Annotate(err, "getting model status")
	}
	if l := len(modelStatuses); l != 1 {
		return nil, errors.Errorf("error finding model status: expected one result, got %d", l)
	}
	modelStatus := modelStatuses[0]
	if errors.Is(modelStatus.Error, errors.NotFound) {
		// This most likely occurred because a model was
		// destroyed half-way through the call.
		return nil, errors.Errorf("model not found, it may have been destroyed during this operation")
	}
	if modelStatus.Error != nil {
		if errors.Is(modelStatus.Error, errors.NotFound) {
			// This most likely occurred because a model was
			// destroyed half-way through the call.
			return nil, errors.Errorf("model not found, it may have been destroyed during this operation")
		}
		return nil, errors.Annotate(modelStatus.Error, "getting model status")
	}
	return &modelStatus, nil
}

func generatePersistentStorageErrorMsg(modelName string, detachableVolumeCount, detachableFilesystemCount int) error {
	var storageBuilder strings.Builder

	if detachableVolumeCount > 0 {
		storageBuilder.WriteString(fmt.Sprintf("\n    %d volume(s)", detachableVolumeCount))
	}

	if detachableFilesystemCount > 0 {
		storageBuilder.WriteString(fmt.Sprintf("\n    %d filesystem(s)", detachableFilesystemCount))
	}

	return errors.Errorf(persistentStorageErrorMsg, modelName, storageBuilder.String())
}

func countDetachableStorage(modelStatus *base.ModelStatus) (int, int) {
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
	return persistentVolumes, persistentFilesystems
}
