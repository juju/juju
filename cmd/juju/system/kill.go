// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package system

import (
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/systemmanager"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/juju"
)

var (
	killDoc         = `Forcefully destroys the specified system`
	DialAPI         = juju.NewAPIFromName
	apiTimeout      = 10 * time.Second
	ErrConnTimedOut = errors.New("connection to state server timed out")
)

// KillCommand kills the specified system.
type KillCommand struct {
	DestroyCommand
}

// Info implements Command.Info.
func (c *KillCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "kill",
		Args:    "<system name>",
		Purpose: "forcefully terminate all machines and other associated resources for a system environment",
		Doc:     killDoc,
	}
}

// SetFlags implements Command.SetFlags.
func (c *KillCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.assumeYes, "y", false, "Do not ask for confirmation")
	f.BoolVar(&c.assumeYes, "yes", false, "")
}

func (c *KillCommand) getSystemAPI(info configstore.EnvironInfo) (destroySystemAPI, error) {
	if c.api != nil {
		return c.api, c.apierr
	}

	// Attempt to connect to the API with a short timeout
	apic := make(chan *api.State)
	errc := make(chan error)
	go func() {
		api, dialErr := DialAPI(c.systemName)
		if dialErr != nil {
			errc <- dialErr
			return
		}
		apic <- api
	}()

	var apiRoot *api.State
	select {
	case err := <-errc:
		return nil, err
	case apiRoot = <-apic:
	case <-time.After(apiTimeout):
		return nil, ErrConnTimedOut
	}

	return systemmanager.NewClient(apiRoot), nil
}

// Run implements Command.Run
func (c *KillCommand) Run(ctx *cmd.Context) error {
	store, err := configstore.Default()
	if err != nil {
		return errors.Annotate(err, "cannot open system info storage")
	}

	cfgInfo, err := store.ReadInfo(c.systemName)
	if err != nil {
		return errors.Annotate(err, "cannot read system info")
	}

	// Verify that we're destroying a system
	apiEndpoint := cfgInfo.APIEndpoint()
	if apiEndpoint.ServerUUID != "" && apiEndpoint.EnvironUUID != apiEndpoint.ServerUUID {
		return errors.Errorf("%q is not a system; use juju environment destroy to destroy it", c.systemName)
	}

	if !c.assumeYes {
		if err = confirmDestruction(ctx, c.systemName); err != nil {
			return err
		}
	}

	// Attempt to connect to the API.
	api, err := c.getSystemAPI(cfgInfo)
	switch {
	case err == nil:
		defer api.Close()
	case errors.Cause(err) == common.ErrPerm:
		return errors.Annotate(err, "cannot destroy system")
	case errors.IsNotFound(err):
		// TODO (cherylj) display message about how to clean up jenv / cache file
		// once a command is created to do so?
		return errors.Annotate(err, "cannot destroy system")
	default:
		logger.Infof("Unable to open API: %s\n", err)
		api = nil
	}

	// Obtain bootstrap / system environ information
	systemEnviron, err := c.getSystemEnviron(cfgInfo, api)
	if err != nil {
		return errors.Annotate(err, "cannot obtain bootstrap information")
	}

	// If we were unable to connect to the API, just destroy the system through
	// the environs interface.
	if api == nil {
		return environs.Destroy(systemEnviron, store)
	}

	// Attempt to destroy the system with destroyEnvs and ignoreBlocks = true
	err = api.DestroySystem(true, true)
	if params.IsCodeNotImplemented(err) {
		// Fall back to using the client endpoint to destroy the system,
		// sending the info we were already able to collect.
		return c.killSystemViaClient(ctx, cfgInfo, systemEnviron, store)
	}

	if err != nil {
		logger.Infof("Unable to destroy system through the API: %s.  Destroying through provider.", err)
	}

	return environs.Destroy(systemEnviron, store)
}

// killSystemViaClient attempts to kill the system using the client
// endpoint for older juju systems which do not implement systemmanager.DestroySystem
func (c *KillCommand) killSystemViaClient(ctx *cmd.Context, info configstore.EnvironInfo, systemEnviron environs.Environ, store configstore.Storage) error {
	api, err := c.getClientAPI()
	if err != nil {
		defer api.Close()
	}

	if api != nil {
		err = api.DestroyEnvironment()
		if err != nil {
			logger.Infof("Unable to destroy system through the API: %s.  Destroying through provider.", err)
		}
	}

	return environs.Destroy(systemEnviron, store)
}
