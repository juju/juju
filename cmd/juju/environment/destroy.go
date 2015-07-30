// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for infos.

package environment

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/apiserver/params"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/configstore"
)

// DestroyCommand destroys the specified environment.
type DestroyCommand struct {
	envcmd.EnvCommandBase
	envName   string
	assumeYes bool
	api       DestroyEnvironmentAPI
}

var destroyDoc = `Destroys the specified environment`
var destroyEnvMsg = `
WARNING! This command will destroy the %q environment.
This includes all machines, services, data and other resources.

Continue [y/N]? `[1:]

// DestroyEnvironmentAPI defines the methods on the environmentmanager
// API that the destroy command calls. It is exported for mocking in tests.
type DestroyEnvironmentAPI interface {
	Close() error
	DestroyEnvironment() error
}

// Info implements Command.Info.
func (c *DestroyCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "destroy",
		Args:    "<environment name>",
		Purpose: "terminate all machines and other associated resources for a non-system environment",
		Doc:     destroyDoc,
	}
}

// SetFlags implements Command.SetFlags.
func (c *DestroyCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.assumeYes, "y", false, "Do not ask for confirmation")
	f.BoolVar(&c.assumeYes, "yes", false, "")
}

// Init implements Command.Init.
func (c *DestroyCommand) Init(args []string) error {
	switch len(args) {
	case 0:
		return errors.New("no environment specified")
	case 1:
		c.envName = args[0]
		c.SetEnvName(c.envName)
		return nil
	default:
		return cmd.CheckEmpty(args[1:])
	}
}

func (c *DestroyCommand) getAPI() (DestroyEnvironmentAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	return c.NewAPIClient()
}

// Run implements Command.Run
func (c *DestroyCommand) Run(ctx *cmd.Context) error {
	store, err := configstore.Default()
	if err != nil {
		return errors.Annotate(err, "cannot open environment info storage")
	}

	cfgInfo, err := store.ReadInfo(c.envName)
	if err != nil {
		return errors.Annotate(err, "cannot read environment info")
	}

	// Verify that we're not destroying a system
	apiEndpoint := cfgInfo.APIEndpoint()
	if apiEndpoint.ServerUUID != "" && apiEndpoint.EnvironUUID == apiEndpoint.ServerUUID {
		return errors.Errorf("%q is a system; use 'juju system destroy' to destroy it", c.envName)
	}

	if !c.assumeYes {
		fmt.Fprintf(ctx.Stdout, destroyEnvMsg, c.envName)

		if err := jujucmd.UserConfirmYes(ctx); err != nil {
			return errors.Annotate(err, "environment destruction")
		}
	}

	// Attempt to connect to the API.  If we can't, fail the destroy.
	api, err := c.getAPI()
	if err != nil {
		return errors.Annotate(err, "cannot connect to API")
	}
	defer api.Close()

	// Attempt to destroy the environment.
	err = api.DestroyEnvironment()
	if err != nil {
		return c.handleError(errors.Annotate(err, "cannot destroy environment"))
	}

	return environs.DestroyInfo(c.envName, store)
}

func (c *DestroyCommand) handleError(err error) error {
	if err == nil {
		return nil
	}
	if params.IsCodeOperationBlocked(err) {
		return block.ProcessBlockedError(err, block.BlockDestroy)
	}
	logger.Errorf(`failed to destroy environment %q`, c.envName)
	return err
}
