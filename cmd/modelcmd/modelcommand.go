// Copyright 2013-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelcmd

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/api"
	apiclient "github.com/juju/juju/api/client/client"
	"github.com/juju/juju/api/client/modelmanager"
	"github.com/juju/juju/api/client/modelupgrader"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/cmd"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/jujuclient"
)

var logger = internallogger.GetLogger("juju.cmd.modelcmd")

// ErrNoModelSpecified is returned by commands that operate on
// an environment if there is no current model, no model
// has been explicitly specified, and there is no default model.
var ErrNoModelSpecified = errors.New(`No model in focus.

Please use "juju models" to see models available to you.
You can set current model by running "juju switch"
or specify any other model on the command line using the "-m" option.
`)

// ModelCommand extends cmd.Command with a SetModelIdentifier method.
type ModelCommand interface {
	Command

	// SetClientStore is called prior to the wrapped command's Init method
	// with the default controller store. It may also be called to override the
	// default controller store for testing.
	SetClientStore(jujuclient.ClientStore)

	// ClientStore returns the controller store that the command is
	// associated with.
	ClientStore() jujuclient.ClientStore

	// SetModelIdentifier sets the model identifier for this command.
	// Setting the model identifier will also set the related controller name.
	// The model name can be qualified with a controller name
	// (controller:model), or unqualified, in which case it will be assumed
	// to be within the current controller.
	//
	// Passing an empty model name will choose the default
	// model, or return an error if there isn't one.
	//
	// SetModelIdentifier is called prior to the wrapped command's Init method
	// with the active model name.
	// The model identifier is guaranteed to be non-empty at entry of Init.
	SetModelIdentifier(modelIdentifier string, allowDefault bool) error

	// ModelIdentifier returns a string identifying the target model.
	// It may be a model name, or a full or partial model UUID.
	ModelIdentifier() (string, error)

	// ModelNameWithQualifier returns the target model name
	// and qualifier.
	ModelNameWithQualifier() (string, string, error)

	// ModelType returns the type of the model.
	ModelType(context.Context) (model.ModelType, error)

	// ControllerName returns the name of the controller that contains
	// the model returned by ModelIdentifier().
	ControllerName() (string, error)

	// ControllerDetails returns the details of the controller that contains
	// the model returned by ModelIdentifier().
	ControllerDetails() (*jujuclient.ControllerDetails, error)

	// maybeInitModel initializes the model name, resolving empty
	// model or controller parts to the current model or controller if
	// needed. It fails a model cannot be determined.
	maybeInitModel() error
}

// ModelCommandBase is a convenience type for embedding in commands
// that wish to implement ModelCommand.
type ModelCommandBase struct {
	CommandBase

	// store is the client controller store that contains information
	// about controllers, models, etc.
	store jujuclient.ClientStore

	// _modelIdentifier, _modelType, _modelGeneration and _controllerName hold
	// the current model identifier, controller name, model type and branch.
	// They are only valid after maybeInitModel is called, and should in
	// general not be accessed directly, but through ModelIdentifier and
	// ControllerName respectively.
	_modelIdentifier string
	_modelType       model.ModelType
	_controllerName  string

	allowDefaultModel bool

	// doneInitModel holds whether maybeInitModel has been called.
	doneInitModel bool

	// initModelError holds the result of the maybeInitModel call.
	initModelError error
}

// SetClientStore implements the ModelCommand interface.
func (c *ModelCommandBase) SetClientStore(store jujuclient.ClientStore) {
	c.store = store
}

// ClientStore implements the ModelCommand interface.
func (c *ModelCommandBase) ClientStore() jujuclient.ClientStore {
	// c.store is set in maybeInitModel() below.
	if c.store == nil && !c.runStarted {
		panic("inappropriate method called before init finished")
	}
	return c.store
}

