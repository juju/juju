// Copyright 2013-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelcmd

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"
	"gopkg.in/macaroon-bakery.v1/httpbakery"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/modelmanager"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/jujuclient"
)

var logger = loggo.GetLogger("juju.cmd.modelcmd")

// ErrNoModelSpecified is returned by commands that operate on
// an environment if there is no current model, no model
// has been explicitly specified, and there is no default model.
var ErrNoModelSpecified = errors.New(`No model in focus.

Please use "juju models" to see models available to you.
You can set current model by running "juju switch"
or specify any other model on the command line using the "-m" flag.
`)

// ModelCommand extends cmd.Command with a SetModelName method.
type ModelCommand interface {
	Command

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
	// Passing an empty model name will choose the default
	// model, or return an error if there isn't one.
	//
	// SetModelName is called prior to the wrapped command's Init method
	// with the active model name. The model name is guaranteed
	// to be non-empty at entry of Init.
	SetModelName(modelName string, allowDefault bool) error

	// ModelName returns the name of the model.
	ModelName() (string, error)

	// ControllerName returns the name of the controller that contains
	// the model returned by ModelName().
	ControllerName() (string, error)

	// initModel initializes the model name, resolving empty
	// model or controller parts to the current model or controller if
	// needed. It fails a model cannot be determined.
	initModel() error
}

// ModelCommandBase is a convenience type for embedding in commands
// that wish to implement ModelCommand.
type ModelCommandBase struct {
	CommandBase

	// store is the client controller store that contains information
	// about controllers, models, etc.
	store jujuclient.ClientStore

	// _modelName and _controllerName hold the current
	// model and controller names. They are only valid
	// after initModel is called, and should in general
	// not be accessed directly, but through ModelName and
	// ControllerName respectively.
	_modelName      string
	_controllerName string

	allowDefaultModel bool

	// doneInitModel holds whether initModel has been called.
	doneInitModel bool

	// initModelError holds the result of the initModel call.
	initModelError error
}

// SetClientStore implements the ModelCommand interface.
func (c *ModelCommandBase) SetClientStore(store jujuclient.ClientStore) {
	c.store = store
}

// ClientStore implements the ModelCommand interface.
func (c *ModelCommandBase) ClientStore() jujuclient.ClientStore {
	c.assertRunStarted()
	return c.store
}

func (c *ModelCommandBase) initModel() error {
	if c.doneInitModel {
		return errors.Trace(c.initModelError)
	}
	c.doneInitModel = true
	c.initModelError = c.initModel0()
	return errors.Trace(c.initModelError)
}

func (c *ModelCommandBase) initModel0() error {
	if c._modelName == "" && !c.allowDefaultModel {
		return errors.Trace(ErrNoModelSpecified)
	}
	if c._modelName == "" {
		c._modelName = os.Getenv(osenv.JujuModelEnvKey)
	}
	controllerName, modelName := SplitModelName(c._modelName)
	if controllerName == "" {
		currentController, err := c.store.CurrentController()
		if err != nil {
			return errors.Trace(translateControllerError(c.store, err))
		}
		controllerName = currentController
	} else if _, err := c.store.ControllerByName(controllerName); err != nil {
		return errors.Trace(err)
	}
	c._controllerName = controllerName
	if modelName == "" {
		currentModel, err := c.store.CurrentModel(controllerName)
		if err != nil {
			return errors.Trace(err)
		}
		modelName = currentModel
	}
	c._modelName = modelName
	return nil
}

