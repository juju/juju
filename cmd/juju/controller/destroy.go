// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"bytes"
	"context"
	"fmt"
	"text/template"
	"time"

	"github.com/juju/clock"
	"github.com/juju/collections/transform"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v6"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/client/modelconfig"
	controllerapi "github.com/juju/juju/api/controller/controller"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/cloud"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/rpc/params"
)

// NewDestroyCommand returns a command to destroy a controller.
func NewDestroyCommand() cmd.Command {
	cmd := destroyCommand{}
	cmd.environsDestroy = environs.Destroy
	// Even though this command is all about destroying a controller we end up
	// needing environment endpoints so we can fall back to the client destroy
	// environment method. This shouldn't really matter in practice as the
	// user trying to take down the controller will need to have access to the
	// controller environment anyway.
	return modelcmd.WrapController(
		&cmd,
		modelcmd.WrapControllerSkipControllerFlags,
		modelcmd.WrapControllerSkipDefaultController,
	)
}

// destroyCommand destroys the specified controller.
type destroyCommand struct {
	destroyCommandBase
	destroyModels  bool
	destroyStorage bool
	releaseStorage bool
	modelTimeout   time.Duration
	force          bool
	noWait         bool
}

// usageDetails has backticks which we want to keep for markdown processing.
// TODO(cheryl): Do we want the usage, options, examples, and see also text in
// backticks for markdown?
var usageDetails = `
All workload models running on the controller will first
need to be destroyed, either in advance, or by
specifying `[1:] + "`--destroy-all-models`." + `

If there is persistent storage in any of the models managed by the
controller, then you must choose to either destroy or release the
storage, using ` + "`--destroy-storage` or `--release-storage` respectively." + `

Sometimes, the destruction of a model may fail as Juju encounters errors
that need to be dealt with before that model can be destroyed.
However, at times, there is a need to destroy a controller ignoring
such model errors. In these rare cases, use --force option but note 
that --force will also remove all units of any hosted applications, their subordinates
and, potentially, machines without given them the opportunity to shutdown cleanly.

Model destruction is a multi-step process. Under normal circumstances, Juju will not
proceed to the next step until the current step has finished. 
However, when using --force, users can also specify --no-wait to progress through steps 
without delay waiting for each step to complete.

WARNING: Passing --force with --model-timeout will continue the final destruction without
consideration or respect for clean shutdown or resource cleanup. If model-timeout 
elapses with --force, you may have resources left behind that will require
manual cleanup. If --force --model-timeout 0 is passed, the models are brutally
removed with haste. It is recommended to use graceful destroy (without --force, --no-wait or
--model-timeout).

`

const usageExamples = `
Destroy the controller and all models. If there is
persistent storage remaining in any of the models, then
this will prompt you to choose to either destroy or release
the storage.

    juju destroy-controller --destroy-all-models mycontroller

Destroy the controller and all models, destroying
any remaining persistent storage.

    juju destroy-controller --destroy-all-models --destroy-storage

Destroy the controller and all models, releasing
any remaining persistent storage from Juju's control.

    juju destroy-controller --destroy-all-models --release-storage

Destroy the controller and all models, continuing
even if there are operational errors.

    juju destroy-controller --destroy-all-models --force
    juju destroy-controller --destroy-all-models --force --no-wait
`

var usageSummary = `
Destroys a controller.`[1:]

var destroySysMsg = `
This command will destroy the %q controller and all its resources
`[1:]

var destroySysMsgDetails = `
{{- if gt .ModelCount 0}}
 - {{.ModelCount}} model{{if gt .ModelCount 1}}s{{end}} will be destroyed
  - model list:{{range .ModelNames}} "{{.}}"{{end}}
 - {{.MachineCount}} machine{{if gt .MachineCount 1}}s{{end}} will be destroyed
 - {{.ApplicationCount}} application{{if gt .ApplicationCount 1}}s{{end}} will be removed
 {{- if gt (len .ApplicationNames) 0}}
  - application list:{{range .ApplicationNames}} "{{.}}"{{end}}
 {{- end}}
 - {{.FilesystemCount}} filesystem{{if gt .FilesystemCount 1}}s{{end}} and {{.VolumeCount}} volume{{if gt .VolumeCount 1}}s{{end}} will be {{if .ReleaseStorage}}released{{else}}destroyed{{end}}
{{- end}}
`[1:]

