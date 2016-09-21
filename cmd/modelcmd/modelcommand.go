// Copyright 2013-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelcmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"

	"github.com/juju/juju/api"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/jujuclient"
)

var logger = loggo.GetLogger("juju.cmd.modelcmd")

// ErrNoModelSpecified is returned by commands that operate on
// an environment if there is no current model, no model
// has been explicitly specified, and there is no default model.
var ErrNoModelSpecified = errors.New(`no model in focus

Please use "juju models" to see models available to you.
You can set current model by running "juju switch"
or specify any other model on the command line using the "-m" flag.
`)

// GetCurrentModel returns the name of the current Juju model.
//
// If $JUJU_MODEL is set, use that. Otherwise, get the current
// controller from controllers.yaml, and then identify the current
// model for that controller in models.yaml. If there is no current
// controller, then an empty string is returned. It is not an error
// to have no current model.
//
// If there is a current controller, but no current model for that
// controller, then GetCurrentModel will return the string
// "<controller>:". If there is a current model as well, it will
// return "<controller>:<model>". Only when $JUJU_MODEL is set,
// will the result possibly be unqualified.
func GetCurrentModel(store jujuclient.ClientStore) (string, error) {
	if model := os.Getenv(osenv.JujuModelEnvKey); model != "" {
		return model, nil
	}

	currentController, err := store.CurrentController()
	if errors.IsNotFound(err) {
		return "", nil
	} else if err != nil {
		return "", errors.Trace(err)
	}

	currentModel, err := store.CurrentModel(currentController)
	if errors.IsNotFound(err) {
		return currentController + ":", nil
	} else if err != nil {
		return "", errors.Trace(err)
	}
	return JoinModelName(currentController, currentModel), nil
}

// ModelCommand extends cmd.Command with a SetModelName method.
type ModelCommand interface {
	CommandBase

	// SetClientStore is called prior to the wrapped command's Init method
	// with the default controller store. It may also be called to override the
	// default controller store for testing.
	SetClientStore(jujuclient.ClientStore)

	// ClientStore returns the controller store that the command is
	// associated with.
	ClientStore() jujuclient.ClientStore

	// SetModelName sets the model name for this command. Setting the model
	// name will also set the related controller name. The model name can
	// be qualified with a controller name (controller:model), or
	// unqualified, in which case it will be assumed to be within the
	// current controller.
	//
	// SetModelName is called prior to the wrapped command's Init method
	// with the active model name. The model name is guaranteed
	// to be non-empty at entry of Init.
	SetModelName(modelName string) error

	// ModelName returns the name of the model.
	ModelName() string

	// ControllerName returns the name of the controller that contains
	// the model returned by ModelName().
	ControllerName() string

	// SetAPIOpener allows the replacement of the default API opener,
	// which ends up calling NewAPIRoot
	SetAPIOpener(opener APIOpener)
}

// ModelCommandBase is a convenience type for embedding in commands
// that wish to implement ModelCommand.
type ModelCommandBase struct {
	JujuCommandBase

	// store is the client controller store that contains information
	// about controllers, models, etc.
	store jujuclient.ClientStore

	modelName      string
	controllerName string

	// opener is the strategy used to open the API connection.
	opener APIOpener
}

// SetClientStore implements the ModelCommand interface.
func (c *ModelCommandBase) SetClientStore(store jujuclient.ClientStore) {
	c.store = store
}

// ClientStore implements the ModelCommand interface.
func (c *ModelCommandBase) ClientStore() jujuclient.ClientStore {
	return c.store
}

// SetModelName implements the ModelCommand interface.
func (c *ModelCommandBase) SetModelName(modelName string) error {
	controllerName, modelName := SplitModelName(modelName)
	if controllerName == "" {
		currentController, err := c.store.CurrentController()
		if errors.IsNotFound(err) {
			return errors.Errorf("no current controller, and none specified")
		} else if err != nil {
			return errors.Trace(err)
		}
		controllerName = currentController
	} else {
		var err error
		if _, err = c.store.ControllerByName(controllerName); err != nil {
			return errors.Trace(err)
		}
	}
	c.controllerName = controllerName
	c.modelName = modelName
	return nil
}

// ModelName implements the ModelCommand interface.
func (c *ModelCommandBase) ModelName() string {
	return c.modelName
}

