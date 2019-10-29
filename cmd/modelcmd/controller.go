// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelcmd

import (
	"fmt"
	"net/http"
	"os"
	"sort"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/macaroon-bakery.v2-unstable/httpbakery"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/controller"
	"github.com/juju/juju/api/modelmanager"
	"github.com/juju/juju/api/usermanager"
	"github.com/juju/juju/cmd/juju/interact"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/jujuclient"
)

var (
	// ErrNoControllersDefined is returned by commands that operate on
	// a controller if there is no current controller, no controller has been
	// explicitly specified, and there is no default controller.
	ErrNoControllersDefined = errors.New(`No controllers registered.

Please either create a new controller using "juju bootstrap" or connect to
another controller that you have been given access to using "juju register".
`)
	// ErrNoCurrentController is returned by commands that operate on
	// a controller if there is no current controller, no controller has been
	// explicitly specified, and there is no default controller but there are
	// controllers that client knows about.
	ErrNoCurrentController = errors.New(`No selected controller.

Please use "juju switch" to select a controller.
`)
)

// ControllerCommand is intended to be a base for all commands
// that need to operate on controllers as opposed to models.
type ControllerCommand interface {
	Command

	// SetClientStore is called prior to the wrapped command's Init method
	// with the default controller store. It may also be called to override the
	// default controller store for testing.
	SetClientStore(jujuclient.ClientStore)

	// ClientStore returns the controller store that the command is
	// associated with.
	ClientStore() jujuclient.ClientStore

	// SetControllerName sets the name of the current controller.
	SetControllerName(controllerName string, allowDefault bool) error

	// ControllerName returns the name of the controller
	// that the command should use. It must only be called
	// after Run has been called.
	ControllerName() (string, error)

	// initModel initializes the controller, resolving an empty
	// controller to the current controller if allowDefault is true.
	initController() error
}

// ControllerCommandBase is a convenience type for embedding in commands
// that wish to implement ControllerCommand.
type ControllerCommandBase struct {
	CommandBase

	store jujuclient.ClientStore

	_controllerName        string
	allowDefaultController bool

	// doneInitController holds whether initController has been called.
	doneInitController bool

	// initControllerError holds the result of the initController call.
	initControllerError error
}

// SetClientStore implements the ControllerCommand interface.
func (c *ControllerCommandBase) SetClientStore(store jujuclient.ClientStore) {
	c.store = store
}

// ClientStore implements the ControllerCommand interface.
func (c *ControllerCommandBase) ClientStore() jujuclient.ClientStore {
	c.assertRunStarted()
	return c.store
}

func (c *ControllerCommandBase) initController() error {
	if c.doneInitController {
		return errors.Trace(c.initControllerError)
	}
	c.doneInitController = true
	c.initControllerError = c.initController0()
	return c.initControllerError
}

// DetermineCurrentController returns current controller on this client.
// It considers commandline flags, environment variables, and current config.
func DetermineCurrentController(store jujuclient.ClientStore) (string, error) {
	modelController, _ := SplitModelName(os.Getenv(osenv.JujuModelEnvKey))
	envController := os.Getenv(osenv.JujuControllerEnvKey)
	if modelController != "" && envController != "" && modelController != envController {
		return "", errors.Errorf("controller name from %v (%v) conflicts with value in %v (%v)",
			osenv.JujuModelEnvKey, modelController,
			osenv.JujuControllerEnvKey, envController,
		)
	}
	controllerName := modelController
	if controllerName == "" {
		controllerName = envController
	}
	if controllerName == "" {
		var err error
		controllerName, err = store.CurrentController()
		if err != nil {
			return "", errors.Trace(err)
		}
	}

	if _, err := store.ControllerByName(controllerName); err != nil {
		return "", errors.Trace(err)
	}
	return controllerName, nil
}

