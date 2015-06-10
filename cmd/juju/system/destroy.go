// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package system

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/api/environmentmanager"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/juju"
)

// DestroyCommand destroys the specified system.
type DestroyCommand struct {
	envcmd.SysCommandBase
	systemName string
	assumeYes  bool

	// The following fields are for mocking out
	// api behavior for testing.
	api       destroyEnvironmentAPI
	clientapi destroyEnvironmentClientAPI
	apierr    error
}

var destroyDoc = `Destroys the specified system`
var destroyEnvMsg = `
WARNING! This command will destroy the %q system.
This includes all machines, services, data and other resources.

Continue [y/N]? `[1:]

// environmentGetterAPI defines the method on the API endpoint that
// the destroy command calls to obtain bootstrap information for
// the system being destroyed.
type environmentGetterAPI interface {
	EnvironmentGet() (map[string]interface{}, error)
}

// destroyEnvironmentAPI defines the methods on the environmentmanager
// API that the destroy command calls.
type destroyEnvironmentAPI interface {
	environmentGetterAPI
	Close() error
	DestroyEnvironment(string) error
}

// destroyEnvironmentClientAPI defines the methods on the client
// API that the destroy command calls.
type destroyEnvironmentClientAPI interface {
	environmentGetterAPI
	Close() error
	DestroyEnvironment() error
}

// Info implements Command.Info.
func (c *DestroyCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "destroy",
		Args:    "<system name>",
		Purpose: "terminate all machines and other associated resources for a system environment",
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
		return errors.New("no system specified")
	case 1:
		c.systemName = args[0]
		return nil
	default:
		return cmd.CheckEmpty(args[1:])
	}
}

func (c *DestroyCommand) getAPI() (destroyEnvironmentAPI, error) {
	if c.api != nil {
		return c.api, c.apierr
	}
	root, err := juju.NewAPIFromName(c.systemName)
	if err != nil {
		return nil, err
	}

	return environmentmanager.NewClient(root), nil
}

func (c *DestroyCommand) getClientAPI() (destroyEnvironmentClientAPI, error) {
	if c.clientapi != nil {
		return c.clientapi, nil
	}
	return juju.NewAPIClientFromName(c.systemName)
}

// Run implements Command.Run
func (c *DestroyCommand) Run(ctx *cmd.Context) error {
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
		fmt.Fprintf(ctx.Stdout, destroyEnvMsg, c.systemName)

		scanner := bufio.NewScanner(ctx.Stdin)
		scanner.Scan()
		err := scanner.Err()
		if err != nil && err != io.EOF {
			return errors.Annotate(err, "environment destruction aborted")
		}
		answer := strings.ToLower(scanner.Text())
		if answer != "y" && answer != "yes" {
			return errors.New("environment destruction aborted")
		}
	}

	// Attempt to connect to the API.  If we can't, fail the destroy.  Users will
	// need to use the system force-destroy command if we can't connect.
	api, err := c.getAPI()
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Warningf("system not found, removing config file")
			ctx.Infof("system not found, removing config file")
			return environs.DestroyInfo(c.systemName, store)
		}
		return c.ensureUserFriendlyErrorLog(errors.Annotate(err, "cannot connect to API"))
	}
	defer api.Close()

	// Obtain bootstrap / system environ information
	systemEnviron, err := c.getSystemEnviron(cfgInfo, api)
	if params.IsCodeNotImplemented(err) {
		// Fall back to using the client endpoint to obtain bootstrap
		// information and to destroy the system, sending the info
		// we were already able to collect.
		return c.destroyEnvironmentViaClient(ctx, cfgInfo, nil, store)
	}
	if err != nil {
		return errors.Annotate(err, "cannot obtain bootstrap information")
	}

	// Attempt to destroy the system.
	err = api.DestroyEnvironment(apiEndpoint.EnvironUUID)
	if params.IsCodeNotImplemented(err) {
		// Fall back to using the client endpoint to destroy the system,
		// sending the info we were already able to collect.
		return c.destroyEnvironmentViaClient(ctx, cfgInfo, systemEnviron, store)
	}
	if err != nil {
		return c.ensureUserFriendlyErrorLog(errors.Annotate(err, "cannot destroy system"))
	}

	return environs.Destroy(systemEnviron, store)
}

// getSystemEnviron gets the bootstrap information required to destroy the environment
// by first checking the config store, then querying the API if the information is not
// in the store.
func (c *DestroyCommand) getSystemEnviron(info configstore.EnvironInfo, api environmentGetterAPI) (_ environs.Environ, err error) {
	bootstrapCfg := info.BootstrapConfig()
	if bootstrapCfg == nil {
		bootstrapCfg, err = api.EnvironmentGet()
		if err != nil {
			return nil, err
		}
	}

	cfg, err := config.New(config.NoDefaults, bootstrapCfg)
	if err != nil {
		return nil, err
	}
	return environs.New(cfg)
}

// destroyEnvironmentViaClient attempts to destroy the environment using the client
// endpoint for older juju systems which do not implement environmentmanager.DestroyEnvironment
func (c *DestroyCommand) destroyEnvironmentViaClient(ctx *cmd.Context, info configstore.EnvironInfo, systemEnviron environs.Environ, store configstore.Storage) error {
	api, err := c.getClientAPI()
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Warningf("environment not found, removing config file")
			ctx.Infof("environment not found, removing config file")
			return environs.DestroyInfo(c.systemName, store)
		}
		return c.ensureUserFriendlyErrorLog(errors.Annotate(err, "cannot connect to API"))
	}
	defer api.Close()

	if systemEnviron == nil {
		systemEnviron, err = c.getSystemEnviron(info, api)
		if err != nil {
			return errors.Annotate(err, "cannot obtain bootstrap information")
		}
	}

	err = api.DestroyEnvironment()
	if err != nil {
		return c.ensureUserFriendlyErrorLog(errors.Annotate(err, "cannot destroy system"))
	}

	return environs.Destroy(systemEnviron, store)
}

// ensureUserFriendlyErrorLog ensures that error will be logged and displayed
// in a user-friendly manner with readable and digestable error message.
func (c *DestroyCommand) ensureUserFriendlyErrorLog(err error) error {
	if err == nil {
		return nil
	}
	if params.IsCodeOperationBlocked(err) {
		return block.ProcessBlockedError(err, block.BlockDestroy)
	}
	logger.Errorf(stdFailureMsg, c.systemName)
	return err
}

var stdFailureMsg = `failed to destroy system %q

If the system is unusable, then you may run

    juju system force-destroy

to forcefully destroy the system. Upon doing so, review
your environment provider console for any resources that need
to be cleaned up.
`
