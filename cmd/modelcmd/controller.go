// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelcmd

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/controller"
	"github.com/juju/juju/api/modelmanager"
	"github.com/juju/juju/api/usermanager"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/jujuclient"
)

var (
	// ErrNoControllerSpecified is returned by commands that operate on
	// a controller if there is no current controller, no controller has been
	// explicitly specified, and there is no default controller.
	ErrNoControllerSpecified = errors.New("no controller specified")

	// ErrNoAccountSpecified is returned by commands that operate on a
	// controller if there is no current account associated with the
	// controller.
	ErrNoAccountSpecified = errors.New("no account specified")
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

	// SetControllerName is called prior to the wrapped command's Init method with
	// the active controller name. The controller name is guaranteed to be non-empty
	// at entry of Init. It records the current model name in the
	// ControllerCommandBase.
	SetControllerName(controllerName string) error

	// ControllerName returns the name of the controller or model used to
	// determine that API end point.
	ControllerName() string

	// SetAPIOpener allows the replacement of the default API opener,
	// which ends up calling NewAPIRoot
	SetAPIOpener(opener APIOpener)
}

// ControllerCommandBase is a convenience type for embedding in commands
// that wish to implement ControllerCommand.
type ControllerCommandBase struct {
	JujuCommandBase

	store          jujuclient.ClientStore
	controllerName string
	accountName    string

	// opener is the strategy used to open the API connection.
	opener APIOpener
}

// SetClientStore implements the ControllerCommand interface.
func (c *ControllerCommandBase) SetClientStore(store jujuclient.ClientStore) {
	c.store = store
}

// ClientStore implements the ControllerCommand interface.
func (c *ControllerCommandBase) ClientStore() jujuclient.ClientStore {
	return c.store
}

// SetControllerName implements the ControllerCommand interface.
func (c *ControllerCommandBase) SetControllerName(controllerName string) error {
	actualControllerName, err := ResolveControllerName(c.ClientStore(), controllerName)
	if err != nil {
		return errors.Trace(err)
	}
	controllerName = actualControllerName

	accountName, err := c.store.CurrentAccount(controllerName)
	if err != nil && !errors.IsNotFound(err) {
		return errors.Trace(err)
	}
	c.controllerName = controllerName
	c.accountName = accountName
	return nil
}

// ControllerName implements the ControllerCommand interface.
func (c *ControllerCommandBase) ControllerName() string {
	return c.controllerName
}

// AccountName implements the ControllerCommand interface.
func (c *ControllerCommandBase) AccountName() string {
	return c.accountName
}

// SetAPIOpener specifies the strategy used by the command to open
// the API connection.
func (c *ControllerCommandBase) SetAPIOpener(opener APIOpener) {
	c.opener = opener
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
	if c.controllerName == "" {
		return nil, errors.Trace(ErrNoControllerSpecified)
	}
	if c.accountName == "" {
		return nil, errors.Trace(ErrNoAccountSpecified)
	}
	opener := c.opener
	if opener == nil {
		opener = OpenFunc(c.JujuCommandBase.NewAPIRoot)
	}
	return opener.Open(c.store, c.controllerName, c.accountName, "")
}

// ConnectionCredentials returns the credentials used to connect to the API for
// the specified controller.
func (c *ControllerCommandBase) ConnectionCredentials() (configstore.APICredentials, error) {
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
// the specified controller.
func (c *ControllerCommandBase) ConnectionEndpoint() (configstore.APIEndpoint, error) {
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
func (c *ControllerCommandBase) ConnectionInfo() (configstore.EnvironInfo, error) {
	// TODO: the user may soon be specified through the command line
	// or through an environment setting, so return these when they are ready.
	if c.controllerName == "" {
		return nil, errors.Trace(ErrNoControllerSpecified)
	}
	info, err := connectionInfoForName(
		c.controllerName,
		configstore.AdminModelName(c.controllerName),
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return info, nil
}

// WrapControllerOption sets various parameters of the
// ControllerCommand wrapper.
type WrapControllerOption func(*sysCommandWrapper)

// ControllerSkipFlags instructs the wrapper to skip -c
// and --controller flag definition.
func ControllerSkipFlags(w *sysCommandWrapper) {
	w.setFlags = false
}

// ControllerSkipDefault instructs the wrapper not to
// use the default controller name.
func ControllerSkipDefault(w *sysCommandWrapper) {
	w.useDefaultControllerName = false
}

// ControllerAPIOpener instructs the underlying controller command to use a
// different APIOpener strategy.
func ControllerAPIOpener(opener APIOpener) WrapControllerOption {
	return func(w *sysCommandWrapper) {
		w.ControllerCommand.SetAPIOpener(opener)
	}
}

// WrapController wraps the specified ControllerCommand, returning a Command
// that proxies to each of the ControllerCommand methods.
func WrapController(c ControllerCommand, options ...WrapControllerOption) cmd.Command {
	wrapper := &sysCommandWrapper{
		ControllerCommand:        c,
		setFlags:                 true,
		useDefaultControllerName: true,
	}
	for _, option := range options {
		option(wrapper)
	}
	return WrapBase(wrapper)
}

type sysCommandWrapper struct {
	ControllerCommand
	setFlags                 bool
	useDefaultControllerName bool
	controllerName           string
}

// SetFlags implements Command.SetFlags, then calls the wrapped command's SetFlags.
func (w *sysCommandWrapper) SetFlags(f *gnuflag.FlagSet) {
	if w.setFlags {
		f.StringVar(&w.controllerName, "c", "", "juju controller to operate in")
		f.StringVar(&w.controllerName, "controller", "", "")
	}
	w.ControllerCommand.SetFlags(f)
}

func (w *sysCommandWrapper) getDefaultControllerName() (string, error) {
	if currentController, err := ReadCurrentController(); err != nil {
		return "", errors.Trace(err)
	} else if currentController != "" {
		return currentController, nil
	}
	return "", errors.Trace(ErrNoControllerSpecified)
}

// Init implements Command.Init, then calls the wrapped command's Init.
func (w *sysCommandWrapper) Init(args []string) error {
	store := w.ClientStore()
	if store == nil {
		store = jujuclient.NewFileClientStore()
		w.SetClientStore(store)
	}
	if w.setFlags {
		if w.controllerName == "" && w.useDefaultControllerName {
			name, err := w.getDefaultControllerName()
			if err != nil {
				return errors.Trace(err)
			}
			w.controllerName = name
		}
		if w.controllerName == "" && !w.useDefaultControllerName {
			return ErrNoControllerSpecified
		}
	}
	if w.controllerName != "" {
		if err := w.SetControllerName(w.controllerName); err != nil {
			return errors.Trace(err)
		}
	}
	return w.ControllerCommand.Init(args)
}

// ResolveControllerName returns the canonical name of a controller given
// an unambiguous identifier for that controller.
// Locally created controllers (i.e. those whose names begin with "local.")
// may be identified with or without the "local." prefix if there exists no
// other controller in the store with the same unprefixed name.
func ResolveControllerName(store jujuclient.ControllerStore, controllerName string) (string, error) {
	_, err := store.ControllerByName(controllerName)
	if err == nil {
		return controllerName, nil
	}
	if !errors.IsNotFound(err) {
		return "", err
	}
	var secondErr error
	localName := "local." + controllerName
	_, secondErr = store.ControllerByName(localName)
	// If fallback name not found, return the original error.
	if errors.IsNotFound(secondErr) {
		return "", err
	}
	return localName, secondErr
}