// destroyControllerAPI defines the methods on the controller API endpoint
// that the destroy command calls.
type destroyControllerAPI interface {
	Close() error
	HostedModelConfigs(context.Context) ([]controllerapi.HostedConfig, error)
	CloudSpec(context.Context, names.ModelTag) (environscloudspec.CloudSpec, error)
	DestroyController(context.Context, controllerapi.DestroyControllerParams) error
	ListBlockedModels(context.Context) ([]params.ModelBlockInfo, error)
	ModelStatus(ctx context.Context, models ...names.ModelTag) ([]base.ModelStatus, error)
	AllModels(context.Context) ([]base.UserModel, error)
	ControllerConfig(context.Context) (controller.Config, error)
}

type modelConfigAPI interface {
	Close() error
	ModelGet(ctx context.Context) (map[string]interface{}, error)
}

// Info implements Command.Info.
func (c *destroyCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "destroy-controller",
		Args:     "<controller name>",
		Purpose:  usageSummary,
		Doc:      usageDetails,
		Examples: usageExamples,
		SeeAlso: []string{
			"kill-controller",
			"unregister",
		},
	})
}

const unsetTimeout = -1 * time.Second

// SetFlags implements Command.SetFlags.
func (c *destroyCommand) SetFlags(f *gnuflag.FlagSet) {
	c.destroyCommandBase.SetFlags(f)
	f.BoolVar(&c.destroyModels, "destroy-all-models", false, "Destroy all models in the controller")
	f.BoolVar(&c.destroyStorage, "destroy-storage", false, "Destroy all storage instances managed by the controller")
	f.BoolVar(&c.releaseStorage, "release-storage", false, "Release all storage instances from management of the controller, without destroying them")
	f.DurationVar(&c.modelTimeout, "model-timeout", unsetTimeout, "Timeout for each step of force model destruction")
	f.BoolVar(&c.force, "force", false, "Force destroy models ignoring any errors")
	f.BoolVar(&c.noWait, "no-wait", false, "Rush through model destruction without waiting for each individual step to complete")
}

// Init implements Command.Init.
func (c *destroyCommand) Init(args []string) error {
	if c.destroyStorage && c.releaseStorage {
		return errors.New("--destroy-storage and --release-storage cannot both be specified")
	}
	if !c.force && c.modelTimeout >= 0 {
		return errors.New("--model-timeout can only be used with --force (dangerous)")
	}
	return c.destroyCommandBase.Init(args)
}

// getModelNames gets slice of model names from modelData.
func getModelNames(data []modelData) []string {
	return transform.Slice(data, func(f modelData) string {
		return fmt.Sprintf("%s/%s (%s)", f.Namespace, f.Name, f.Life)
	})
}

// getApplicationNames gets slice of application names from modelData.
func getApplicationNames(data []base.Application) []string {
	return transform.Slice(data, func(app base.Application) string {
		return app.Name
	})
}

// printDestroyWarningDetails prints to stderr the warning with additional info about destroying controller.
func printDestroyWarningDetails(ctx *cmd.Context, modelStatus environmentStatus, releaseStorage bool) error {
	destroyMsgDetailsTmpl := template.New("destroyMsdDetails")
	destroyMsgDetailsTmpl, err := destroyMsgDetailsTmpl.Parse(destroySysMsgDetails)
	if err != nil {
		return errors.Annotate(err, "Destroy controller message template parsing error.")
	}
	_ = destroyMsgDetailsTmpl.Execute(ctx.Stderr, map[string]any{
		"ModelCount":       modelStatus.Controller.HostedModelCount,
		"ModelNames":       getModelNames(modelStatus.Models),
		"MachineCount":     modelStatus.Controller.HostedMachineCount,
		"ApplicationCount": modelStatus.Controller.ApplicationCount - 1, //  -1 not to count controller app itself
		"ApplicationNames": getApplicationNames(modelStatus.Applications),
		"FilesystemCount":  modelStatus.Controller.TotalFilesystemCount,
		"VolumeCount":      modelStatus.Controller.TotalVolumeCount,
		"ReleaseStorage":   releaseStorage,
	})
	return nil
}

