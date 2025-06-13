// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"bufio"
	"bytes"
	stdcontext "context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/juju/clock"
	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/client/credentialmanager"
	"github.com/juju/juju/api/client/storage"
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
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/rpc/params"
)

// NewDestroyCommand returns a command to destroy a controller.
func NewDestroyCommand() cmd.Command {
	cmd := destroyCommand{}
	cmd.controllerCredentialAPIFunc = cmd.credentialAPIForControllerModel
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
	storageAPI     storageAPI
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
All models (initial model plus all workload/hosted) associated with the
controller will first need to be destroyed, either in advance, or by
specifying `[1:] + "`--destroy-all-models`." + `

If there is persistent storage in any of the models managed by the
controller, then you must choose to either destroy or release the
storage, using ` + "`--destroy-storage` or `--release-storage` respectively." + `

Sometimes, the destruction of a hosted model may fail as Juju encounters errors
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

Examples:
    # Destroy the controller and all hosted models. If there is
    # persistent storage remaining in any of the models, then
    # this will prompt you to choose to either destroy or release
    # the storage.
    juju destroy-controller --destroy-all-models mycontroller

    # Destroy the controller and all hosted models, destroying
    # any remaining persistent storage.
    juju destroy-controller --destroy-all-models --destroy-storage

    # Destroy the controller and all hosted models, releasing
    # any remaining persistent storage from Juju's control.
    juju destroy-controller --destroy-all-models --release-storage

    # Destroy the controller and all hosted models, continuing
    # even if there are operational errors.
    juju destroy-controller --destroy-all-models --force
    juju destroy-controller --destroy-all-models --force --no-wait

See also:
    kill-controller
    unregister`

var usageSummary = `
Destroys a controller.`[1:]

var destroySysMsg = `
WARNING! This command will destroy the %q controller.
This includes all machines, applications, data and other resources.

Continue? (y/N):`[1:]

// destroyControllerAPI defines the methods on the controller API endpoint
// that the destroy command calls.
type destroyControllerAPI interface {
	Close() error
	BestAPIVersion() int
	ModelConfig() (map[string]interface{}, error)
	HostedModelConfigs() ([]controllerapi.HostedConfig, error)
	CloudSpec(names.ModelTag) (environscloudspec.CloudSpec, error)
	DestroyController(controllerapi.DestroyControllerParams) error
	ListBlockedModels() ([]params.ModelBlockInfo, error)
	ModelStatus(models ...names.ModelTag) ([]base.ModelStatus, error)
	AllModels() ([]base.UserModel, error)
	ControllerConfig() (controller.Config, error)
}

type storageAPI interface {
	Close() error
	ListStorageDetails() ([]params.StorageDetails, error)
}

// destroyClientAPI defines the methods on the client API endpoint that the
// destroy command might call.
type destroyClientAPI interface {
	Close() error
	ModelGet() (map[string]interface{}, error)
	DestroyModel() error
}

// Info implements Command.Info.
func (c *destroyCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "destroy-controller",
		Args:    "<controller name>",
		Purpose: usageSummary,
		Doc:     usageDetails,
	})
}

const unsetTimeout = -1 * time.Second

// SetFlags implements Command.SetFlags.
func (c *destroyCommand) SetFlags(f *gnuflag.FlagSet) {
	c.destroyCommandBase.SetFlags(f)
	f.BoolVar(&c.destroyModels, "destroy-all-models", false, "Destroy all hosted models in the controller")
	f.BoolVar(&c.destroyStorage, "destroy-storage", false, "Destroy all storage instances managed by the controller")
	f.BoolVar(&c.releaseStorage, "release-storage", false, "Release all storage instances from management of the controller, without destroying them")
	f.DurationVar(&c.modelTimeout, "model-timeout", unsetTimeout, "Timeout for each step of force model destruction")
	f.BoolVar(&c.force, "force", false, "Force destroy hosted models ignoring any errors")
	f.BoolVar(&c.noWait, "no-wait", false, "Rush through hosted model destruction without waiting for each individual step to complete")
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

// Run implements Command.Run
func (c *destroyCommand) Run(ctx *cmd.Context) error {
	controllerName, err := c.ControllerName()
	if err != nil {
		return errors.Trace(err)
	}
	store := c.ClientStore()
	if !c.assumeYes {
		if err := confirmDestruction(ctx, controllerName); err != nil {
			return err
		}
	}

	// Attempt to connect to the API.  If we can't, fail the destroy.  Users will
	// need to use the controller kill command if we can't connect.
	api, err := c.getControllerAPI()
	if err != nil {
		return c.ensureUserFriendlyErrorLog(errors.Annotate(err, "cannot connect to API"), ctx, nil)
	}
	defer api.Close()

	if api.BestAPIVersion() < 4 {
		// Versions before 4 support only destroying the storage,
		// and will not raise an error if there is storage in the
		// controller. Force the user to specify up-front.
		if c.releaseStorage {
			return errors.New("this juju controller only supports destroying storage")
		}
		if !c.destroyStorage {
			models, err := api.AllModels()
			if err != nil {
				return errors.Trace(err)
			}
			var anyStorage bool
			for _, model := range models {
				hasStorage, err := c.modelHasStorage(model.Name)
				if err != nil {
					return errors.Trace(err)
				}
				if hasStorage {
					anyStorage = true
					break
				}
			}
			if anyStorage {
				return errors.Errorf(`cannot destroy controller %q

Destroying this controller will destroy the storage,
but you have not indicated that you want to do that.

Please run the the command again with --destroy-storage
to confirm that you want to destroy the storage along
with the controller.

If instead you want to keep the storage, you must first
upgrade the controller to version 2.3 or greater.

`, controllerName)
			}
			c.destroyStorage = true
		}
	}

	// Obtain controller environ so we can clean up afterwards.
	controllerEnviron, err := c.getControllerEnviron(ctx, store, controllerName, api)
	if err != nil {
		return errors.Annotate(err, "getting controller environ")
	}

	cloudCallCtx := cloudCallContext(c.controllerCredentialAPIFunc)

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

		err = api.DestroyController(controllerapi.DestroyControllerParams{
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

		updateStatus := newTimedStatusUpdater(ctx, api, controllerEnviron.Config().UUID(), clock.WallClock)
		// wait for 2 seconds to let empty hosted models changed from alive to dying.
		modelStatus := updateStatus(0)
		if !c.destroyModels {
			if err := c.checkNoAliveHostedModels(ctx, modelStatus.models); err != nil {
				return errors.Trace(err)
			}
			if hasHostedModels && !hasUnDeadModels(modelStatus.models) {
				// When we called DestroyController before, we were
				// informed that there were hosted models remaining.
				// When we checked just now, there were none. We should
				// try destroying again.
				continue
			}
		}
		if !c.destroyStorage && !c.releaseStorage && hasPersistentStorage {
			if err := c.checkNoPersistentStorage(ctx, modelStatus); err != nil {
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
		ctx.Infof("Waiting for hosted model resources to be reclaimed")
		for ; hasUnreclaimedResources(modelStatus); modelStatus = updateStatus(2 * time.Second) {
			ctx.Infof("%s", fmtCtrStatus(modelStatus.controller))
			for _, model := range modelStatus.models {
				ctx.Verbosef("%s", fmtModelStatus(model))
			}
		}
		ctx.Infof("All hosted models reclaimed, cleaning up controller machines")
		return c.environsDestroy(controllerName, controllerEnviron, cloudCallCtx, store)
	}
}

func (c *destroyCommand) modelHasStorage(modelName string) (bool, error) {
	client, err := c.getStorageAPI(modelName)
	if err != nil {
		return false, errors.Trace(err)
	}
	defer client.Close()

	storage, err := client.ListStorageDetails()
	if err != nil {
		return false, errors.Trace(err)
	}
	return len(storage) > 0, nil
}

// checkNoAliveHostedModels ensures that the given set of hosted models
// contains none that are Alive. If there are, an message is printed
// out to
func (c *destroyCommand) checkNoAliveHostedModels(ctx *cmd.Context, models []modelData) error {
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

The controller has live hosted models. If you want
to destroy all hosted models in the controller,
run this command again with the --destroy-all-models
option.

Models:
%s`, controllerName, buf.String())
}

