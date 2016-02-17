// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/utils/clock"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/configstore"
)

const killDoc = `
Forcibly destroy the specified controller.  If the API server is accessible,
this command will attempt to destroy the controller model and all
hosted models and their resources.

If the API server is unreachable, the machines of the controller model
will be destroyed through the cloud provisioner.  If there are additional
machines, including machines within hosted models, these machines will
not be destroyed and will never be reconnected to the Juju controller being
destroyed. 
`

// NewKillCommand returns a command to kill a controller. Killing is a forceful
// destroy.
func NewKillCommand() cmd.Command {
	// Even though this command is all about killing a controller we end up
	// needing environment endpoints so we can fall back to the client destroy
	// environment method. This shouldn't really matter in practice as the
	// user trying to take down the controller will need to have access to the
	// controller environment anyway.
	return wrapKillCommand(&killCommand{}, nil, clock.WallClock)
}

// wrapKillCommand provides the common wrapping used by tests and
// the default NewKillCommand above.
func wrapKillCommand(kill *killCommand, apiOpen modelcmd.APIOpener, clock clock.Clock) cmd.Command {
	if apiOpen == nil {
		apiOpen = modelcmd.OpenFunc(kill.JujuCommandBase.NewAPIRoot)
	}
	openStrategy := modelcmd.NewTimeoutOpener(apiOpen, clock, 10*time.Second)
	return modelcmd.WrapController(
		kill,
		modelcmd.ControllerSkipFlags,
		modelcmd.ControllerSkipDefault,
		modelcmd.ControllerAPIOpener(openStrategy),
	)
}

// killCommand kills the specified controller.
type killCommand struct {
	destroyCommandBase
}

// Info implements Command.Info.
func (c *killCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "kill-controller",
		Args:    "<controller name>",
		Purpose: "forcibly terminate all machines and other associated resources for a juju controller",
		Doc:     killDoc,
	}
}

// SetFlags implements Command.SetFlags.
func (c *killCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.assumeYes, "y", false, "do not ask for confirmation")
	f.BoolVar(&c.assumeYes, "yes", false, "")
}

// Init implements Command.Init.
func (c *killCommand) Init(args []string) error {
	return c.destroyCommandBase.Init(args)
}

// Run implements Command.Run
func (c *killCommand) Run(ctx *cmd.Context) error {
	store, err := configstore.Default()
	if err != nil {
		return errors.Annotate(err, "cannot open controller info storage")
	}

	controllerName := c.ControllerName()
	cfgInfo, err := store.ReadInfo(configstore.EnvironInfoName(
		controllerName, configstore.AdminModelName(controllerName),
	))
	if err != nil {
		return errors.Annotate(err, "cannot read controller info")
	}

	// Verify that we're destroying a controller
	apiEndpoint := cfgInfo.APIEndpoint()
	if apiEndpoint.ServerUUID != "" && apiEndpoint.ModelUUID != apiEndpoint.ServerUUID {
		return errors.Errorf("%q is not a controller; use juju model destroy to destroy it", c.ControllerName())
	}

	if !c.assumeYes {
		if err = confirmDestruction(ctx, c.ControllerName()); err != nil {
			return err
		}
	}

	// Attempt to connect to the API.
	api, err := c.getControllerAPI()
	switch {
	case err == nil:
		defer api.Close()
	case errors.Cause(err) == common.ErrPerm:
		return errors.Annotate(err, "cannot destroy controller")
	default:
		if errors.Cause(err) != modelcmd.ErrConnTimedOut {
			logger.Debugf("unable to open api: %s", err)
		}
		ctx.Infof("Unable to open API: %s\n", err)
		api = nil
	}

	// Obtain bootstrap / controller environ information
	controllerEnviron, err := c.getControllerEnviron(cfgInfo, api)
	if err != nil {
		return errors.Annotate(err, "cannot obtain bootstrap information")
	}

	// If we were unable to connect to the API, just destroy the controller through
	// the environs interface.
	if api == nil {
		ctx.Infof("Unable to connect to the API server. Destroying through provider.")
		return environs.Destroy(c.ControllerName(), controllerEnviron, store, c.ClientStore())
	}

	// Attempt to destroy the controller and all environments.
	err = api.DestroyController(true)
	if err != nil {
		ctx.Infof("Unable to destroy controller through the API: %s.  Destroying through provider.", err)
		return environs.Destroy(c.ControllerName(), controllerEnviron, store, c.ClientStore())
	}

	ctx.Infof("Destroying controller %q\nWaiting for resources to be reclaimed", c.ControllerName())

	updateStatus := newTimedStatusUpdater(ctx, api, apiEndpoint.ModelUUID)
	for ctrStatus, envsStatus := updateStatus(0); hasUnDeadEnvirons(envsStatus); ctrStatus, envsStatus = updateStatus(2 * time.Second) {
		ctx.Infof(fmtCtrStatus(ctrStatus))
		for _, envStatus := range envsStatus {
			ctx.Verbosef(fmtEnvStatus(envStatus))
		}
	}

	ctx.Infof("All hosted models reclaimed, cleaning up controller machines")

	return environs.Destroy(c.ControllerName(), controllerEnviron, store, c.ClientStore())
}
