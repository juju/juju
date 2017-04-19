// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelcmd

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/controller"
	"github.com/juju/juju/api/modelmanager"
	"github.com/juju/juju/api/usermanager"
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
	CommandBase

	// SetClientStore is called prior to the wrapped command's Init method
	// with the default controller store. It may also be called to override the
	// default controller store for testing.
	SetClientStore(jujuclient.ClientStore)

	// ClientStore returns the controller store that the command is
	// associated with.
	ClientStore() jujuclient.ClientStore

	// SetControllerName sets the value returned by ControllerName.
	// It returns an error if the controller with the given name
	// is not found, unless the name is empty and allowDefault is true,
	// in which case the name of the current controller will be used.
	SetControllerName(controllerName string, allowDefault bool) error

	// ControllerName returns the name of the controller
	// that the command should use.
	ControllerName() string
}

// ControllerCommandBase is a convenience type for embedding in commands
// that wish to implement ControllerCommand.
type ControllerCommandBase struct {
	JujuCommandBase

	store          jujuclient.ClientStore
	controllerName string
}

// SetClientStore implements the ControllerCommand interface.
func (c *ControllerCommandBase) SetClientStore(store jujuclient.ClientStore) {
	c.store = store
}

// ClientStore implements the ControllerCommand interface.
func (c *ControllerCommandBase) ClientStore() jujuclient.ClientStore {
	return c.store
}

// SetControllerName implements ControllerCommand.SetControllerName.
func (c *ControllerCommandBase) SetControllerName(controllerName string, allowDefault bool) error {
	store := c.ClientStore()
	if controllerName == "" && allowDefault {
		currentController, err := store.CurrentController()
		if err != nil {
			return translateControllerError(store, err)
		}
		controllerName = currentController
	} else if _, err := store.ControllerByName(controllerName); err != nil {
		return errors.Trace(err)
	}
	c.controllerName = controllerName
	return nil
}

// ControllerName implements the ControllerCommand interface.
func (c *ControllerCommandBase) ControllerName() string {
	return c.controllerName
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
	_, err := c.store.ModelByName(c.controllerName, modelName)
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
	return c.newAPIRoot(modelName)
}

func (c *ControllerCommandBase) newAPIRoot(modelName string) (api.Connection, error) {
	if c.controllerName == "" {
		controllers, err := c.store.AllControllers()
		if err != nil {
			return nil, errors.Trace(err)
		}
		if len(controllers) == 0 {
			return nil, errors.Trace(ErrNoControllersDefined)
		}
		return nil, errors.Trace(ErrNoCurrentController)
	}
	return c.JujuCommandBase.NewAPIRoot(c.store, c.controllerName, modelName)
}

// ModelUUIDs returns the model UUIDs for the given model names.
func (c *ControllerCommandBase) ModelUUIDs(modelNames []string) ([]string, error) {
	var result []string
	store := c.ClientStore()
	controllerName := c.ControllerName()
	for _, modelName := range modelNames {
		model, err := store.ModelByName(controllerName, modelName)
		if errors.IsNotFound(err) {
			// The model isn't known locally, so query the models available in the controller.
			logger.Infof("model %q not cached locally, refreshing models from controller", modelName)
			if err := c.RefreshModels(store, controllerName); err != nil {
				return nil, errors.Annotatef(err, "refreshing model %q", modelName)
			}
			model, err = store.ModelByName(controllerName, modelName)
		}
		if err != nil {
			return nil, errors.Annotatef(err, "model %q not found", modelName)
		}
		result = append(result, model.ModelUUID)
	}
	return result, nil
}

// WrapControllerOption specifies an option to the WrapController function.
type WrapControllerOption func(*sysCommandWrapper)

// Options for the WrapController call.
var (
	// WrapControllerSkipControllerFlags specifies that the -c
	// and --controller flag flags should not be defined.
	WrapControllerSkipControllerFlags WrapControllerOption = wrapControllerSkipControllerFlags

	// WrapSkipDefaultModel specifies that no default controller should
	// be used.
	WrapControllerSkipDefaultController WrapControllerOption = wrapControllerSkipDefaultController
)

func wrapControllerSkipControllerFlags(w *sysCommandWrapper) {
	w.setControllerFlags = false
}

func wrapControllerSkipDefaultController(w *sysCommandWrapper) {
	w.useDefaultController = false
}

// WrapController wraps the specified ControllerCommand, returning a Command
// that proxies to each of the ControllerCommand methods.
func WrapController(c ControllerCommand, options ...WrapControllerOption) CommandBase {
	wrapper := &sysCommandWrapper{
		ControllerCommand:    c,
		setControllerFlags:   true,
		useDefaultController: true,
	}
	for _, option := range options {
		option(wrapper)
	}
	return WrapBase(wrapper)
}

type sysCommandWrapper struct {
	ControllerCommand
	setControllerFlags   bool
	useDefaultController bool
	controllerName       string
}

// wrapped implements wrapper.wrapped.
func (w *sysCommandWrapper) inner() cmd.Command {
	return w.ControllerCommand
}

// SetFlags implements Command.SetFlags, then calls the wrapped command's SetFlags.
func (w *sysCommandWrapper) SetFlags(f *gnuflag.FlagSet) {
	if w.setControllerFlags {
		f.StringVar(&w.controllerName, "c", "", "Controller to operate in")
		f.StringVar(&w.controllerName, "controller", "", "")
	}
	w.ControllerCommand.SetFlags(f)
}

// Init implements Command.Init, then calls the wrapped command's Init.
func (w *sysCommandWrapper) Init(args []string) error {
	store := w.ClientStore()
	if store == nil {
		store = jujuclient.NewFileClientStore()
	}
	store = QualifyingClientStore{store}
	w.SetClientStore(store)

	if w.setControllerFlags {
		if err := w.SetControllerName(w.controllerName, w.useDefaultController); err != nil {
			return errors.Trace(err)
		}
	}
	return w.ControllerCommand.Init(args)
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
