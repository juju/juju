// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package envcmd

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/juju"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/state/api"
)

var logger = loggo.GetLogger("juju.cmd.envcmd")

const CurrentEnvironmentFilename = "current-environment"

// ErrNoEnvironmentSpecified is returned by commands that operate on
// an environment if there is no current environment, no environment
// has been explicitly specified, and there is no default environment.
var ErrNoEnvironmentSpecified = fmt.Errorf("no environment specified")

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

// Write the envName to the file $JUJU_HOME/current-environment file.
func WriteCurrentEnvironment(envName string) error {
	path := getCurrentEnvironmentFilePath()
	err := ioutil.WriteFile(path, []byte(envName+"\n"), 0644)
	if err != nil {
		return fmt.Errorf("unable to write to the environment file: %q, %s", path, err)
	}
	return nil
}

// There is simple ordering for the default environment.  Firstly check the
// JUJU_ENV environment variable.  If that is set, it gets used.  If it isn't
// set, look in the $JUJU_HOME/current-environment file.  If neither are
// available, read environments.yaml and use the default environment therein.
func getDefaultEnvironment() (string, error) {
	if defaultEnv := os.Getenv(osenv.JujuEnvEnvKey); defaultEnv != "" {
		return defaultEnv, nil
	}
	if currentEnv := ReadCurrentEnvironment(); currentEnv != "" {
		return currentEnv, nil
	}
	envs, err := environs.ReadEnvirons("")
	if err != nil {
		return "", err
	}
	if envs.Default == "" {
		return "", ErrNoEnvironmentSpecified
	}
	return envs.Default, nil
}

// EnvironCommand extends cmd.Command with a SetEnvName method.
type EnvironCommand interface {
	cmd.Command

	// SetEnvName is called prior to the wrapped command's Init method
	// with the active environment name. The environment name is guaranteed
	// to be non-empty at entry of Init.
	SetEnvName(envName string)
}

// EnvCommandBase is a convenience type for embedding in commands
// that wish to implement EnvironCommand.
type EnvCommandBase struct {
	cmd.CommandBase
	// EnvName will very soon be package visible only as we want to be able
	// to specify an environment in multiple ways, and not always referencing
	// a file on disk based on the EnvName or the environemnts.yaml file.
	envName string
}

func (c *EnvCommandBase) SetEnvName(envName string) {
	c.envName = envName
}

func (c *EnvCommandBase) NewAPIClient() (*api.Client, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, err
	}
	return root.Client(), nil
}

func (c *EnvCommandBase) NewAPIRoot() (*api.State, error) {
	// This is work in progress as we remove the EnvName from downstream code.
	// We want to be able to specify the environment in a number of ways, one of
	// which is the connection name on the client machine.
	return juju.NewAPIFromName(c.envName)
}

func (c *EnvCommandBase) Config(store configstore.Storage) (*config.Config, error) {
	cfg, _, err := environs.ConfigForName(c.envName, store)
	return cfg, err
}

// ConnectionCredentials returns the credentials used to connect to the API for
// the specified environment.
func (c *EnvCommandBase) ConnectionCredentials() (configstore.APICredentials, error) {
	// TODO: the user may soon be specified through the command line
	// or through an environment setting, so return these when they are ready.
	var emptyCreds configstore.APICredentials
	info, err := connectionInfoForName(c.envName)
	if err != nil {
		return emptyCreds, errors.Trace(err)
	}
	return info.APICredentials(), nil
}

// ConnectionEndpoint returns the end point information used to connect to the API for
// the specified environment.
func (c *EnvCommandBase) ConnectionEndpoint(refresh bool) (configstore.APIEndpoint, error) {
	// TODO: the endpoint information may soon be specified through the command line
	// or through an environment setting, so return these when they are ready.
	// NOTE: refresh when specified through command line should error.
	var emptyEndpoint configstore.APIEndpoint
	info, err := connectionInfoForName(c.envName)
	if err != nil {
		return emptyEndpoint, errors.Trace(err)
	}
	endpoint := info.APIEndpoint()
	if !refresh && len(endpoint.Addresses) > 0 {
		logger.Debugf("found cached addresses, not connecting to API server")
		return endpoint, nil
	}

	// We need to connect to refresh our endpoint settings
	// The side effect of connecting is that we update the store with new API information
	refresher, err := endpointRefresher(c)
	if err != nil {
		return emptyEndpoint, err
	}
	refresher.Close()

	info, err = connectionInfoForName(c.envName)
	if err != nil {
		return emptyEndpoint, err
	}
	return info.APIEndpoint(), nil
}

// ConnectionWriter defines the methods needed to write information about
// a given connection.  This is a subset of the methods in the interface
// defined in configstore.EnvironInfo.
type ConnectionWriter interface {
	Write() error
	SetAPICredentials(configstore.APICredentials)
	SetAPIEndpoint(configstore.APIEndpoint)
	SetBootstrapConfig(map[string]interface{})
	Location() string
}

var endpointRefresher = func(c *EnvCommandBase) (io.Closer, error) {
	return c.NewAPIRoot()
}

var getConfigStore = func() (configstore.Storage, error) {
	store, err := configstore.Default()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return store, nil
}

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

// ConnectionWriter returns an instance that is able to be used
// to record information about the connection.  When the connection
// is determined through either command line parameters or environment
// variables, an error is returned.
func (c *EnvCommandBase) ConnectionWriter() (ConnectionWriter, error) {
	// TODO: when accessing with just command line params or environment
	// variables, this should error.
	return connectionInfoForName(c.envName)
}

// ConnectionName returns the name of the connection if there is one.
// It is possible that the name of the connection is empty if the
// connection information is supplied through command line arguments
// or environment variables.
func (c *EnvCommandBase) ConnectionName() string {
	return c.envName
}

// Wrap wraps the specified EnvironCommand, returning a Command
// that proxies to each of the EnvironCommand methods.
func Wrap(c EnvironCommand) cmd.Command {
	return &environCommandWrapper{EnvironCommand: c}
}

type environCommandWrapper struct {
	EnvironCommand
	envName string
}

// ensureEnvName ensures that w.envName is non-empty, or sets it to
// the default environment name. If there is no default environment name,
// then ensureEnvName returns ErrNoEnvironmentSpecified.
func (w *environCommandWrapper) ensureEnvName() error {
	if w.envName != "" {
		return nil
	}
	defaultEnv, err := getDefaultEnvironment()
	if err != nil {
		return err
	}
	w.envName = defaultEnv
	return nil
}

func (w *environCommandWrapper) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&w.envName, "e", "", "juju environment to operate in")
	f.StringVar(&w.envName, "environment", "", "")
	w.EnvironCommand.SetFlags(f)
}

func (w *environCommandWrapper) Init(args []string) error {
	if err := w.ensureEnvName(); err != nil {
		return err
	}
	w.SetEnvName(w.envName)
	return w.EnvironCommand.Init(args)
}