func (c *ModelCommandBase) maybeInitModel() error {
	// maybeInitModel() might have been called previously before the actual command's
	// Init() method was invoked. If allowDefaultModel = false, then we would have
	// returned [ErrNoModelSpecified,ErrNoControllersDefined,ErrNoCurrentController]
	// at that point and so need to try again.
	// If any other error result was returned, we bail early here.
	noRetry := func(original error) bool {
		c := errors.Cause(c.initModelError)
		return c != ErrNoModelSpecified && c != ErrNoControllersDefined && !IsNoCurrentController(c)
	}
	if c.doneInitModel && noRetry(c.initModelError) {
		return errors.Trace(c.initModelError)
	}

	// A previous call to maybeInitModel returned
	// [ErrNoModelSpecified,ErrNoControllersDefined,ErrNoCurrentController],
	// so we try again because the model should now have been set.

	// Set up the client store if not already done.
	if !c.doneInitModel {
		store := c.store
		if store == nil {
			store = jujuclient.NewFileClientStore()
		}
		store = QualifyingClientStore{store}
		c.SetClientStore(store)
	}

	c.doneInitModel = true
	c.initModelError = c.initModel0()
	return errors.Trace(c.initModelError)
}

func (c *ModelCommandBase) initModel0() error {
	if c._modelIdentifier == "" && !c.allowDefaultModel {
		return errors.Trace(ErrNoModelSpecified)
	}
	if c._modelIdentifier == "" {
		c._modelIdentifier = os.Getenv(osenv.JujuModelEnvKey)
	}

	controllerName, modelIdentifier := SplitModelName(c._modelIdentifier)
	if controllerName == "" {
		currentController, err := DetermineCurrentController(c.store)
		if err != nil {
			return errors.Trace(translateControllerError(c.store, err))
		}
		controllerName = currentController
	} else if _, err := c.store.ControllerByName(controllerName); err != nil {
		return errors.Trace(err)
	}
	c._controllerName = controllerName

	if modelIdentifier == "" {
		currentModel, err := c.store.CurrentModel(controllerName)
		if err != nil {
			return errors.Trace(err)
		}
		modelIdentifier = currentModel
	}
	c._modelIdentifier = modelIdentifier
	return nil
}

