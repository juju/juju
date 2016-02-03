// Copyright 2013-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelcmd

import (
	"io"
	"os"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"launchpad.net/gnuflag"

	"github.com/juju/juju/api"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/version"
)

var logger = loggo.GetLogger("juju.cmd.envcmd")

// ErrNoModelSpecified is returned by commands that operate on
// an environment if there is no current model, no model
// has been explicitly specified, and there is no default model.
var ErrNoModelSpecified = errors.New("no model specified")

// GetDefaultModel returns the name of the Juju default model.
// There is simple ordering for the default model.  Firstly check the
// JUJU_MODEL environment variable.  If that is set, it gets used.  If it isn't
// set, look in the $JUJU_HOME/current-environment file.  If neither are
// available, an empty string is returned; not having a default model
// specified is not an error.
func GetDefaultModel() (string, error) {
	if defaultEnv := os.Getenv(osenv.JujuModelEnvKey); defaultEnv != "" {
		return defaultEnv, nil
	}
	if currentModel, err := ReadCurrentModel(); err != nil {
		return "", errors.Trace(err)
	} else if currentModel != "" {
		return currentModel, nil
	}
	if currentController, err := ReadCurrentController(); err != nil {
		return "", errors.Trace(err)
	} else if currentController != "" {
		return "", errors.Errorf("not operating on an model, using controller %q", currentController)
	}
	return "", nil
}

// ModelCommand extends cmd.Command with a SetModelName method.
type ModelCommand interface {
	CommandBase

	// SetModelName is called prior to the wrapped command's Init method
	// with the active model name. The model name is guaranteed
	// to be non-empty at entry of Init.
	SetModelName(modelName string)

	// ModelName returns the name of the model.
	ModelName() string

	// SetAPIOpener allows the replacement of the default API opener,
	// which ends up calling NewAPIRoot
	SetAPIOpener(opener APIOpener)
}

// ModelCommandBase is a convenience type for embedding in commands
// that wish to implement ModelCommand.
type ModelCommandBase struct {
	JujuCommandBase

	// ModelName will very soon be package visible only as we want to be able
	// to specify an model in multiple ways, and not always referencing
	// a file on disk based on the ModelName.
	modelName string

	// opener is the strategy used to open the API connection.
	opener APIOpener

	envGetterClient ModelGetter
	envGetterErr    error
}

// SetModelName implements the ModelCommand interface.
func (c *ModelCommandBase) SetModelName(modelName string) {
	c.modelName = modelName
}

// ModelName implements the ModelCommand interface.
func (c *ModelCommandBase) ModelName() string {
	return c.modelName
}

// SetAPIOpener specifies the strategy used by the command to open
// the API connection.
func (c *ModelCommandBase) SetAPIOpener(opener APIOpener) {
	c.opener = opener
}

func (c *ModelCommandBase) NewAPIClient() (*api.Client, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return root.Client(), nil
}

// NewModelGetter returns a new object which implements the
// ModelGetter interface.
func (c *ModelCommandBase) NewModelGetter() (ModelGetter, error) {
	if c.envGetterErr != nil {
		return nil, c.envGetterErr
	}

	if c.envGetterClient != nil {
		return c.envGetterClient, nil
	}

	return c.NewAPIClient()
}

// NewAPIRoot returns a new connection to the API server for the environment.
func (c *ModelCommandBase) NewAPIRoot() (api.Connection, error) {
	// This is work in progress as we remove the ModelName from downstream code.
	// We want to be able to specify the environment in a number of ways, one of
	// which is the connection name on the client machine.
	if c.modelName == "" {
		return nil, errors.Trace(ErrNoModelSpecified)
	}
	opener := c.opener
	if opener == nil {
		opener = NewPassthroughOpener(c.JujuCommandBase.NewAPIRoot)
	}
	return opener.Open(c.modelName)
}

// ConnectionCredentials returns the credentials used to connect to the API for
// the specified environment.
func (c *ModelCommandBase) ConnectionCredentials() (configstore.APICredentials, error) {
	// TODO: the user may soon be specified through the command line
	// or through an environment setting, so return these when they are ready.
	var emptyCreds configstore.APICredentials
	if c.modelName == "" {
		return emptyCreds, errors.Trace(ErrNoModelSpecified)
	}
	info, err := ConnectionInfoForName(c.modelName)
	if err != nil {
		return emptyCreds, errors.Trace(err)
	}
	return info.APICredentials(), nil
}