// SetModelName implements the ModelCommand interface.
func (c *ModelCommandBase) SetModelName(modelName string, allowDefault bool) error {
	c._modelName = modelName
	c.allowDefaultModel = allowDefault
	if c.runStarted {
		if err := c.initModel(); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// ModelName implements the ModelCommand interface.
func (c *ModelCommandBase) ModelName() (string, error) {
	c.assertRunStarted()
	if err := c.initModel(); err != nil {
		return "", errors.Trace(err)
	}
	return c._modelName, nil
}

// ControllerName implements the ModelCommand interface.
func (c *ModelCommandBase) ControllerName() (string, error) {
	c.assertRunStarted()
	if err := c.initModel(); err != nil {
		return "", errors.Trace(err)
	}
	return c._controllerName, nil
}

func (c *ModelCommandBase) BakeryClient() (*httpbakery.Client, error) {
	controllerName, err := c.ControllerName()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return c.CommandBase.BakeryClient(c.ClientStore(), controllerName)
}

func (c *ModelCommandBase) CookieJar() (http.CookieJar, error) {
	controllerName, err := c.ControllerName()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return c.CommandBase.CookieJar(c.ClientStore(), controllerName)
}

func (c *ModelCommandBase) NewAPIClient() (*api.Client, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return root.Client(), nil
}

func (c *ModelCommandBase) ModelDetails() (string, *jujuclient.ModelDetails, error) {
	modelName, err := c.ModelName()
	if err != nil {
		return "", nil, errors.Trace(err)
	}
	controllerName, err := c.ControllerName()
	if err != nil {
		return "", nil, errors.Trace(err)
	}
	if modelName == "" {
		return "", nil, errors.Trace(ErrNoModelSpecified)
	}
	details, err := c.store.ModelByName(controllerName, modelName)
	if err != nil {
		if !errors.IsNotFound(err) {
			return "", nil, errors.Trace(err)
		}
		// The model isn't known locally, so query the models
		// available in the controller, and cache them locally.
		if err := c.RefreshModels(c.store, controllerName); err != nil {
			return "", nil, errors.Annotate(err, "refreshing models")
		}
		details, err = c.store.ModelByName(controllerName, modelName)
	}
	return modelName, details, err
}

// NewAPIRoot returns a new connection to the API server for the environment
// directed to the model specified on the command line.
func (c *ModelCommandBase) NewAPIRoot() (api.Connection, error) {
	modelName, _, err := c.ModelDetails()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return c.newAPIRoot(modelName)
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
	controllerName, err := c.ControllerName()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return c.CommandBase.NewAPIRoot(c.store, controllerName, modelName)
}

// ModelUUIDs returns the model UUIDs for the given model names.
func (c *ModelCommandBase) ModelUUIDs(modelNames []string) ([]string, error) {
	controllerName, err := c.ControllerName()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return c.CommandBase.ModelUUIDs(c.ClientStore(), controllerName, modelNames)
}

// CurrentAccountDetails returns details of the account associated with
// the current controller.
func (c *ModelCommandBase) CurrentAccountDetails() (*jujuclient.AccountDetails, error) {
	controllerName, err := c.ControllerName()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return c.ClientStore().AccountDetails(controllerName)
}

// NewModelManagerAPIClient returns an API client for the
// ModelManager on the current controller using the current credentials.
func (c *ModelCommandBase) NewModelManagerAPIClient() (*modelmanager.Client, error) {
	root, err := c.NewControllerAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return modelmanager.NewClient(root), nil
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

// Wrap wraps the specified ModelCommand, returning a ModelCommand
// that proxies to each of the ModelCommand methods.
// Any provided options are applied to the wrapped command
// before it is returned.
func Wrap(c ModelCommand, options ...WrapOption) ModelCommand {
	wrapper := &modelCommandWrapper{
		ModelCommand:    c,
		skipModelFlags:  false,
		useDefaultModel: true,
	}
	for _, option := range options {
		option(wrapper)
	}
	// Define a new type so that we can embed the ModelCommand
	// interface one level deeper than cmd.Command, so that
	// we'll get the Command methods from WrapBase
	// and all the ModelCommand methods not in cmd.Command
	// from modelCommandWrapper.
	type embed struct {
		*modelCommandWrapper
	}
	return struct {
		embed
		cmd.Command
	}{
		Command: WrapBase(wrapper),
		embed:   embed{wrapper},
	}
}

type modelCommandWrapper struct {
	ModelCommand

	skipModelFlags  bool
	useDefaultModel bool
	modelName       string
}

func (w *modelCommandWrapper) inner() cmd.Command {
	return w.ModelCommand
}

func (w *modelCommandWrapper) Init(args []string) error {
	if !w.skipModelFlags {
		if err := w.ModelCommand.SetModelName(w.modelName, w.useDefaultModel); err != nil {
			return errors.Trace(err)
		}
	}
	if err := w.ModelCommand.Init(args); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (w *modelCommandWrapper) Run(ctx *cmd.Context) error {
	w.setRunStarted()
	store := w.ClientStore()
	if store == nil {
		store = jujuclient.NewFileClientStore()
	}
	store = QualifyingClientStore{store}
	w.SetClientStore(store)
	return w.ModelCommand.Run(ctx)
}

func (w *modelCommandWrapper) SetFlags(f *gnuflag.FlagSet) {
	if !w.skipModelFlags {
		f.StringVar(&w.modelName, "m", "", "Model to operate in. Accepts [<controller name>:]<model name>")
		f.StringVar(&w.modelName, "model", "", "")
	}
	w.ModelCommand.SetFlags(f)
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
