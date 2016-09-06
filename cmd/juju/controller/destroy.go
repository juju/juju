// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/controller"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/jujuclient"
)

// NewDestroyCommand returns a command to destroy a controller.
func NewDestroyCommand() cmd.Command {
	// Even though this command is all about destroying a controller we end up
	// needing environment endpoints so we can fall back to the client destroy
	// environment method. This shouldn't really matter in practice as the
	// user trying to take down the controller will need to have access to the
	// controller environment anyway.
	return modelcmd.WrapController(
		&destroyCommand{},
		modelcmd.WrapControllerSkipControllerFlags,
		modelcmd.WrapControllerSkipDefaultController,
	)
}

// destroyCommand destroys the specified controller.
type destroyCommand struct {
	destroyCommandBase
	destroyModels bool
}

// usageDetails has backticks which we want to keep for markdown processing.
// TODO(cheryl): Do we want the usage, options, examples, and see also text in
// backticks for markdown?
var usageDetails = `
All models (initial model plus all workload/hosted) associated with the
controller will first need to be destroyed, either in advance, or by
specifying `[1:] + "`--destroy-all-models`." + `

Examples:
    juju destroy-controller --destroy-all-models mycontroller

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
	ModelConfig() (map[string]interface{}, error)
	CloudSpec(names.ModelTag) (environs.CloudSpec, error)
	DestroyController(destroyModels bool) error
	ListBlockedModels() ([]params.ModelBlockInfo, error)
	ModelStatus(models ...names.ModelTag) ([]base.ModelStatus, error)
	AllModels() ([]base.UserModel, error)
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
	return &cmd.Info{
		Name:    "destroy-controller",
		Args:    "<controller name>",
		Purpose: usageSummary,
		Doc:     usageDetails,
	}
}

// SetFlags implements Command.SetFlags.
func (c *destroyCommand) SetFlags(f *gnuflag.FlagSet) {
	c.destroyCommandBase.SetFlags(f)
	f.BoolVar(&c.destroyModels, "destroy-all-models", false, "Destroy all hosted models in the controller")
}

// Run implements Command.Run
func (c *destroyCommand) Run(ctx *cmd.Context) error {
	controllerName := c.ControllerName()
	store := c.ClientStore()
	if !c.assumeYes {
		if err := confirmDestruction(ctx, c.ControllerName()); err != nil {
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

	// Obtain controller environ so we can clean up afterwards.
	controllerEnviron, err := c.getControllerEnviron(store, controllerName, api)
	if err != nil {
		return errors.Annotate(err, "getting controller environ")
	}

	for {
		// Attempt to destroy the controller.
		ctx.Infof("Destroying controller")
		var hasHostedModels bool
		err = api.DestroyController(c.destroyModels)
		if err != nil {
			if params.IsCodeHasHostedModels(err) {
				hasHostedModels = true
			} else {
				return c.ensureUserFriendlyErrorLog(
					errors.Annotate(err, "cannot destroy controller"),
					ctx, api,
				)
			}
		}

		updateStatus := newTimedStatusUpdater(ctx, api, controllerEnviron.Config().UUID())
		ctrStatus, modelsStatus := updateStatus(0)
		if !c.destroyModels {
			if err := c.checkNoAliveHostedModels(ctx, modelsStatus); err != nil {
				return errors.Trace(err)
			}
			if hasHostedModels && !hasUnDeadModels(modelsStatus) {
				// When we called DestroyController before, we were
				// informed that there were hosted models remaining.
				// When we checked just now, there were none. We should
				// try destroying again.
				continue
			}
		}

		// Even if we've not just requested for hosted models to be destroyed,
		// there may be some being destroyed already. We need to wait for them.
		ctx.Infof("Waiting for hosted model resources to be reclaimed")
		for ; hasUnDeadModels(modelsStatus); ctrStatus, modelsStatus = updateStatus(2 * time.Second) {
			ctx.Infof(fmtCtrStatus(ctrStatus))
			for _, model := range modelsStatus {
				ctx.Verbosef(fmtModelStatus(model))
			}
		}
		ctx.Infof("All hosted models reclaimed, cleaning up controller machines")
		return environs.Destroy(c.ControllerName(), controllerEnviron, store)
	}
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
		if model.Life != params.Alive {
			continue
		}
		buf.WriteString(fmtModelStatus(model))
		buf.WriteRune('\n')
	}
	return errors.Errorf(`cannot destroy controller %q

The controller has live hosted models. If you want
to destroy all hosted models in the controller,
run this command again with the --destroy-all-models
flag.

Models:
%s`, c.ControllerName(), buf.String())
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
				logger.Errorf("Unable to list blocked models: %s", err)
				return cmd.ErrSilent
			}
			ctx.Infof(out.String())
		}
		return cmd.ErrSilent
	}
	if params.IsCodeHasHostedModels(destroyErr) {
		return destroyErr
	}
	logger.Errorf(stdFailureMsg, c.ControllerName())
	return destroyErr
}

const destroyControllerBlockedMsg = `there are blocks preventing controller destruction
To remove all blocks in the controller, please run:

    juju controller remove-blocks

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
}

func (c *destroyCommandBase) getControllerAPI() (destroyControllerAPI, error) {
	if c.api != nil {
		return c.api, c.apierr
	}
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return controller.NewClient(root), nil
}

// SetFlags implements Command.SetFlags.
func (c *destroyCommandBase) SetFlags(f *gnuflag.FlagSet) {
	c.ControllerCommandBase.SetFlags(f)
	f.BoolVar(&c.assumeYes, "y", false, "Do not ask for confirmation")
	f.BoolVar(&c.assumeYes, "yes", false, "")
}

// Init implements Command.Init.
func (c *destroyCommandBase) Init(args []string) error {
	switch len(args) {
	case 0:
		return errors.New("no controller specified")
	case 1:
		return c.SetControllerName(args[0])
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
	store jujuclient.ClientStore,
	controllerName string,
	sysAPI destroyControllerAPI,
) (environs.Environ, error) {
	env, err := c.getControllerEnvironFromStore(store, controllerName)
	if errors.IsNotFound(err) {
		return c.getControllerEnvironFromAPI(sysAPI, controllerName)
	} else if err != nil {
		return nil, errors.Annotate(err, "getting environ using bootstrap config from client store")
	}
	return env, nil
}

func (c *destroyCommandBase) getControllerEnvironFromStore(
	store jujuclient.ClientStore,
	controllerName string,
) (environs.Environ, error) {
	bootstrapConfig, params, err := modelcmd.NewGetBootstrapConfigParamsFunc(store)(controllerName)
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
	return environs.New(environs.OpenParams{
		Cloud:  params.Cloud,
		Config: cfg,
	})
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
	return environs.New(environs.OpenParams{
		Cloud:  cloudSpec,
		Config: cfg,
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