// SetModelIdentifier implements the ModelCommand interface.
func (c *ModelCommandBase) SetModelIdentifier(modelIdentifier string, allowDefault bool) error {
	c._modelIdentifier = modelIdentifier
	c.allowDefaultModel = allowDefault

	// After setting the model name, we may need to ensure we have access to the
	// other model details if not already done.
	if err := c.maybeInitModel(); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// ModelIdentifier implements the ModelCommand interface.
func (c *ModelCommandBase) ModelIdentifier() (string, error) {
	c.assertRunStarted()
	if err := c.maybeInitModel(); err != nil {
		return "", errors.Trace(err)
	}
	return c._modelIdentifier, nil
}

// ModelNameWithQualifier returns the current model name
// with its qualifier if it has one.
func (c *ModelCommandBase) ModelNameWithQualifier() (string, string, error) {
	modelIdent, err := c.ModelIdentifier()
	if err != nil {
		return "", "", errors.Trace(err)
	}
	currentModel, _, err := c.modelFromStore(c._controllerName, modelIdent)
	if err != nil {
		return "", "", errors.Trace(err)
	}
	if !jujuclient.IsQualifiedModelName(currentModel) {
		return currentModel, "", nil
	}
	modelName, qualifier, err := jujuclient.SplitFullyQualifiedModelName(currentModel)
	if err != nil {
		return "", "", errors.Trace(err)
	}
	return modelName, qualifier, nil
}

// ModelType implements the ModelCommand interface.
func (c *ModelCommandBase) ModelType(ctx context.Context) (model.ModelType, error) {
	if c._modelType != "" {
		return c._modelType, nil
	}

	// If we need to look up the model type, we need to ensure we
	// have access to the model details.
	if err := c.maybeInitModel(); err != nil {
		return "", errors.Trace(err)
	}

	_, details, err := c.modelFromStore(c._controllerName, c._modelIdentifier)
	if err != nil {
		if !c.runStarted {
			return "", errors.Trace(err)
		}
		_, details, err = c.modelDetails(ctx, c._controllerName, c._modelIdentifier)
		if err != nil {
			return "", errors.Trace(err)
		}
	}
	c._modelType = details.ModelType
	return c._modelType, nil
}

// ControllerName implements the ModelCommand interface.
func (c *ModelCommandBase) ControllerName() (string, error) {
	c.assertRunStarted()
	if err := c.maybeInitModel(); err != nil {
		return "", errors.Trace(err)
	}
	return c._controllerName, nil
}

// ControllerDetails implements the ModelCommand interface.
func (c *ModelCommandBase) ControllerDetails() (*jujuclient.ControllerDetails, error) {
	c.assertRunStarted()
	if err := c.maybeInitModel(); err != nil {
		return nil, errors.Trace(err)
	}
	controllerDetails, err := c.store.ControllerByName(c._controllerName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return controllerDetails, nil
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

func (c *ModelCommandBase) NewAPIClient(ctx context.Context) (*apiclient.Client, error) {
	root, err := c.NewAPIRoot(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return apiclient.NewClient(root, logger), nil
}

// ModelDetails returns details from the file store for the model indicated by
// the currently set controller name and model identifier.
func (c *ModelCommandBase) ModelDetails(ctx context.Context) (string, *jujuclient.ModelDetails, error) {
	modelIdentifier, err := c.ModelIdentifier()
	if err != nil {
		return "", nil, errors.Trace(err)
	}
	controllerName, err := c.ControllerName()
	if err != nil {
		return "", nil, errors.Trace(err)
	}

	name, details, err := c.modelDetails(ctx, controllerName, modelIdentifier)
	return name, details, errors.Trace(err)
}

func (c *ModelCommandBase) modelDetails(ctx context.Context, controllerName, modelIdentifier string) (
	string, *jujuclient.ModelDetails, error,
) {
	if modelIdentifier == "" {
		return "", nil, errors.Trace(ErrNoModelSpecified)
	}

	name, details, err := c.modelFromStore(controllerName, modelIdentifier)
	if err != nil {
		if !errors.Is(err, errors.NotFound) {
			return "", nil, errors.Trace(err)
		}
		logger.Debugf(context.TODO(), "model %q not found, refreshing", modelIdentifier)
		// The model is not known locally, so query the models
		// available in the controller, and cache them locally.
		if err := c.RefreshModels(ctx, c.store, controllerName); err != nil {
			return "", nil, errors.Annotate(err, "refreshing models")
		}
		name, details, err = c.modelFromStore(controllerName, modelIdentifier)
	}
	return name, details, errors.Trace(err)
}

// modelFromStore attempts to retrieve details from the store, first under the
// assumption that the input identifier is a model name, then using treating
// the identifier as a full or partial model UUID.
// If a model is successfully located its name and details are returned.
func (c *ModelCommandBase) modelFromStore(controllerName, modelIdentifier string) (
	string, *jujuclient.ModelDetails, error,
) {
	// Check if the model identifier is a name that identifies a stored model.
	// This will be the most common case.
	details, err := c.store.ModelByName(controllerName, modelIdentifier)
	if err == nil {
		return modelIdentifier, details, nil
	}
	if !errors.Is(err, errors.NotFound) {
		return "", nil, errors.Trace(err)
	}

	// If the identifier is at 6 least characters long,
	// attempt to match one of the stored model UUIDs.
	if len(modelIdentifier) > 5 {
		models, err := c.store.AllModels(controllerName)
		if err != nil {
			return "", nil, errors.Trace(err)
		}

		for name, details := range models {
			if strings.HasPrefix(details.ModelUUID, modelIdentifier) {
				return name, &details, nil
			}
		}
		return "", nil, errors.NotFoundf("model %s:%s", controllerName, modelIdentifier)
	}

	// Keep the not-found error from the store if we have one.
	// This will preserve the user-qualified model identifier.
	return "", nil, errors.Trace(err)
}

// NewAPIRoot returns a new connection to the API server for the environment
// directed to the model specified on the command line.
func (c *ModelCommandBase) NewAPIRoot(ctx context.Context) (api.Connection, error) {
	return c.NewAPIRootWithAddressOverride(ctx, nil)
}

// NewAPIRootWithAddressOverride returns a new connection to the API server for the environment
// directed to the model specified on the command line, using any address overrides.
func (c *ModelCommandBase) NewAPIRootWithAddressOverride(ctx context.Context, addresses []string) (api.Connection, error) {
	// We need to call ModelDetails() here and not just ModelName() to force
	// a refresh of the internal model details if those are not yet stored locally.
	modelName, _, err := c.ModelDetails(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	conn, err := c.newAPIRoot(ctx, modelName, nil, addresses)
	return conn, errors.Trace(err)
}

// NewAPIRootWithDialOpts returns a new connection to the API server for the
// environment directed to the model specified on the command line (and with
// the given dial options if non-nil).
func (c *ModelCommandBase) NewAPIRootWithDialOpts(ctx context.Context, dialOpts *api.DialOpts) (api.Connection, error) {
	// We need to call ModelDetails() here and not just ModelName() to force
	// a refresh of the internal model details if those are not yet stored locally.
	modelName, _, err := c.ModelDetails(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	conn, err := c.newAPIRoot(ctx, modelName, dialOpts, nil)
	return conn, errors.Trace(err)
}

// NewControllerAPIRoot returns a new connection to the API server for the environment
// directed to the controller specified on the command line.
// This is for the use of model-centered commands that still want
// to talk to controller-only APIs.
func (c *ModelCommandBase) NewControllerAPIRoot(ctx context.Context) (api.Connection, error) {
	return c.newAPIRoot(ctx, "", nil, nil)
}

// newAPIRoot is the internal implementation of NewAPIRoot and NewControllerAPIRoot;
// if modelName is empty, it makes a controller-only connection.
func (c *ModelCommandBase) newAPIRoot(ctx context.Context, modelName string, dialOpts *api.DialOpts, addressOverride []string) (api.Connection, error) {
	controllerName, err := c.ControllerName()
	if err != nil {
		return nil, errors.Trace(err)
	}
	conn, err := c.CommandBase.NewAPIRootWithDialOpts(ctx, c.store, controllerName, modelName, addressOverride, dialOpts)
	return conn, errors.Trace(err)
}

// ModelUUIDs returns the model UUIDs for the given model names.
func (c *ModelCommandBase) ModelUUIDs(ctx context.Context, modelNames []string) ([]string, error) {
	controllerName, err := c.ControllerName()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return c.CommandBase.ModelUUIDs(ctx, c.ClientStore(), controllerName, modelNames)
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
func (c *ModelCommandBase) NewModelManagerAPIClient(ctx context.Context) (*modelmanager.Client, error) {
	root, err := c.NewControllerAPIRoot(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return modelmanager.NewClient(root), nil
}

// NewModelUpgraderAPIClient returns an API client for the
// ModelUpgrader on the current controller using the current credentials.
func (c *ModelCommandBase) NewModelUpgraderAPIClient(ctx context.Context) (*modelupgrader.Client, error) {
	root, err := c.NewAPIRoot(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return modelupgrader.NewClient(root), nil
}

// WrapOption specifies an option to the Wrap function.
type WrapOption func(*modelCommandWrapper)

// Options for the Wrap function.
var (
	// WrapSkipModelFlags specifies that the -m and --model flags
	// should not be defined.
	WrapSkipModelFlags WrapOption = wrapSkipModelFlags

	// WrapSkipModelInit specifies that then initial model won't be initialised,
	// but later requests to the model should work.
	WrapSkipModelInit WrapOption = wrapSkipModelInit

	// WrapSkipDefaultModel specifies that no default model should
	// be used.
	WrapSkipDefaultModel WrapOption = wrapSkipDefaultModel
)

func wrapSkipModelFlags(w *modelCommandWrapper) {
	w.skipModelFlags = true
}

func wrapSkipModelInit(w *modelCommandWrapper) {
	w.skipModelInit = true
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
		skipModelInit:   false,
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
	skipModelInit   bool
	useDefaultModel bool

	// modelIdentifier may be a model name, a full model UUID,
	// or a short 6-8 model UUID prefix.
	modelIdentifier string
}

func (w *modelCommandWrapper) inner() cmd.Command {
	return w.ModelCommand
}

type modelSpecificCommand interface {
	// IncompatibleModel returns an error if the command is being run against
	// a model with which it is not compatible.
	IncompatibleModel(err error) error
}

// IAASOnlyCommand is used as a marker and is embedded
// by commands which should only run in IAAS models.
type IAASOnlyCommand interface {
	_iaasonly() // not implemented, marker only.
}

// CAASOnlyCommand is used as a marker and is embedded
// by commands which should only run in CAAS models.
type CAASOnlyCommand interface {
	_caasonly() // not implemented, marker only.
}

// validateCommandForModelType returns an error if an IAAS-only command
// is run on a CAAS model.
func (w *modelCommandWrapper) validateCommandForModelType(ctx context.Context, runStarted bool) error {
	_, iaasOnly := w.inner().(IAASOnlyCommand)
	_, caasOnly := w.inner().(CAASOnlyCommand)
	if !caasOnly && !iaasOnly {
		return nil
	}

	modelType, err := w.ModelCommand.ModelType(ctx)
	if err != nil {
		err = errors.Cause(err)
		// We need to error if Run() has been invoked the model is known and there was
		// some other error. If the model is not yet known, we'll grab the details
		// during the Run() API call later.
		if runStarted || (err != ErrNoModelSpecified && !errors.Is(err, errors.NotFound)) {
			return errors.Trace(err)
		}
		return nil
	}
	if modelType == model.CAAS && iaasOnly {
		err = errors.Errorf("Juju command %q not supported on container models", w.Info().Name)
	}
	if modelType == model.IAAS && caasOnly {
		err = errors.Errorf("Juju command %q only supported on k8s container models", w.Info().Name)
	}

	if c, ok := w.inner().(modelSpecificCommand); ok {
		return c.IncompatibleModel(err)
	}
	return err
}

func (w *modelCommandWrapper) Init(args []string) error {
	// The following is a bit convoluted, but requires that we either set the
	// model and/or validate the model depending the options passed in.
	var (
		shouldSetModel      = true
		shouldValidateModel = true
	)

	if w.skipModelFlags {
		shouldSetModel = false
	}
	if w.skipModelInit {
		shouldSetModel = false
		shouldValidateModel = false
	}

	if shouldSetModel {
		if err := w.ModelCommand.SetModelIdentifier(w.modelIdentifier, w.useDefaultModel); err != nil {
			return errors.Trace(err)
		}
	}
	if shouldValidateModel {
		// If we are able to get access to the model type before running the actual
		// command's Init(), we can bail early if the command is not supported for the
		// specific model type. Otherwise, if the command is one which doesn't allow a
		// default model, we need to wait till Run() is invoked.
		if err := w.validateCommandForModelType(context.TODO(), false); err != nil {
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

	// Some commands are only supported on IAAS models.
	if !w.skipModelInit {
		if err := w.validateCommandForModelType(ctx, true); err != nil {
			return errors.Trace(err)
		}
	}
	err := w.ModelCommand.Run(ctx)
	if redirErr, ok := errors.Cause(err).(*api.RedirectError); ok {
		modelIdentifier, _ := w.ModelCommand.ModelIdentifier()
		return newModelMigratedError(store, modelIdentifier, redirErr)
	}
	return err
}

func (w *modelCommandWrapper) SetFlags(f *gnuflag.FlagSet) {
	if !w.skipModelFlags {
		desc := "Model to operate in. Accepts [<controller name>:]<model name>|<model UUID>"
		f.StringVar(&w.modelIdentifier, "m", "", desc)
		f.StringVar(&w.modelIdentifier, "model", "", "")
	}
	w.ModelCommand.SetFlags(f)
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
