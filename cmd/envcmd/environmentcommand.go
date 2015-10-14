// Copyright 2013-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package envcmd

import (
	"io"
	"os"
	"strconv"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/api"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/juju"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/version"
)

var logger = loggo.GetLogger("juju.cmd.envcmd")

// ErrNoEnvironmentSpecified is returned by commands that operate on
// an environment if there is no current environment, no environment
// has been explicitly specified, and there is no default environment.
var ErrNoEnvironmentSpecified = errors.New("no environment specified")

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
	if currentEnv, err := ReadCurrentEnvironment(); err != nil {
		return "", errors.Trace(err)
	} else if currentEnv != "" {
		return currentEnv, nil
	}
	if currentSystem, err := ReadCurrentSystem(); err != nil {
		return "", errors.Trace(err)
	} else if currentSystem != "" {
		return "", errors.Errorf("not operating on an environment, using system %q", currentSystem)
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

	// compatVersion defines the minimum CLI version
	// that this command should be compatible with.
	compatVerson *int

	envGetterClient EnvironmentGetter
	envGetterErr    error
}

func (c *EnvCommandBase) SetEnvName(envName string) {
	c.envName = envName
}

func (c *EnvCommandBase) NewAPIClient() (*api.Client, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return root.Client(), nil
}

// NewEnvironmentGetter returns a new object which implements the
// EnvironmentGetter interface.
func (c *EnvCommandBase) NewEnvironmentGetter() (EnvironmentGetter, error) {
	if c.envGetterErr != nil {
		return nil, c.envGetterErr
	}

	if c.envGetterClient != nil {
		return c.envGetterClient, nil
	}

	return c.NewAPIClient()
}

func (c *EnvCommandBase) NewAPIRoot() (api.Connection, error) {
	// This is work in progress as we remove the EnvName from downstream code.
	// We want to be able to specify the environment in a number of ways, one of
	// which is the connection name on the client machine.
	if c.envName == "" {
		return nil, errors.Trace(ErrNoEnvironmentSpecified)
	}
	return juju.NewAPIFromName(c.envName)
}

// Config returns the configuration for the environment; obtaining bootstrap
// information from the API if necessary.  If callers already have an active
// client API connection, it will be used.  Otherwise, a new API connection will
// be used if necessary.
func (c *EnvCommandBase) Config(store configstore.Storage, client EnvironmentGetter) (*config.Config, error) {
	if c.envName == "" {
		return nil, errors.Trace(ErrNoEnvironmentSpecified)
	}
	cfg, _, err := environs.ConfigForName(c.envName, store)
	if err == nil {
		return cfg, nil
	} else if !environs.IsEmptyConfig(err) {
		return nil, errors.Trace(err)
	}

	if client == nil {
		client, err = c.NewEnvironmentGetter()
		if err != nil {
			return nil, errors.Trace(err)
		}
		defer client.Close()
	}

	bootstrapCfg, err := client.EnvironmentGet()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return config.New(config.NoDefaults, bootstrapCfg)
}

// ConnectionCredentials returns the credentials used to connect to the API for
// the specified environment.
func (c *EnvCommandBase) ConnectionCredentials() (configstore.APICredentials, error) {
	// TODO: the user may soon be specified through the command line
	// or through an environment setting, so return these when they are ready.
	var emptyCreds configstore.APICredentials
	if c.envName == "" {
		return emptyCreds, errors.Trace(ErrNoEnvironmentSpecified)
	}
	info, err := ConnectionInfoForName(c.envName)
	if err != nil {
		return emptyCreds, errors.Trace(err)
	}
	return info.APICredentials(), nil
}

// ConnectionEndpoint returns the end point information used to
// connect to the API for the specified environment.
func (c *EnvCommandBase) ConnectionEndpoint(refresh bool) (configstore.APIEndpoint, error) {
	// TODO: the endpoint information may soon be specified through the command line
	// or through an environment setting, so return these when they are ready.
	// NOTE: refresh when specified through command line should error.
	var emptyEndpoint configstore.APIEndpoint
	if c.envName == "" {
		return emptyEndpoint, errors.Trace(ErrNoEnvironmentSpecified)
	}
	info, err := ConnectionInfoForName(c.envName)
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

	info, err = ConnectionInfoForName(c.envName)
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

// ConnectionWriter returns an instance that is able to be used
// to record information about the connection.  When the connection
// is determined through either command line parameters or environment
// variables, an error is returned.
func (c *EnvCommandBase) ConnectionWriter() (ConnectionWriter, error) {
	// TODO: when accessing with just command line params or environment
	// variables, this should error.
	if c.envName == "" {
		return nil, errors.Trace(ErrNoEnvironmentSpecified)
	}
	return ConnectionInfoForName(c.envName)
}

// CompatVersion returns the minimum CLI version
// that this command should be compatible with.
func (c *EnvCommandBase) CompatVersion() int {
	if c.compatVerson != nil {
		return *c.compatVerson
	}
	compatVerson := 1
	val := os.Getenv(osenv.JujuCLIVersion)
	if val != "" {
		vers, err := strconv.Atoi(val)
		if err != nil {
			logger.Warningf("invalid %s value: %v", osenv.JujuCLIVersion, val)
		} else {
			compatVerson = vers
		}
	}
	c.compatVerson = &compatVerson
	return *c.compatVerson
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

func (w *environCommandWrapper) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&w.envName, "e", "", "juju environment to operate in")
	f.StringVar(&w.envName, "environment", "", "")
	w.EnvironCommand.SetFlags(f)
}

func (w *environCommandWrapper) Init(args []string) error {
	if w.envName == "" {
		// Look for the default.
		defaultEnv, err := GetDefaultEnvironment()
		if err != nil {
			return err
		}
		w.envName = defaultEnv
	}
	w.SetEnvName(w.envName)
	return w.EnvironCommand.Init(args)
}

type bootstrapContext struct {
	*cmd.Context
	verifyCredentials bool
}

// ShouldVerifyCredentials implements BootstrapContext.ShouldVerifyCredentials
func (ctx *bootstrapContext) ShouldVerifyCredentials() bool {
	return ctx.verifyCredentials
}

// BootstrapContext returns a new BootstrapContext constructed from a command Context.
func BootstrapContext(cmdContext *cmd.Context) environs.BootstrapContext {
	return &bootstrapContext{
		Context:           cmdContext,
		verifyCredentials: true,
	}
}

// BootstrapContextNoVerify returns a new BootstrapContext constructed from a command Context
// where the validation of credentials is false.
func BootstrapContextNoVerify(cmdContext *cmd.Context) environs.BootstrapContext {
	return &bootstrapContext{
		Context:           cmdContext,
		verifyCredentials: false,
	}
}

type EnvironmentGetter interface {
	EnvironmentGet() (map[string]interface{}, error)
	Close() error
}

// GetEnvironmentVersion retrieves the environment's agent-version
// value from an API client.
func GetEnvironmentVersion(client EnvironmentGetter) (version.Number, error) {
	noVersion := version.Number{}
	attrs, err := client.EnvironmentGet()
	if err != nil {
		return noVersion, errors.Annotate(err, "unable to retrieve environment config")
	}
	vi, found := attrs["agent-version"]
	if !found {
		return noVersion, errors.New("version not found in environment config")
	}
	vs, ok := vi.(string)
	if !ok {
		return noVersion, errors.New("invalid environment version type in config")
	}
	v, err := version.Parse(vs)
	if err != nil {
		return noVersion, errors.Annotate(err, "unable to parse environment version")
	}
	return v, nil
}