// checkNoPersistentStorage ensures that the controller contains
// no persistent storage. If there is any, a message is printed
// out informing the user that they must choose to destroy or
// release the storage.
func (c *destroyCommand) checkNoPersistentStorage(ctx *cmd.Context, envStatus environmentStatus) error {
	models := append([]modelData{envStatus.controller.Model}, envStatus.models...)

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
	if n := modelsWithPersistentStorage; n == 1 {
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
		logger.Errorf(destroyControllerBlockedMsg)
		if api != nil {
			models, err := api.ListBlockedModels()
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
				logger.Errorf("Unable to list models: %s", err)
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
	logger.Errorf(stdFailureMsg, controllerName)
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
	assumeYes bool

	// The following fields are for mocking out
	// api behavior for testing.
	api       destroyControllerAPI
	apierr    error
	clientapi destroyClientAPI

	controllerCredentialAPIFunc newCredentialAPIFunc

	environsDestroy func(string, environs.ControllerDestroyer, context.ProviderCallContext, jujuclient.ControllerStore) error
}

func (c *destroyCommandBase) getControllerAPI() (destroyControllerAPI, error) {
	// Note that some tests set c.api to a non-nil value
	// even when c.apierr is non-nil, hence the separate test.
	if c.apierr != nil {
		return nil, c.apierr
	}
	if c.api != nil {
		return c.api, nil
	}
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return controllerapi.NewClient(root), nil
}

func (c *destroyCommand) getStorageAPI(modelName string) (storageAPI, error) {
	if c.storageAPI != nil {
		return c.storageAPI, nil
	}
	root, err := c.NewModelAPIRoot(modelName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return storage.NewClient(root), nil
}

// SetFlags implements Command.SetFlags.
func (c *destroyCommandBase) SetFlags(f *gnuflag.FlagSet) {
	c.ControllerCommandBase.SetFlags(f)
	f.BoolVar(&c.assumeYes, "y", false, "Do not ask for confirmation")
	f.BoolVar(&c.assumeYes, "yes", false, "")
	f.BoolVar(&c.assumeYes, "no-prompt", false, "")
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
) (environs.BootstrapEnviron, error) {
	// TODO: (hml) 2018-08-01
	// We should try to destroy via the API first, from store is a
	// fall back position.
	env, err := c.getControllerEnvironFromStore(ctx, store, controllerName)
	if errors.IsNotFound(err) {
		return c.getControllerEnvironFromAPI(sysAPI, controllerName)
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
	_, params, err := modelcmd.NewGetBootstrapConfigParamsFunc(
		ctx, store, environs.GlobalProviderRegistry(),
	)(controllerName)
	if err != nil {
		return environscloudspec.CloudSpec{}, errors.Trace(err)
	}

	return params.Cloud, nil
}

func (c *destroyCommandBase) getControllerEnvironFromStore(
	ctx *cmd.Context,
	store jujuclient.ClientStore,
	controllerName string,
) (environs.BootstrapEnviron, error) {
	bootstrapConfig, params, err := modelcmd.NewGetBootstrapConfigParamsFunc(
		ctx, store, environs.GlobalProviderRegistry(),
	)(controllerName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	provider, err := environs.Provider(bootstrapConfig.CloudType)
	if err != nil {
		return nil, errors.Trace(err)
	}
	cfg, err := provider.PrepareConfig(*params)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ctrlUUID, err := c.ControllerUUID(store, controllerName)
	if err != nil {
		return nil, errors.Trace(err)
	}

	openParams := environs.OpenParams{
		ControllerUUID: ctrlUUID,
		Cloud:          params.Cloud,
		Config:         cfg,
	}
	if bootstrapConfig.CloudType == cloud.CloudTypeCAAS {
		return caas.New(stdcontext.TODO(), openParams)
	}
	return environs.New(stdcontext.TODO(), openParams)
}

func (c *destroyCommandBase) getControllerEnvironFromAPI(
	api destroyControllerAPI,
	controllerName string,
) (environs.Environ, error) {
	if api == nil {
		return nil, errors.New(
			"unable to get bootstrap information from client store or API",
		)
	}
	attrs, err := api.ModelConfig()
	if err != nil {
		return nil, errors.Annotate(err, "getting model config from API")
	}
	cfg, err := config.New(config.NoDefaults, attrs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	cloudSpec, err := api.CloudSpec(names.NewModelTag(cfg.UUID()))
	if err != nil {
		return nil, errors.Annotate(err, "getting cloud spec from API")
	}
	ctrlCfg, err := api.ControllerConfig()
	if err != nil {
		return nil, errors.Annotate(err, "getting controller config from API")
	}
	return environs.New(stdcontext.TODO(), environs.OpenParams{
		ControllerUUID: ctrlCfg.ControllerUUID(),
		Cloud:          cloudSpec,
		Config:         cfg,
	})
}

func confirmDestruction(ctx *cmd.Context, controllerName string) error {
	// Get confirmation from the user that they want to continue
	fmt.Fprintf(ctx.Stdout, destroySysMsg, controllerName)

	scanner := bufio.NewScanner(ctx.Stdin)
	scanner.Scan()
	err := scanner.Err()
	if err != nil && err != io.EOF {
		return errors.Annotate(err, "controller destruction aborted")
	}
	answer := strings.ToLower(scanner.Text())
	if answer != "y" && answer != "yes" {
		return errors.New("controller destruction aborted")
	}

	return nil
}

// CredentialAPI defines the methods on the credential API endpoint that the
// destroy command might call.
type CredentialAPI interface {
	InvalidateModelCredential(string) error
	Close() error
}

func (c *destroyCommandBase) credentialAPIForControllerModel() (CredentialAPI, error) {
	// Note that the api here needs to operate on a controller model itself,
	// as the controller model's cloud credential is the controller cloud credential.
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return credentialmanager.NewClient(root), nil
}

type newCredentialAPIFunc func() (CredentialAPI, error)

func cloudCallContext(newAPIFunc newCredentialAPIFunc) context.ProviderCallContext {
	callCtx := context.NewCloudCallContext(stdcontext.Background())
	callCtx.InvalidateCredentialFunc = func(reason string) error {
		api, err := newAPIFunc()
		if err != nil {
			return errors.Trace(err)
		}
		defer api.Close()
		return api.InvalidateModelCredential(reason)
	}
	return callCtx
}