// Run implements Command.Run
func (c *destroyCommand) Run(ctx *cmd.Context) error {
	controllerName, err := c.ControllerName()
	if err != nil {
		return errors.Trace(err)
	}
	store := c.ClientStore()

	// Attempt to connect to the API.  If we can't, fail the destroy.  Users will
	// need to use the controller kill command if we can't connect.
	api, err := c.getControllerAPI(ctx)
	if err != nil {
		return c.ensureUserFriendlyErrorLog(errors.Annotate(err, "cannot connect to API"), ctx, nil)
	}
	defer func() { _ = api.Close() }()

	controllerModelConfigAPI, err := c.getControllerModelConfigAPI(ctx)
	if err != nil {
		return fmt.Errorf("cannot connect to model config API: %w", err)
	}
	defer func() { _ = controllerModelConfigAPI.Close() }()

	// Obtain controller environ so we can clean up afterwards.
	controllerEnviron, err := c.getControllerEnviron(ctx, store, controllerName, api, controllerModelConfigAPI)
	if err != nil {
		return errors.Annotate(err, "getting controller environ")
	}

	ctx.Warningf(destroySysMsg, controllerName)
	updateStatus := newTimedStatusUpdater(ctx, api, controllerEnviron.Config().UUID(), clock.WallClock)
	modelStatus := updateStatus(0)

	// check Alive models and --destroy-all-models flag usage
	if !c.destroyModels {
		if err := c.checkNoAliveHostedModels(modelStatus.Models); err != nil {
			return errors.Trace(err)
		}
	}
	// check user has not specified whether storage should be destroyed or released.
	// Make sure there are no filesystems or volumes in the model.
	if !c.destroyStorage && !c.releaseStorage {
		if err := c.checkNoPersistentStorage(modelStatus); err != nil {
			return errors.Trace(err)
		}
	}
	// ask for confirmation after all flag checks
	if c.DestroyConfirmationCommandBase.NeedsConfirmation() {
		if err := printDestroyWarningDetails(ctx, modelStatus, c.releaseStorage); err != nil {
			return errors.Trace(err)
		}
		if err := jujucmd.UserConfirmName(controllerName, "controller", ctx); err != nil {
			return errors.Annotate(err, "controller destruction")
		}
	}

	for {
		// Attempt to destroy the controller.
		ctx.Infof("Destroying controller")
		var hasHostedModels bool
		var hasPersistentStorage bool
		var destroyStorage *bool
		if c.destroyStorage || c.releaseStorage {
			// Set destroyStorage to true or false, if
			// --destroy-storage or --release-storage
			// is specified, respectively.
			destroyStorage = &c.destroyStorage
		}

		var force *bool
		var maxWait *time.Duration
		if c.force {
			force = &c.force
			if c.noWait {
				zeroSec := 0 * time.Second
				maxWait = &zeroSec
			}
		}

		var modelTimeout *time.Duration
		if c.modelTimeout >= 0 {
			modelTimeout = &c.modelTimeout
		}

		err = api.DestroyController(ctx, controllerapi.DestroyControllerParams{
			DestroyModels:  c.destroyModels,
			DestroyStorage: destroyStorage,
			Force:          force,
			MaxWait:        maxWait,
			ModelTimeout:   modelTimeout,
		})
		if err != nil {
			if params.IsCodeHasHostedModels(err) {
				hasHostedModels = true
			} else if params.IsCodeHasPersistentStorage(err) {
				hasPersistentStorage = true
			} else {
				return c.ensureUserFriendlyErrorLog(
					errors.Annotate(err, "cannot destroy controller"),
					ctx, api,
				)
			}
		}

		updateStatus = newTimedStatusUpdater(ctx, api, controllerEnviron.Config().UUID(), clock.WallClock)
		modelStatus = updateStatus(0)
		if !c.destroyModels {
			if err := c.checkNoAliveHostedModels(modelStatus.Models); err != nil {
				return errors.Trace(err)
			}
			if hasHostedModels && !hasUnDeadModels(modelStatus.Models) {
				// When we called DestroyController before, we were
				// informed that there were hosted models remaining.
				// When we checked just now, there were none. We should
				// try destroying again.
				continue
			}
		}
		if !c.destroyStorage && !c.releaseStorage && hasPersistentStorage {
			if err := c.checkNoPersistentStorage(modelStatus); err != nil {
				return errors.Trace(err)
			}
			// When we called DestroyController before, we were
			// informed that there was persistent storage remaining.
			// When we checked just now, there was none. We should
			// try destroying again.
			continue
		}

		// Even if we've not just requested for hosted models to be destroyed,
		// there may be some being destroyed already. We need to wait for them.
		// Check for both undead models and live machines, as machines may be
		// in the controller model.
		ctx.Infof("Waiting for model resources to be reclaimed")
		// wait for 2 seconds to let empty hosted models changed from alive to dying.
		for ; hasUnreclaimedResources(modelStatus); modelStatus = updateStatus(2 * time.Second) {
			ctx.Infof("%s", fmtCtrStatus(modelStatus.Controller))
			for _, model := range modelStatus.Models {
				ctx.Verbosef("%s", fmtModelStatus(model))
			}
		}
		ctx.Infof("All models reclaimed, cleaning up controller machines")
		return c.environsDestroy(controllerName, controllerEnviron, ctx, store)
	}
}