// ConnectionEndpoint returns the end point information used to
// connect to the API for the specified environment.
func (c *ModelCommandBase) ConnectionEndpoint(refresh bool) (configstore.APIEndpoint, error) {
	// TODO: the endpoint information may soon be specified through the command line
	// or through an environment setting, so return these when they are ready.
	// NOTE: refresh when specified through command line should error.
	var emptyEndpoint configstore.APIEndpoint
	if c.modelName == "" {
		return emptyEndpoint, errors.Trace(ErrNoModelSpecified)
	}
	info, err := ConnectionInfoForName(c.modelName)
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

	info, err = ConnectionInfoForName(c.modelName)
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

var endpointRefresher = func(c *ModelCommandBase) (io.Closer, error) {
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
// environment (modelName) and returns it.
func ConnectionInfoForName(modelName string) (configstore.EnvironInfo, error) {
	store, err := getConfigStore()
	if err != nil {
		return nil, errors.Trace(err)
	}
	info, err := store.ReadInfo(modelName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return info, nil
}

// ConnectionWriter returns an instance that is able to be used
// to record information about the connection.  When the connection
// is determined through either command line parameters or environment
// variables, an error is returned.
func (c *ModelCommandBase) ConnectionWriter() (ConnectionWriter, error) {
	// TODO: when accessing with just command line params or environment
	// variables, this should error.
	if c.modelName == "" {
		return nil, errors.Trace(ErrNoModelSpecified)
	}
	return ConnectionInfoForName(c.modelName)
}

// ConnectionName returns the name of the connection if there is one.
// It is possible that the name of the connection is empty if the
// connection information is supplied through command line arguments
// or environment variables.
func (c *ModelCommandBase) ConnectionName() string {
	return c.modelName
}

// WrapControllerOption sets various parameters of the
// ModelCommand wrapper.
type WrapEnvOption func(*modelCommandWrapper)

// ModelSkipFlags instructs the wrapper to skip --m and
// --model flag definition.
func ModelSkipFlags(w *modelCommandWrapper) {
	w.skipFlags = true
}

// ModelSkipDefault instructs the wrapper not to
// use the default model.
func ModelSkipDefault(w *modelCommandWrapper) {
	w.useDefaultModel = false
}

// EnvAPIOpener instructs the underlying environment command to use a
// different Opener strategy.
func EnvAPIOpener(opener APIOpener) WrapEnvOption {
	return func(w *modelCommandWrapper) {
		w.ModelCommand.SetAPIOpener(opener)
	}
}

// Wrap wraps the specified ModelCommand, returning a Command
// that proxies to each of the ModelCommand methods.
// Any provided options are applied to the wrapped command
// before it is returned.
func Wrap(c ModelCommand, options ...WrapEnvOption) cmd.Command {
	wrapper := &modelCommandWrapper{
		ModelCommand:    c,
		skipFlags:       false,
		useDefaultModel: true,
		allowEmptyEnv:   false,
	}
	for _, option := range options {
		option(wrapper)
	}
	return WrapBase(wrapper)
}

type modelCommandWrapper struct {
	ModelCommand

	skipFlags       bool
	useDefaultModel bool
	allowEmptyEnv   bool
	modelName       string
}

func (w *modelCommandWrapper) SetFlags(f *gnuflag.FlagSet) {
	if !w.skipFlags {
		f.StringVar(&w.modelName, "m", "", "juju model to operate in")
		f.StringVar(&w.modelName, "model", "", "")
	}
	w.ModelCommand.SetFlags(f)
}

func (w *modelCommandWrapper) Init(args []string) error {
	if !w.skipFlags {
		if w.modelName == "" && w.useDefaultModel {
			// Look for the default.
			defaultModel, err := GetDefaultModel()
			if err != nil {
				return err
			}
			w.modelName = defaultModel
		}
		if w.modelName == "" && !w.useDefaultModel {
			if w.allowEmptyEnv {
				return w.ModelCommand.Init(args)
			} else {
				return errors.Trace(ErrNoModelSpecified)
			}
		}
	}
	w.SetModelName(w.modelName)
	return w.ModelCommand.Init(args)
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

type ModelGetter interface {
	ModelGet() (map[string]interface{}, error)
	Close() error
}

// GetModelVersion retrieves the models's agent-version
// value from an API client.
func GetModelVersion(client ModelGetter) (version.Number, error) {
	noVersion := version.Number{}
	attrs, err := client.ModelGet()
	if err != nil {
		return noVersion, errors.Annotate(err, "unable to retrieve model config")
	}
	vi, found := attrs["agent-version"]
	if !found {
		return noVersion, errors.New("version not found in model config")
	}
	vs, ok := vi.(string)
	if !ok {
		return noVersion, errors.New("invalid model version type in config")
	}
	v, err := version.Parse(vs)
	if err != nil {
		return noVersion, errors.Annotate(err, "unable to parse model version")
	}
	return v, nil
}