func (c *ControllerCommandBase) initController0() error {
	if c._controllerName == "" && !c.allowDefaultController {
		return errors.New("no controller specified")
	}
	if c._controllerName == "" {
		controllerName, err := DetermineCurrentController(c.store)
		if err != nil {
			return errors.Trace(translateControllerError(c.store, err))
		}
		c._controllerName = controllerName
	}
	if _, err := c.store.ControllerByName(c._controllerName); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// SetControllerName implements ControllerCommand.SetControllerName.
func (c *ControllerCommandBase) SetControllerName(controllerName string, allowDefault bool) error {
	logger.Infof("setting controllerName to %q %v", controllerName, allowDefault)
	c._controllerName = controllerName
	c.allowDefaultController = allowDefault
	if c.runStarted {
		if err := c.initController(); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// ControllerName implements the ControllerCommand interface.
func (c *ControllerCommandBase) ControllerName() (string, error) {
	c.assertRunStarted()
	if err := c.initController(); err != nil {
		return "", errors.Trace(err)
	}
	return c._controllerName, nil
}

func (c *ControllerCommandBase) BakeryClient() (*httpbakery.Client, error) {
	controllerName, err := c.ControllerName()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return c.CommandBase.BakeryClient(c.ClientStore(), controllerName)
}

func (c *ControllerCommandBase) CookieJar() (http.CookieJar, error) {
	controllerName, err := c.ControllerName()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return c.CommandBase.CookieJar(c.ClientStore(), controllerName)
}

// NewModelManagerAPIClient returns an API client for the
// ModelManager on the current controller using the current credentials.
func (c *ControllerCommandBase) NewModelManagerAPIClient() (*modelmanager.Client, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return modelmanager.NewClient(root), nil
}

// NewControllerAPIClient returns an API client for the Controller on
// the current controller using the current credentials.
func (c *ControllerCommandBase) NewControllerAPIClient() (*controller.Client, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return controller.NewClient(root), nil
}

// NewUserManagerAPIClient returns an API client for the UserManager on the
// current controller using the current credentials.
func (c *ControllerCommandBase) NewUserManagerAPIClient() (*usermanager.Client, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return usermanager.NewClient(root), nil
}

// NewAPIRoot returns a restricted API for the current controller using the current
// credentials.  Only the UserManager and ModelManager may be accessed
// through this API connection.
func (c *ControllerCommandBase) NewAPIRoot() (api.Connection, error) {
	return c.newAPIRoot("")
}

// NewAPIRoot returns a new connection to the API server for the named model
// in the specified controller.
func (c *ControllerCommandBase) NewModelAPIRoot(modelName string) (api.Connection, error) {
	controllerName, err := c.ControllerName()
	if err != nil {
		return nil, errors.Trace(err)
	}
	_, err = c.store.ModelByName(controllerName, modelName)
	if err != nil {
		if !errors.IsNotFound(err) {
			return nil, errors.Trace(err)
		}
		// The model isn't known locally, so query the models
		// available in the controller, and cache them locally.
		if err := c.RefreshModels(c.store, controllerName); err != nil {
			return nil, errors.Annotate(err, "refreshing models")
		}
	}
	return c.newAPIRoot(modelName)
}

func (c *ControllerCommandBase) newAPIRoot(modelName string) (api.Connection, error) {
	controllerName, err := c.ControllerName()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return c.CommandBase.NewAPIRoot(c.store, controllerName, modelName)
}

// ModelUUIDs returns the model UUIDs for the given model names.
func (c *ControllerCommandBase) ModelUUIDs(modelNames []string) ([]string, error) {
	controllerName, err := c.ControllerName()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return c.CommandBase.ModelUUIDs(c.ClientStore(), controllerName, modelNames)
}

// CurrentAccountDetails returns details of the account associated with
// the current controller.
func (c *ControllerCommandBase) CurrentAccountDetails() (*jujuclient.AccountDetails, error) {
	controllerName, err := c.ControllerName()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return c.ClientStore().AccountDetails(controllerName)
}

// WrapControllerOption specifies an option to the WrapController function.
type WrapControllerOption func(*controllerCommandWrapper)

// Options for the WrapController call.
var (
	// WrapControllerSkipControllerFlags specifies that the -c
	// and --controller flag flags should not be defined.
	WrapControllerSkipControllerFlags WrapControllerOption = wrapControllerSkipControllerFlags

	// WrapSkipDefaultModel specifies that no default controller should
	// be used.
	WrapControllerSkipDefaultController WrapControllerOption = wrapControllerSkipDefaultController
)

func wrapControllerSkipControllerFlags(w *controllerCommandWrapper) {
	w.setControllerFlags = false
}

func wrapControllerSkipDefaultController(w *controllerCommandWrapper) {
	w.useDefaultController = false
}

// WrapController wraps the specified ControllerCommand, returning a Command
// that proxies to each of the ControllerCommand methods.
func WrapController(c ControllerCommand, options ...WrapControllerOption) ControllerCommand {
	wrapper := &controllerCommandWrapper{
		ControllerCommand:    c,
		setControllerFlags:   true,
		useDefaultController: true,
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
		*controllerCommandWrapper
	}
	return struct {
		embed
		cmd.Command
	}{
		Command: WrapBase(wrapper),
		embed:   embed{wrapper},
	}
}

type controllerCommandWrapper struct {
	ControllerCommand
	setControllerFlags   bool
	useDefaultController bool
	controllerName       string
}

// wrapped implements wrapper.wrapped.
func (w *controllerCommandWrapper) inner() cmd.Command {
	return w.ControllerCommand
}

// SetFlags implements Command.SetFlags, then calls the wrapped command's SetFlags.
func (w *controllerCommandWrapper) SetFlags(f *gnuflag.FlagSet) {
	if w.setControllerFlags {
		f.StringVar(&w.controllerName, "c", "", "Controller to operate in")
		f.StringVar(&w.controllerName, "controller", "", "")
	}
	w.ControllerCommand.SetFlags(f)
}

// Init implements Command.Init, then calls the wrapped command's Init.
func (w *controllerCommandWrapper) Init(args []string) error {
	if w.setControllerFlags {
		if err := w.SetControllerName(w.controllerName, w.useDefaultController); err != nil {
			return errors.Trace(err)
		}
	}
	if err := w.ControllerCommand.Init(args); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (w *controllerCommandWrapper) Run(ctx *cmd.Context) error {
	w.setRunStarted()
	store := w.ClientStore()
	if store == nil {
		store = jujuclient.NewFileClientStore()
	}
	store = QualifyingClientStore{store}
	w.SetClientStore(store)
	return w.ControllerCommand.Run(ctx)
}

func translateControllerError(store jujuclient.ClientStore, err error) error {
	if !errors.IsNotFound(err) {
		return err
	}
	controllers, err2 := store.AllControllers()
	if err2 != nil {
		return err2
	}
	if len(controllers) == 0 {
		return errors.Wrap(err, ErrNoControllersDefined)
	}
	return errors.Wrap(err, ErrNoCurrentController)
}

// OptionalControllerCommand is used as a base for commands which can
// act locally or on a controller. It is primarily intended to be used
// by cloud and credential related commands which can either update a
// local client cache, or a running controller.
type OptionalControllerCommand struct {
	CommandBase
	Store jujuclient.ClientStore

	EnabledFlag string

	// Local stores whether a client side (aka local) copy is requested.
	Local bool

	// Client stores whether the command will operate on a client copy.
	Client bool

	ControllerName string

	SkipCurrentControllerPrompt bool
}

// SetFlags initializes the flags supported by the command.
func (c *OptionalControllerCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	f.BoolVar(&c.Client, "client", false, "Command to operate on or with a copy from this client")
	// TODO (juju3) remove me
	f.BoolVar(&c.Local, "local", false, "DEPRECATED (use --client-only): Local operation only; controller not affected")
	f.StringVar(&c.ControllerName, "c", "", "Controller to operate in")
	f.StringVar(&c.ControllerName, "controller", "", "")
	f.BoolVar(&c.SkipCurrentControllerPrompt, "no-prompt", false, "Skip prompting for confirmation to use current controller, always use it when detected")
}

// Init populates the command with the args from the command line.
func (c *OptionalControllerCommand) Init(args []string) (err error) {
	if c.Local && !c.Client {
		c.Client = c.Local
	}
	return nil
}

// MaybePrompt checks if the command was give a --client or --controller options.
// If not, it will prompt user to clarify whether the operation is to take place on
// a client copy, a controller copy or both.
func (c *OptionalControllerCommand) MaybePrompt(ctxt *cmd.Context, action string) error {
	if c.Client || c.ControllerName != "" {
		return nil
	}
	pollster := interact.New(ctxt.Stdin, ctxt.Stdout, interact.NewErrWriter(ctxt.Stdout))
	useClient, err := pollster.YN(fmt.Sprintf("This operation can be applied to both a copy on this client and a controller of your choice.\n"+
		"Do you want to %v this client", action), true)
	if err != nil {
		return errors.Trace(err)
	}
	c.Client = useClient
	useController, err := pollster.YN(fmt.Sprintf("Do you want to %v a controller", action), true)
	if err != nil {
		return errors.Trace(err)
	}
	if useController {
		return c.queryControllerName(ctxt, pollster)
	}
	return nil
}

func (c *OptionalControllerCommand) queryControllerName(ctxt *cmd.Context, pollster *interact.Pollster) error {
	all, err := c.Store.AllControllers()
	if err != nil {
		return errors.Trace(err)
	}
	if len(all) == 0 {
		// No controllers, so nothing to do.
		ctxt.Infof("No registered controllers on this client: either bootstrap one or register one.")
		return nil
	}

	if len(all) == 1 {
		cName := ""
		for n := range all {
			cName = n
			break
		}
		useController, err := pollster.YN(fmt.Sprintf("Only one controller %q is registered. Use it", cName), true)
		if err != nil {
			return errors.Trace(err)
		}
		if useController {
			c.ControllerName = cName
		}
		return nil
	}
	controllers := []string{}
	for one := range all {
		controllers = append(controllers, one)
	}
	sort.Strings(controllers)
	knownClouds := interact.VerifyOptions("controller", controllers, true)

	nameVerify := func(s string) (ok bool, errmsg string, err error) {
		ok, errmsg, err = knownClouds(s)
		if err != nil {
			return false, "", errors.Trace(err)
		}
		if ok {
			return true, "", nil
		}
		return false, errmsg, nil
	}

	defaultController, err := DetermineCurrentController(c.Store)
	if err != nil && !errors.IsNotFound(err) {
		return errors.Trace(err)
	}
	if defaultController == "" {
		defaultController = controllers[0]
	}
	cName, err := pollster.SelectVerify(interact.List{
		Singular: "controller name",
		Plural:   "controller names",
		Options:  controllers,
		Default:  defaultController,
	}, nameVerify)
	if err != nil {
		return err
	}
	c.ControllerName = cName
	return nil
}
