// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
	"launchpad.net/gnuflag"

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
		modelcmd.ControllerSkipFlags,
		modelcmd.ControllerSkipDefault,
	)
}

// destroyCommand destroys the specified controller.
type destroyCommand struct {
	destroyCommandBase
	destroyModels bool
}

var destroyDoc = `Destroys the specified controller`
var destroySysMsg = `
WARNING! This command will destroy the %q controller.
This includes all machines, services, data and other resources.

Continue [y/N]? `[1:]

// destroyControllerAPI defines the methods on the controller API endpoint
// that the destroy command calls.
type destroyControllerAPI interface {
	Close() error
	ModelConfig() (map[string]interface{}, error)
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
		Purpose: "terminate all machines and other associated resources for the juju controller",
		Doc:     destroyDoc,
	}
}

// SetFlags implements Command.SetFlags.
func (c *destroyCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.destroyModels, "destroy-all-models", false, "destroy all hosted models in the controller")
	c.destroyCommandBase.SetFlags(f)
}

// Run implements Command.Run
func (c *destroyCommand) Run(ctx *cmd.Context) error {
	controllerName := c.ControllerName()
	store := c.ClientStore()
	controllerDetails, err := store.ControllerByName(controllerName)
	if err != nil {
		return errors.Annotate(err, "cannot read controller info")
	}

	if !c.assumeYes {
		if err = confirmDestruction(ctx, c.ControllerName()); err != nil {
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

	// Attempt to destroy the controller.
	err = api.DestroyController(c.destroyModels)
	if err != nil {
		return c.ensureUserFriendlyErrorLog(errors.Annotate(err, "cannot destroy controller"), ctx, api)
	}

	ctx.Infof("Destroying controller %q", c.ControllerName())
	if c.destroyModels {
		ctx.Infof("Waiting for hosted model resources to be reclaimed.")

		updateStatus := newTimedStatusUpdater(ctx, api, controllerDetails.ControllerUUID)
		for ctrStatus, modelsStatus := updateStatus(0); hasUnDeadModels(modelsStatus); ctrStatus, modelsStatus = updateStatus(2 * time.Second) {
			ctx.Infof(fmtCtrStatus(ctrStatus))
			for _, model := range modelsStatus {
				ctx.Verbosef(fmtModelStatus(model))
			}
		}

		ctx.Infof("All hosted models reclaimed, cleaning up controller machines")
	}
	return environs.Destroy(c.ControllerName(), controllerEnviron, store)
}

// ensureUserFriendlyErrorLog ensures that error will be logged and displayed
// in a user-friendly manner with readable and digestable error message.
func (c *destroyCommand) ensureUserFriendlyErrorLog(destroyErr error, ctx *cmd.Context, api destroyControllerAPI) error {
	if destroyErr == nil {
		return nil
	}
	if params.IsCodeOperationBlocked(destroyErr) {
		logger.Errorf(`there are blocks preventing controller destruction
To remove all blocks in the controller, please run:

    juju controller remove-blocks

`)
		if api != nil {
			models, err := api.ListBlockedModels()
			var bytes []byte
			if err == nil {
				bytes, err = formatTabularBlockedModels(models)
			}

			if err != nil {
				logger.Errorf("Unable to list blocked models: %s", err)
				return cmd.ErrSilent
			}
			ctx.Infof(string(bytes))
		}
		return cmd.ErrSilent
	}
	logger.Errorf(stdFailureMsg, c.ControllerName())
	return destroyErr
}

var stdFailureMsg = `failed to destroy controller %q

If the controller is unusable, then you may run

    juju kill-controller

to forcibly destroy the controller. Upon doing so, review
your model provider console for any resources that need
to be cleaned up.
`

func formatTabularBlockedModels(value interface{}) ([]byte, error) {
	models, ok := value.([]params.ModelBlockInfo)
	if !ok {
		return nil, errors.Errorf("expected value of type %T, got %T", models, value)
	}

	var out bytes.Buffer
	const (
		// To format things into columns.
		minwidth = 0
		tabwidth = 1
		padding  = 2
		padchar  = ' '
		flags    = 0
	)
	tw := tabwriter.NewWriter(&out, minwidth, tabwidth, padding, padchar, flags)
	fmt.Fprintf(tw, "NAME\tMODEL UUID\tOWNER\tBLOCKS\n")
	for _, model := range models {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", model.Name, model.UUID, model.OwnerTag, blocksToStr(model.Blocks))
	}
	tw.Flush()
	return out.Bytes(), nil
}

func blocksToStr(blocks []string) string {
	result := ""
	sep := ""
	for _, blk := range blocks {
		result = result + sep + block.OperationFromType(blk)
		sep = ","
	}

	return result
}

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
	store jujuclient.ClientStore, controllerName string, sysAPI destroyControllerAPI,
) (_ environs.Environ, err error) {
	cfg, err := modelcmd.NewGetBootstrapConfigFunc(store)(controllerName)
	if errors.IsNotFound(err) {
		if sysAPI == nil {
			return nil, errors.New(
				"unable to get bootstrap information from client store or API",
			)
		}
		bootstrapConfig, err := sysAPI.ModelConfig()
		if err != nil {
			return nil, errors.Annotate(err, "getting bootstrap config from API")
		}
		cfg, err = config.New(config.NoDefaults, bootstrapConfig)
		if err != nil {
			return nil, errors.Trace(err)
		}
	} else if err != nil {
		return nil, errors.Annotate(err, "getting bootstrap config from client store")
	}
	return environs.New(cfg)
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
