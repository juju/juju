// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package envcmd

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/environmentmanager"
	"github.com/juju/juju/api/usermanager"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/juju"
)

// ErrNoSystemSpecified is returned by commands that operate on
// a system if there is no current system, no system has been
// explicitly specified, and there is no default system.
var ErrNoSystemSpecified = errors.New("no system specified")

// SystemCommand extends cmd.Command with a SetEnvName method.
type SystemCommand interface {
	cmd.Command

	// TODO (cherylj): Once we have a way of recording the current system
	// we need to stop using the environment.

	// SetEnvName is called prior to the wrapped command's Init method
	// with the active environment name. The environment name is guaranteed
	// to be non-empty at entry of Init.
	SetEnvName(envName string)
}

// SysCommandBase is a convenience type for embedding in commands
// that wish to implement SystemCommand.
type SysCommandBase struct {
	cmd.CommandBase
	envName string
}

// SetEnvName records the current environment name in the SysCommandBase
func (c *SysCommandBase) SetEnvName(envName string) {
	c.envName = envName
}

// NewEnvironmentManagerAPIClient returns an API client for the EnvironmentManager on the
// current system using the current credentials.
func (c *SysCommandBase) NewEnvironmentManagerAPIClient() (*environmentmanager.Client, error) {
	root, err := c.newAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return environmentmanager.NewClient(root), nil
}

// NewUserManagerAPIClient returns an API client for the UserManager on the
// current system using the current credentials.
func (c *SysCommandBase) NewUserManagerAPIClient() (*usermanager.Client, error) {
	root, err := c.newAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return usermanager.NewClient(root), nil
}

// newAPIRoot returns a restricted API for the current system using the current
// credentials.  Only the UserManager and EnvironmentManager may be accessed
// through this API connection.
func (c *SysCommandBase) newAPIRoot() (*api.State, error) {
	if c.envName == "" {
		return nil, errors.Trace(ErrNoSystemSpecified)
	}
	return juju.NewAPIFromName(c.envName)
}

// ConnectionCredentials returns the credentials used to connect to the API for
// the specified environment.
func (c *SysCommandBase) ConnectionCredentials() (configstore.APICredentials, error) {
	// TODO: the user may soon be specified through the command line
	// or through an environment setting, so return these when they are ready.
	var emptyCreds configstore.APICredentials
	if c.envName == "" {
		return emptyCreds, errors.Trace(ErrNoSystemSpecified)
	}
	info, err := connectionInfoForName(c.envName)
	if err != nil {
		return emptyCreds, errors.Trace(err)
	}
	return info.APICredentials(), nil
}

// connectionInfoForName reads the environment information for the named
// environment (envName) and returns it.
func connectionInfoForName(envName string) (configstore.EnvironInfo, error) {
	store, err := getConfigStore()
	if err != nil {
		return nil, errors.Trace(err)
	}
	info, err := store.ReadInfo(envName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return info, nil
}

// Wrap wraps the specified SystemCommand, returning a Command
// that proxies to each of the SystemCommand methods.
func WrapSystem(c SystemCommand) cmd.Command {
	return &sysCommandWrapper{SystemCommand: c}
}

type sysCommandWrapper struct {
	SystemCommand
	envName string
}

// SetFlags implements Command.SetFlags, then calls the wrapped command's SetFlags.
func (w *sysCommandWrapper) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&w.envName, "s", "", "juju system to operate in")
	f.StringVar(&w.envName, "system", "", "")
	w.SystemCommand.SetFlags(f)
}

// Init implements Command.Init, then calls the wrapped command's Init.
func (w *sysCommandWrapper) Init(args []string) error {
	if w.envName == "" {
		// Look for the default.
		defaultEnv, err := GetDefaultEnvironment()
		if err != nil {
			return err
		}
		w.envName = defaultEnv
	}
	w.SetEnvName(w.envName)
	return w.SystemCommand.Init(args)
}
