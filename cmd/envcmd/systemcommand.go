// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package envcmd

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/environmentmanager"
	"github.com/juju/juju/api/systemmanager"
	"github.com/juju/juju/api/usermanager"
	"github.com/juju/juju/environs/configstore"
)

// ErrNoSystemSpecified is returned by commands that operate on
// a system if there is no current system, no system has been
// explicitly specified, and there is no default system.
var ErrNoSystemSpecified = errors.New("no system specified")

// SystemCommand is intended to be a base for all commands
// that need to operate on systems as opposed to environments.
type SystemCommand interface {
	CommandBase

	// SetSystemName is called prior to the wrapped command's Init method with
	// the active system name. The system name is guaranteed to be non-empty
	// at entry of Init. It records the current environment name in the
	// SysCommandBase.
	SetSystemName(systemName string)

	// SystemName returns the name of the system or environment used to
	// determine that API end point.
	SystemName() string
}

// SysCommandBase is a convenience type for embedding in commands
// that wish to implement SystemCommand.
type SysCommandBase struct {
	JujuCommandBase

	systemName string
}

// SetSystemName implements the SystemCommand interface.
func (c *SysCommandBase) SetSystemName(systemName string) {
	c.systemName = systemName
}

// SystemName implements the SystemCommand interface.
func (c *SysCommandBase) SystemName() string {
	return c.systemName
}

// NewEnvironmentManagerAPIClient returns an API client for the
// EnvironmentManager on the current system using the current credentials.
func (c *SysCommandBase) NewEnvironmentManagerAPIClient() (*environmentmanager.Client, error) {
	root, err := c.newAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return environmentmanager.NewClient(root), nil
}

// NewSystemManagerAPIClient returns an API client for the SystemManager on
// the current system using the current credentials.
func (c *SysCommandBase) NewSystemManagerAPIClient() (*systemmanager.Client, error) {
	root, err := c.newAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return systemmanager.NewClient(root), nil
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
func (c *SysCommandBase) newAPIRoot() (api.Connection, error) {
	if c.systemName == "" {
		return nil, errors.Trace(ErrNoSystemSpecified)
	}
	return c.NewAPIRoot(c.systemName)
}

// ConnectionCredentials returns the credentials used to connect to the API for
// the specified system.
func (c *SysCommandBase) ConnectionCredentials() (configstore.APICredentials, error) {
	// TODO: the user may soon be specified through the command line
	// or through an environment setting, so return these when they are ready.
	var emptyCreds configstore.APICredentials
	info, err := c.ConnectionInfo()
	if err != nil {
		return emptyCreds, errors.Trace(err)
	}
	return info.APICredentials(), nil
}

// ConnectionEndpoint returns the endpoint details used to connect to the API for
// the specified system.
func (c *SysCommandBase) ConnectionEndpoint() (configstore.APIEndpoint, error) {
	// TODO: the user may soon be specified through the command line
	// or through an environment setting, so return these when they are ready.
	var empty configstore.APIEndpoint
	info, err := c.ConnectionInfo()
	if err != nil {
		return empty, errors.Trace(err)
	}
	return info.APIEndpoint(), nil
}

// ConnectionInfo returns the environ info from the cached config store.
func (c *SysCommandBase) ConnectionInfo() (configstore.EnvironInfo, error) {
	// TODO: the user may soon be specified through the command line
	// or through an environment setting, so return these when they are ready.
	if c.systemName == "" {
		return nil, errors.Trace(ErrNoSystemSpecified)
	}
	info, err := ConnectionInfoForName(c.systemName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return info, nil
}

// WrapSystemOption sets various parameters of the
// SystemCommand wrapper.
type WrapSystemOption func(*sysCommandWrapper)

// SystemSkipFlags instructs the wrapper to skip --s
// and --system flag definition.
func SystemSkipFlags(w *sysCommandWrapper) {
	w.setFlags = false
}

// SystemSkipDefault instructs the wrapper not to
// use the default system name.
func SystemSkipDefault(w *sysCommandWrapper) {
	w.useDefaultSystemName = false
}

// WrapSystem wraps the specified SystemCommand, returning a Command
// that proxies to each of the SystemCommand methods.
func WrapSystem(c SystemCommand, options ...WrapSystemOption) cmd.Command {
	wrapper := &sysCommandWrapper{
		SystemCommand:        c,
		setFlags:             true,
		useDefaultSystemName: true,
	}
	for _, option := range options {
		option(wrapper)
	}
	return WrapBase(wrapper)
}

type sysCommandWrapper struct {
	SystemCommand
	setFlags             bool
	useDefaultSystemName bool
	systemName           string
}

// SetFlags implements Command.SetFlags, then calls the wrapped command's SetFlags.
func (w *sysCommandWrapper) SetFlags(f *gnuflag.FlagSet) {
	if w.setFlags {
		f.StringVar(&w.systemName, "s", "", "juju system to operate in")
		f.StringVar(&w.systemName, "system", "", "")
	}
	w.SystemCommand.SetFlags(f)
}

func (w *sysCommandWrapper) getDefaultSystemName() (string, error) {
	if currentSystem, err := ReadCurrentSystem(); err != nil {
		return "", errors.Trace(err)
	} else if currentSystem != "" {
		return currentSystem, nil
	}
	if currentEnv, err := ReadCurrentEnvironment(); err != nil {
		return "", errors.Trace(err)
	} else if currentEnv != "" {
		return currentEnv, nil
	}
	return "", errors.Trace(ErrNoSystemSpecified)
}

// Init implements Command.Init, then calls the wrapped command's Init.
func (w *sysCommandWrapper) Init(args []string) error {
	if w.setFlags {
		if w.systemName == "" && w.useDefaultSystemName {
			name, err := w.getDefaultSystemName()
			if err != nil {
				return errors.Trace(err)
			}
			w.systemName = name
		}
		if w.systemName == "" && !w.useDefaultSystemName {
			return ErrNoSystemSpecified
		}
	}
	w.SetSystemName(w.systemName)
	return w.SystemCommand.Init(args)
}
