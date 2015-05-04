// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package syscmd

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/environmentmanager"
	"github.com/juju/juju/api/usermanager"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/juju"
	"github.com/juju/juju/juju/osenv"
)

var logger = loggo.GetLogger("juju.cmd.syscmd")

const CurrentEnvironmentFilename = "current-environment"

// ErrNoSystemSpecified is returned by commands that operate on
// a system if there is no current system, no system has been
// explicitly specified, and there is no default system.
var ErrNoSystemSpecified = errors.New("no system specified")

func getCurrentEnvironmentFilePath() string {
	return filepath.Join(osenv.JujuHome(), CurrentEnvironmentFilename)
}

// Read the file $JUJU_HOME/current-environment and return the value stored
// there.  If the file doesn't exist, or there is a problem reading the file,
// an empty string is returned.
func ReadCurrentEnvironment() string {
	current, err := ioutil.ReadFile(getCurrentEnvironmentFilePath())
	// The file not being there, or not readable isn't really an error for us
	// here.  We treat it as "can't tell, so you get the default".
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(current))
}

// GetDefaultEnvironment returns the name of the Juju default environment.
// There is simple ordering for the default environment.  Firstly check the
// JUJU_ENV environment variable.  If that is set, it gets used.  If it isn't
// set, look in the $JUJU_HOME/current-environment file.  If neither are
// available, read environments.yaml and use the default environment therein.
// If no default is specified in the environments file, an empty string is returned.
// Not having a default environment specified is not an error.
func GetDefaultEnvironment() (string, error) {
	if defaultEnv := os.Getenv(osenv.JujuEnvEnvKey); defaultEnv != "" {
		return defaultEnv, nil
	}
	if currentEnv := ReadCurrentEnvironment(); currentEnv != "" {
		return currentEnv, nil
	}
	envs, err := environs.ReadEnvirons("")
	if environs.IsNoEnv(err) {
		// That's fine, not an error here.
		return "", nil
	} else if err != nil {
		return "", errors.Trace(err)
	}
	return envs.Default, nil
}

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

// NewEnvMgrAPIClient returns an API client for the EnvironmentManager on the
// current system using the current credentials.
func (c *SysCommandBase) NewEnvMgrAPIClient() (*environmentmanager.Client, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return environmentmanager.NewClient(root), nil
}

// NewUserMgrAPIClient returns an API client for the UserManager on the
// current system using the current credentials.
func (c *SysCommandBase) NewUserMgrAPIClient() (*usermanager.Client, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return usermanager.NewClient(root), nil
}

// NewAPIRoot returns a restricted API for the current system using the current
// credentials.  Only the UserManager and EnvironmentManager may be accessed
// through this API connection.
func (c *SysCommandBase) NewAPIRoot() (*api.State, error) {
	if c.envName == "" {
		return nil, errors.Trace(ErrNoSystemSpecified)
	}
	return juju.NewSystemAPIFromName(c.envName)
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
	info, err := ConnectionInfoForName(c.envName)
	if err != nil {
		return emptyCreds, errors.Trace(err)
	}
	return info.APICredentials(), nil
}

func getConfigStore() (configstore.Storage, error) {
	store, err := configstore.Default()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return store, nil
}

// ConnectionInfoForName reads the environment information for the named
// environment (envName) and returns it.
func ConnectionInfoForName(envName string) (configstore.EnvironInfo, error) {
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
func Wrap(c SystemCommand) cmd.Command {
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