// checkNoAliveHostedModels ensures that the given set of hosted models
// contains none that are Alive. If there are, a message is printed
// out to
func (c *destroyCommand) checkNoAliveHostedModels(models []modelData) error {
	if !hasAliveModels(models) {
		return nil
	}
	// The user did not specify --destroy-all-models,
	// and there are models still alive.
	var buf bytes.Buffer
	for _, model := range models {
		if model.Life != life.Alive {
			continue
		}
		buf.WriteString(fmtModelStatus(model))
		buf.WriteRune('\n')
	}
	controllerName, err := c.ControllerName()
	if err != nil {
		return errors.Trace(err)
	}
	return errors.Errorf(`cannot destroy controller %q

The controller has live models. If you want
to destroy all models in the controller,
run this command again with the --destroy-all-models
option.

Models:
%s`, controllerName, buf.String())
}

// checkNoPersistentStorage ensures that the controller contains
// no persistent storage. If there is any, a message is printed
// out informing the user that they must choose to destroy or
// release the storage.
func (c *destroyCommand) checkNoPersistentStorage(envStatus environmentStatus) error {
	models := append([]modelData{envStatus.Controller.Model}, envStatus.Models...)

	var modelsWithPersistentStorage int
	var persistentVolumesTotal int
	var persistentFilesystemsTotal int
	for _, m := range models {
		if m.PersistentVolumeCount+m.PersistentFilesystemCount == 0 {
			continue
		}
		modelsWithPersistentStorage++
		persistentVolumesTotal += m.PersistentVolumeCount
		persistentFilesystemsTotal += m.PersistentFilesystemCount
	}

	var buf bytes.Buffer
	if n := persistentVolumesTotal; n > 0 {
		fmt.Fprintf(&buf, "%d volume", n)
		if n > 1 {
			buf.WriteRune('s')
		}
		if persistentFilesystemsTotal > 0 {
			buf.WriteString(" and ")
		}
	}
	if n := persistentFilesystemsTotal; n > 0 {
		fmt.Fprintf(&buf, "%d filesystem", n)
		if n > 1 {
			buf.WriteRune('s')
		}
	}
	buf.WriteRune(' ')
	if n := modelsWithPersistentStorage; n == 0 {
		return nil
	} else if n == 1 {
		buf.WriteString("in 1 model")
	} else {
		fmt.Fprintf(&buf, "across %d models", n)
	}

	controllerName, err := c.ControllerName()
	if err != nil {
		return errors.Trace(err)
	}

	return errors.Errorf(`cannot destroy controller %q

The controller has persistent storage remaining:
	%s

To destroy the storage, run the destroy-controller
command again with the "--destroy-storage" option.

To release the storage from Juju's management
without destroying it, use the "--release-storage"
option instead. The storage can then be imported
into another Juju model.

`, controllerName, buf.String())
}

