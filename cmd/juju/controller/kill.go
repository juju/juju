// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/utils/clock"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs"
)

const killDoc = `
Forcibly destroy the specified controller.  If the API server is accessible,
this command will attempt to destroy the controller model and all hosted models
and their resources.

If the API server is unreachable, the machines of the controller model will be
destroyed through the cloud provisioner.  If there are additional machines,
including machines within hosted models, these machines will not be destroyed
and will never be reconnected to the Juju controller being destroyed.

See also:
    destroy-controller
    unregister
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
		modelcmd.WrapControllerSkipControllerFlags,
		modelcmd.WrapControllerSkipDefaultController,
		modelcmd.WrapControllerAPIOpener(openStrategy),
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
		Purpose: "Forcibly terminate all machines and other associated resources for a Juju controller.",
		Doc:     killDoc,
	}
}

// Init implements Command.Init.
func (c *killCommand) Init(args []string) error {
	return c.destroyCommandBase.Init(args)
}

// Run implements Command.Run
func (c *killCommand) Run(ctx *cmd.Context) error {
	controllerName := c.ControllerName()
	store := c.ClientStore()
	if !c.assumeYes {
		if err := confirmDestruction(ctx, controllerName); err != nil {
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

	// Obtain controller environ so we can clean up afterwards.
	controllerEnviron, err := c.getControllerEnviron(store, controllerName, api)
	if err != nil {
		return errors.Annotate(err, "getting controller environ")
	}
	// If we were unable to connect to the API, just destroy the controller through
	// the environs interface.
	if api == nil {
		ctx.Infof("Unable to connect to the API server. Destroying through provider.")
		return environs.Destroy(controllerName, controllerEnviron, store)
	}

	// Attempt to destroy the controller and all environments.
	err = api.DestroyController(true)
	if err != nil {
		ctx.Infof("Unable to destroy controller through the API: %s.  Destroying through provider.", err)
		return environs.Destroy(controllerName, controllerEnviron, store)
	}

	ctx.Infof("Destroying controller %q\nWaiting for resources to be reclaimed", controllerName)

	updateStatus := newTimedStatusUpdater(ctx, api, controllerEnviron.Config().UUID())
	for ctrStatus, envsStatus := updateStatus(0); hasUnDeadModels(envsStatus); ctrStatus, envsStatus = updateStatus(2 * time.Second) {
		ctx.Infof(fmtCtrStatus(ctrStatus))
		for _, envStatus := range envsStatus {
			ctx.Verbosef(fmtModelStatus(envStatus))
		}
	}

	ctx.Infof("All hosted models reclaimed, cleaning up controller machines")

	return environs.Destroy(controllerName, controllerEnviron, store)
}
