// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/controller"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/configstore"
)

var (
	apiTimeout      = 10 * time.Second
	ErrConnTimedOut = errors.New("connection to controller timed out")
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
	return envcmd.WrapBase(&killCommand{})
}

// killCommand kills the specified controller.
type killCommand struct {
	destroyCommandBase
	// TODO (cherylj) If timeouts for dialing the API are added to new or
	// existing commands later, the dialer should be pulled into a common
	// base and made to be an interface rather than a function.
	apiDialerFunc func(string) (api.Connection, error)
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

func (c *killCommand) getControllerAPI(info configstore.EnvironInfo) (destroyControllerAPI, error) {
	if c.api != nil {
		return c.api, c.apierr
	}

	// Attempt to connect to the API with a short timeout
	apic := make(chan api.Connection)
	errc := make(chan error)
	go func() {
		api, dialErr := c.apiDialerFunc(c.controllerName)
		if dialErr != nil {
			errc <- dialErr
			return
		}
		apic <- api
	}()

	var apiRoot api.Connection
	select {
	case err := <-errc:
		return nil, err
	case apiRoot = <-apic:
	case <-time.After(apiTimeout):
		return nil, ErrConnTimedOut
	}

	return controller.NewClient(apiRoot), nil
}

// Run implements Command.Run
func (c *killCommand) Run(ctx *cmd.Context) error {
	if c.apiDialerFunc == nil {
		c.apiDialerFunc = c.NewAPIRoot
	}

	store, err := configstore.Default()
	if err != nil {
		return errors.Annotate(err, "cannot open controller info storage")
	}

	cfgInfo, err := store.ReadInfo(c.controllerName)
	if err != nil {
		return errors.Annotate(err, "cannot read controller info")
	}

	// Verify that we're destroying a controller
	apiEndpoint := cfgInfo.APIEndpoint()
	if apiEndpoint.ServerUUID != "" && apiEndpoint.EnvironUUID != apiEndpoint.ServerUUID {
		return errors.Errorf("%q is not a controller; use juju environment destroy to destroy it", c.controllerName)
	}

	if !c.assumeYes {
		if err = confirmDestruction(ctx, c.controllerName); err != nil {
			return err
		}
	}

	// Attempt to connect to the API.
	api, err := c.getControllerAPI(cfgInfo)
	switch {
	case err == nil:
		defer api.Close()
	case errors.Cause(err) == common.ErrPerm:
		return errors.Annotate(err, "cannot destroy controller")
	default:
		if err != ErrConnTimedOut {
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
		return environs.Destroy(controllerEnviron, store)
	}

	// Attempt to destroy the controller with destroyEnvs and ignoreBlocks = true
	err = api.DestroyController(true, true)
	if params.IsCodeNotImplemented(err) {
		// Fall back to using the client endpoint to destroy the controller,
		// sending the info we were already able to collect.
		return c.killControllerViaClient(ctx, cfgInfo, controllerEnviron, store)
	}

	if err != nil {
		ctx.Infof("Unable to destroy controller through the API: %s.  Destroying through provider.", err)
	}

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