// ensureUserFriendlyErrorLog ensures that error will be logged and displayed
// in a user-friendly manner with readable and digestable error message.
func (c *destroyCommand) ensureUserFriendlyErrorLog(destroyErr error, ctx *cmd.Context, api destroyControllerAPI) error {
	if destroyErr == nil {
		return nil
	}
	if params.IsCodeOperationBlocked(destroyErr) {
		logger.Errorf(context.TODO(), destroyControllerBlockedMsg)
		if api != nil {
			models, err := api.ListBlockedModels(ctx)
			out := &bytes.Buffer{}
			if err == nil {
				var info interface{}
				info, err = block.FormatModelBlockInfo(models)
				if err != nil {
					return errors.Trace(err)
				}
				err = block.FormatTabularBlockedModels(out, info)
			}
			if err != nil {
				logger.Errorf(context.TODO(), "Unable to list models: %s", err)
				return cmd.ErrSilent
			}
			ctx.Infof("%s", out.String())
		}
		return cmd.ErrSilent
	}
	controllerName, err := c.ControllerName()
	if err != nil {
		return errors.Trace(err)
	}
	logger.Errorf(context.TODO(), stdFailureMsg, controllerName)
	return destroyErr
}

const destroyControllerBlockedMsg = `there are models with disabled commands preventing controller destruction

To enable controller destruction, please run:

    juju enable-destroy-controller

`

// TODO(axw) this should only be printed out if we couldn't
// connect to the controller.
const stdFailureMsg = `failed to destroy controller %q

If the controller is unusable, then you may run

    juju kill-controller

to forcibly destroy the controller. Upon doing so, review
your cloud provider console for any resources that need
to be cleaned up.

`

// destroyCommandBase provides common attributes and methods that both the controller
// destroy and controller kill commands require.
type destroyCommandBase struct {
	modelcmd.ControllerCommandBase
	modelcmd.DestroyConfirmationCommandBase

	// The following fields are for mocking out
	// api behavior for testing.
	api    destroyControllerAPI
	apierr error

	controllerModelConfigAPI modelConfigAPI

	environsDestroy func(string, environs.ControllerDestroyer, context.Context, jujuclient.ControllerStore) error
}