// ControllerName implements the ModelCommand interface.
func (c *ModelCommandBase) ControllerName() string {
	return c.controllerName
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

// NewAPIRoot returns a new connection to the API server for the environment
// directed to the model specified on the command line.
func (c *ModelCommandBase) NewAPIRoot() (api.Connection, error) {
	// This is work in progress as we remove the ModelName from downstream code.
	// We want to be able to specify the environment in a number of ways, one of
	// which is the connection name on the client machine.
	if c.modelName == "" {
		return nil, errors.Trace(ErrNoModelSpecified)
	}
	_, err := c.store.ModelByName(c.controllerName, c.modelName)
	if err != nil {
		if !errors.IsNotFound(err) {
			return nil, errors.Trace(err)
		}
		// The model isn't known locally, so query the models
		// available in the controller, and cache them locally.
		if err := c.RefreshModels(c.store, c.controllerName); err != nil {
			return nil, errors.Annotate(err, "refreshing models")
		}
	}
	return c.newAPIRoot(c.modelName)
}

// NewControllerAPIRoot returns a new connection to the API server for the environment
// directed to the controller specified on the command line.
// This is for the use of model-centered commands that still want
// to talk to controller-only APIs.
func (c *ModelCommandBase) NewControllerAPIRoot() (api.Connection, error) {
	return c.newAPIRoot("")
}

// newAPIRoot is the internal implementation of NewAPIRoot and NewControllerAPIRoot;
// if modelName is empty, it makes a controller-only connection.
func (c *ModelCommandBase) newAPIRoot(modelName string) (api.Connection, error) {
	if c.controllerName == "" {
		controllers, err := c.store.AllControllers()
		if err != nil {
			return nil, errors.Trace(err)
		}
		if len(controllers) == 0 {
			return nil, errors.Trace(ErrNoControllersDefined)
		}
		return nil, errors.Trace(ErrNotLoggedInToController)
	}
	opener := c.opener
	if opener == nil {
		opener = OpenFunc(c.JujuCommandBase.NewAPIRoot)
	}
	return opener.Open(c.store, c.controllerName, modelName)
}

// ConnectionName returns the name of the connection if there is one.
// It is possible that the name of the connection is empty if the
// connection information is supplied through command line arguments
// or environment variables.
func (c *ModelCommandBase) ConnectionName() string {
	return c.modelName
}

// WrapOption specifies an option to the Wrap function.
type WrapOption func(*modelCommandWrapper)

// Options for the Wrap function.
var (
	// WrapSkipModelFlags specifies that the -m and --model flags
	// should not be defined.
	WrapSkipModelFlags WrapOption = wrapSkipModelFlags

	// WrapSkipDefaultModel specifies that no default model should
	// be used.
	WrapSkipDefaultModel WrapOption = wrapSkipDefaultModel
)

func wrapSkipModelFlags(w *modelCommandWrapper) {
	w.skipModelFlags = true
}

func wrapSkipDefaultModel(w *modelCommandWrapper) {
	w.useDefaultModel = false
}

// Wrap wraps the specified ModelCommand, returning a Command
// that proxies to each of the ModelCommand methods.
// Any provided options are applied to the wrapped command
// before it is returned.
func Wrap(c ModelCommand, options ...WrapOption) cmd.Command {
	wrapper := &modelCommandWrapper{
		ModelCommand:    c,
		skipModelFlags:  false,
		useDefaultModel: true,
	}
	for _, option := range options {
		option(wrapper)
	}
	return WrapBase(wrapper)
}

type modelCommandWrapper struct {
	ModelCommand

	skipModelFlags  bool
	useDefaultModel bool
	modelName       string
}

func (w *modelCommandWrapper) Run(ctx *cmd.Context) error {
	return w.ModelCommand.Run(ctx)
}

func (w *modelCommandWrapper) SetFlags(f *gnuflag.FlagSet) {
	if !w.skipModelFlags {
		f.StringVar(&w.modelName, "m", "", "Model to operate in. Accepts [<controller name>:]<model name>")
		f.StringVar(&w.modelName, "model", "", "")
	}
	w.ModelCommand.SetFlags(f)
}

func (w *modelCommandWrapper) Init(args []string) error {
	store := w.ClientStore()
	if store == nil {
		store = jujuclient.NewFileClientStore()
	}
	store = QualifyingClientStore{store}
	w.SetClientStore(store)
	if !w.skipModelFlags {
		if w.modelName == "" && w.useDefaultModel {
			// Look for the default.
			defaultModel, err := GetCurrentModel(store)
			if err != nil {
				return err
			}
			w.modelName = defaultModel
		}
		if w.modelName == "" && !w.useDefaultModel {
			return errors.Trace(ErrNoModelSpecified)
		}
	}
	if w.modelName != "" {
		if err := w.SetModelName(w.modelName); err != nil {
			return errors.Annotate(err, "setting model name")
		}
	}
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

// SplitModelName splits a model name into its controller
// and model parts. If the model is unqualified, then the
// returned controller string will be empty, and the returned
// model string will be identical to the input.
func SplitModelName(name string) (controller, model string) {
	if i := strings.IndexRune(name, ':'); i >= 0 {
		return name[:i], name[i+1:]
	}
	return "", name
}

// JoinModelName joins a controller and model name into a
// qualified model name.
func JoinModelName(controller, model string) string {
	return fmt.Sprintf("%s:%s", controller, model)
}
