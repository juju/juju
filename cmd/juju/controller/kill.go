// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/utils/clock"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/configstore"
)

const killDoc = `
Forcibly destroy the specified controller.  If the API server is accessible,
this command will attempt to destroy the controller environment and all
hosted environments and their resources.

If the API server is unreachable, the machines of the controller environment
will be destroyed through the cloud provisioner.  If there are additional
machines, including machines within hosted environments, these machines will
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
func wrapKillCommand(kill *killCommand, fn func(string) (api.Connection, error), clock clock.Clock) cmd.Command {
	if fn == nil {
		fn = kill.JujuCommandBase.NewAPIRoot
	}
	openStrategy := envcmd.NewTimeoutOpener(fn, clock, 10*time.Second)
	return envcmd.Wrap(
		kill,
		envcmd.EnvSkipFlags,
		envcmd.EnvSkipDefault,
		envcmd.EnvAPIOpener(openStrategy),
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

	cfgInfo, err := store.ReadInfo(c.EnvName())
	if err != nil {
		return errors.Annotate(err, "cannot read controller info")
	}

	// Verify that we're destroying a controller
	apiEndpoint := cfgInfo.APIEndpoint()
	if apiEndpoint.ServerUUID != "" && apiEndpoint.EnvironUUID != apiEndpoint.ServerUUID {
		return errors.Errorf("%q is not a controller; use juju environment destroy to destroy it", c.EnvName())
	}

	if !c.assumeYes {
		if err = confirmDestruction(ctx, c.EnvName()); err != nil {
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
		if errors.Cause(err) != envcmd.ErrConnTimedOut {
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
		return environs.Destroy(controllerEnviron, store)
	}

	// Attempt to destroy the controller and all environments.
	err = api.DestroyController(true)
	if params.IsCodeNotImplemented(err) {
		// Fall back to using the client endpoint to destroy the controller,
		// sending the info we were already able to collect.
		ctx.Infof("Unable to destroy controller through the API: %s.  Destroying through provider.", err)
		return c.killControllerViaClient(ctx, cfgInfo, controllerEnviron, store)
	}

	if err != nil {
		ctx.Infof("Unable to destroy controller through the API: %s.  Destroying through provider.", err)
		return environs.Destroy(controllerEnviron, store)
	}

	ctx.Infof("Destroying controller %q\nWaiting for resources to be reclaimed", c.EnvName())

	updateStatus := newTimedStatusUpdater(ctx, api, apiEndpoint.EnvironUUID)
	for ctrStatus, envsStatus := updateStatus(0); hasUnDeadEnvirons(envsStatus); ctrStatus, envsStatus = updateStatus(2 * time.Second) {
		ctx.Infof(fmtCtrStatus(ctrStatus))
		for _, envStatus := range envsStatus {
			ctx.Verbosef(fmtEnvStatus(envStatus))
		}
	}

	ctx.Infof("All hosted environments reclaimed, cleaning up controller machines")

	return environs.Destroy(controllerEnviron, store)
}

// killControllerViaClient attempts to kill the controller using the client
// endpoint for older juju controllers which do not implement
// controller.DestroyController
func (c *killCommand) killControllerViaClient(ctx *cmd.Context, info configstore.EnvironInfo, controllerEnviron environs.Environ, store configstore.Storage) error {
	api, err := c.getClientAPI()
	if err != nil {
		defer api.Close()
	}

	if api != nil {
		err = api.DestroyEnvironment()
		if err != nil {
			ctx.Infof("Unable to destroy controller through the API: %s.  Destroying through provider.", err)
		}
	}

	return environs.Destroy(controllerEnviron, store)
}