func (c *destroyCommandBase) getControllerAPI(ctx context.Context) (destroyControllerAPI, error) {
	// Note that some tests set c.api to a non-nil value
	// even when c.apierr is non-nil, hence the separate test.
	if c.apierr != nil {
		return nil, c.apierr
	}
	if c.api != nil {
		return c.api, nil
	}
	root, err := c.NewAPIRoot(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return controllerapi.NewClient(root), nil
}

func (c *destroyCommandBase) getControllerModelConfigAPI(ctx context.Context) (modelConfigAPI, error) {
	if c.controllerModelConfigAPI != nil {
		return c.controllerModelConfigAPI, nil
	}
	root, err := c.NewAPIRoot(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return modelconfig.NewClient(root), nil
}

// SetFlags implements Command.SetFlags.
func (c *destroyCommandBase) SetFlags(f *gnuflag.FlagSet) {
	c.ControllerCommandBase.SetFlags(f)
	c.DestroyConfirmationCommandBase.SetFlags(f)
}

// Init implements Command.Init.
func (c *destroyCommandBase) Init(args []string) error {
	switch len(args) {
	case 0:
		return errors.New("no controller specified")
	case 1:
		return c.SetControllerName(args[0], false)
	default:
		return cmd.CheckEmpty(args[1:])
	}
}

// getControllerEnviron returns the Environ for the controller model.
//
// getControllerEnviron gets the information required to get the
// Environ by first checking the config store, then querying the
// API if the information is not in the store.
func (c *destroyCommandBase) getControllerEnviron(
	ctx *cmd.Context,
	store jujuclient.ClientStore,
	controllerName string,
	sysAPI destroyControllerAPI,
	controllerModelConfigAPI modelConfigAPI,
) (environs.BootstrapEnviron, error) {
	// TODO: (hml) 2018-08-01
	// We should try to destroy via the API first, from store is a
	// fall back position.
	env, err := c.getControllerEnvironFromStore(ctx, store, controllerName)
	if errors.Is(err, errors.NotFound) {
		return c.getControllerEnvironFromAPI(ctx, sysAPI, controllerModelConfigAPI)
	} else if err != nil {
		return nil, errors.Annotate(err, "getting environ using bootstrap config from client store")
	}
	return env, nil
}

func (c *destroyCommandBase) getControllerCloudSpecFromStore(
	ctx *cmd.Context,
	store jujuclient.ClientStore,
	controllerName string,
) (environscloudspec.CloudSpec, error) {
	_, spec, _, err := modelcmd.NewGetBootstrapConfigParamsFunc(
		ctx, store, environs.GlobalProviderRegistry(),
	)(controllerName)
	if err != nil {
		return environscloudspec.CloudSpec{}, errors.Trace(err)
	}

	return *spec, nil
}

func (c *destroyCommandBase) getControllerEnvironFromStore(
	ctx *cmd.Context,
	store jujuclient.ClientStore,
	controllerName string,
) (environs.BootstrapEnviron, error) {
	bootstrapConfig, spec, cfg, err := modelcmd.NewGetBootstrapConfigParamsFunc(
		ctx, store, environs.GlobalProviderRegistry(),
	)(controllerName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	provider, err := environs.Provider(bootstrapConfig.CloudType)
	if err != nil {
		return nil, errors.Trace(err)
	}
	err = provider.ValidateCloud(ctx, *spec)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ctrlUUID, err := c.ControllerUUID(store, controllerName)
	if err != nil {
		return nil, errors.Trace(err)
	}

	openParams := environs.OpenParams{
		ControllerUUID: ctrlUUID,
		Cloud:          *spec,
		Config:         cfg,
	}
	if cloud.CloudTypeIsCAAS(bootstrapConfig.CloudType) {
		return caas.New(ctx, openParams, environs.NoopCredentialInvalidator())
	}
	return environs.New(ctx, openParams, environs.NoopCredentialInvalidator())
}

func (c *destroyCommandBase) getControllerEnvironFromAPI(
	ctx context.Context,
	api destroyControllerAPI,
	controllerModelConfigAPI modelConfigAPI,
) (environs.Environ, error) {
	if api == nil {
		return nil, errors.New(
			"unable to get bootstrap information from client store or API",
		)
	}
	attrs, err := controllerModelConfigAPI.ModelGet(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "getting model config from API")
	}
	cfg, err := config.New(config.NoDefaults, attrs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	cloudSpec, err := api.CloudSpec(ctx, names.NewModelTag(cfg.UUID()))
	if err != nil {
		return nil, errors.Annotate(err, "getting cloud spec from API")
	}
	ctrlCfg, err := api.ControllerConfig(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "getting controller config from API")
	}
	return environs.New(ctx, environs.OpenParams{
		ControllerUUID: ctrlCfg.ControllerUUID(),
		Cloud:          cloudSpec,
		Config:         cfg,
	}, environs.NoopCredentialInvalidator())
}
