// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelcmd

import (
	"fmt"
	"net/http"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/modelmanager"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/juju"
	"github.com/juju/juju/jujuclient"
)

var errNoNameSpecified = errors.New("no name specified")

// CommandBase extends cmd.Command with a closeContext method.
// It is implicitly implemented by any type that embeds JujuCommandBase.
type CommandBase interface {
	cmd.Command

	// closeContext closes the command's API context.
	closeContext()
	setCmdContext(*cmd.Context)
}

// ModelAPI provides access to the model client facade methods.
type ModelAPI interface {
	ListModels(user string) ([]base.UserModel, error)
	Close() error
}

// JujuCommandBase is a convenience type for embedding that need
// an API connection.
type JujuCommandBase struct {
	cmd.CommandBase
	cmdContext  *cmd.Context
	apiContext  *APIContext
	modelApi    ModelAPI
	apiOpenFunc api.OpenFunc
}

// closeContext closes the command's API context
// if it has actually been created.
func (c *JujuCommandBase) closeContext() {
	if c.apiContext != nil {
		if err := c.apiContext.Close(); err != nil {
			logger.Errorf("%v", err)
		}
	}
}

// SetModelApi sets the api used to access model information.
func (c *JujuCommandBase) SetModelApi(api ModelAPI) {
	c.modelApi = api
}

// SetAPIOpen sets the function used for opening an API connection.
func (c *JujuCommandBase) SetAPIOpen(apiOpen api.OpenFunc) {
	c.apiOpenFunc = apiOpen
}

func (c *JujuCommandBase) modelAPI(store jujuclient.ClientStore, controllerName, accountName string) (ModelAPI, error) {
	if c.modelApi != nil {
		return c.modelApi, nil
	}
	conn, err := c.NewAPIRoot(store, controllerName, accountName, "")
	if err != nil {
		return nil, errors.Trace(err)
	}
	c.modelApi = modelmanager.NewClient(conn)
	return c.modelApi, nil
}

// NewAPIRoot returns a new connection to the API server for the given
// model or controller.
func (c *JujuCommandBase) NewAPIRoot(
	store jujuclient.ClientStore,
	controllerName, accountName, modelName string,
) (api.Connection, error) {
	params, err := c.NewAPIConnectionParams(
		store, controllerName, accountName, modelName,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return juju.NewAPIConnection(params)
}

// NewAPIConnectionParams returns a juju.NewAPIConnectionParams with the
// given arguments such that a call to juju.NewAPIConnection with the
// result behaves the same as a call to JujuCommandBase.NewAPIRoot with
// the same arguments.
func (c *JujuCommandBase) NewAPIConnectionParams(
	store jujuclient.ClientStore,
	controllerName, accountName, modelName string,
) (juju.NewAPIConnectionParams, error) {
	if err := c.initAPIContext(); err != nil {
		return juju.NewAPIConnectionParams{}, errors.Trace(err)
	}
	return newAPIConnectionParams(
		store, controllerName, accountName, modelName,
		c.apiContext.BakeryClient, c.apiOpen,
	)
}

// HTTPClient returns an http.Client that contains the loaded
// persistent cookie jar. Note that this client is not good for
// connecting to the Juju API itself because it does not
// have the correct TLS setup - use api.Connection.HTTPClient
// for that.
func (c *JujuCommandBase) HTTPClient() (*http.Client, error) {
	if err := c.initAPIContext(); err != nil {
		return nil, errors.Trace(err)
	}
	return c.apiContext.BakeryClient.Client, nil
}

// BakeryClient returns a macaroon bakery client that
// uses the same HTTP client returned by HTTPClient.
func (c *JujuCommandBase) BakeryClient() (*httpbakery.Client, error) {
	if err := c.initAPIContext(); err != nil {
		return nil, errors.Trace(err)
	}
	return c.apiContext.BakeryClient, nil
}

// APIOpen establishes a connection to the API server using the
// the given api.Info and api.DialOpts.
func (c *JujuCommandBase) APIOpen(info *api.Info, opts api.DialOpts) (api.Connection, error) {
	if err := c.initAPIContext(); err != nil {
		return nil, errors.Trace(err)
	}
	return c.apiOpen(info, opts)
}

// RefreshModels refreshes the local models cache for the current user
// on the specified controller.
func (c *JujuCommandBase) RefreshModels(store jujuclient.ClientStore, controllerName, accountName string) error {
	accountDetails, err := store.AccountByName(controllerName, accountName)
	if err != nil {
		return errors.Trace(err)
	}

	modelManager, err := c.modelAPI(store, controllerName, accountName)
	if err != nil {
		return errors.Trace(err)
	}
	defer modelManager.Close()

	models, err := modelManager.ListModels(accountDetails.User)
	if err != nil {
		return errors.Trace(err)
	}
	for _, model := range models {
		modelDetails := jujuclient.ModelDetails{model.UUID}
		if err := store.UpdateModel(controllerName, accountName, model.Name, modelDetails); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// initAPIContext lazily initializes c.apiContext. Doing this lazily means that
// we avoid unnecessarily loading and saving the cookies
// when a command does not actually make an API connection.
func (c *JujuCommandBase) initAPIContext() error {
	if c.apiContext != nil {
		return nil
	}
	apiContext, err := NewAPIContext(c.cmdContext)
	if err != nil {
		return errors.Trace(err)
	}
	c.apiContext = apiContext
	return nil
}

func (c *JujuCommandBase) setCmdContext(ctx *cmd.Context) {
	c.cmdContext = ctx
}

// apiOpen establishes a connection to the API server using the
// the give api.Info and api.DialOpts.
func (c *JujuCommandBase) apiOpen(info *api.Info, opts api.DialOpts) (api.Connection, error) {
	if c.apiOpenFunc != nil {
		return c.apiOpenFunc(info, opts)
	}
	return api.Open(info, opts)
}

// WrapBase wraps the specified CommandBase, returning a Command
// that proxies to each of the CommandBase methods.
func WrapBase(c CommandBase) cmd.Command {
	return &baseCommandWrapper{
		CommandBase: c,
	}
}

type baseCommandWrapper struct {
	CommandBase
}

// Run implements Command.Run.
func (w *baseCommandWrapper) Run(ctx *cmd.Context) error {
	defer w.closeContext()
	w.setCmdContext(ctx)
	return w.CommandBase.Run(ctx)
}

// SetFlags implements Command.SetFlags.
func (w *baseCommandWrapper) SetFlags(f *gnuflag.FlagSet) {
	w.CommandBase.SetFlags(f)
}

// Init implements Command.Init.
func (w *baseCommandWrapper) Init(args []string) error {
	return w.CommandBase.Init(args)
}

func newAPIConnectionParams(
	store jujuclient.ClientStore,
	controllerName,
	accountName,
	modelName string,
	bakery *httpbakery.Client,
	apiOpen api.OpenFunc,
) (juju.NewAPIConnectionParams, error) {
	if controllerName == "" {
		return juju.NewAPIConnectionParams{}, errors.Trace(errNoNameSpecified)
	}
	var accountDetails *jujuclient.AccountDetails
	if accountName != "" {
		var err error
		accountDetails, err = store.AccountByName(controllerName, accountName)
		if err != nil {
			return juju.NewAPIConnectionParams{}, errors.Trace(err)
		}
	}
	var modelUUID string
	if modelName != "" {
		modelDetails, err := store.ModelByName(controllerName, accountName, modelName)
		if err != nil {
			return juju.NewAPIConnectionParams{}, errors.Trace(err)
		}
		modelUUID = modelDetails.ModelUUID
	}
	dialOpts := api.DefaultDialOpts()
	dialOpts.BakeryClient = bakery

	openAPI := func(info *api.Info, opts api.DialOpts) (api.Connection, error) {
		conn, err := apiOpen(info, opts)
		if err != nil {
			userTag, ok := info.Tag.(names.UserTag)
			if ok && userTag.IsLocal() && params.IsCodeLoginExpired(err) {
				// This is a bit gross, but we don't seem to have
				// a way of having an error with a cause that does
				// not influence the error message. We want to keep
				// the type/code so we don't lose the fact that the
				// error was caused by an API login expiry.
				return nil, &params.Error{
					Code: params.CodeLoginExpired,
					Message: fmt.Sprintf(`login expired

Your login for the %q controller has expired.
To log back in, run the following command:

    juju login %v
`, controllerName, userTag.Name()),
				}
			}
			return nil, err
		}
		return conn, nil
	}

	return juju.NewAPIConnectionParams{
		Store:           store,
		ControllerName:  controllerName,
		BootstrapConfig: NewGetBootstrapConfigFunc(store),
		AccountDetails:  accountDetails,
		ModelUUID:       modelUUID,
		DialOpts:        dialOpts,
		OpenAPI:         openAPI,
	}, nil
}

// NewGetBootstrapConfigFunc returns a function that, given a controller name,
// returns the bootstrap config for that controller in the given client store.
func NewGetBootstrapConfigFunc(store jujuclient.ClientStore) func(string) (*config.Config, error) {
	return bootstrapConfigGetter{store}.getBootstrapConfig
}

type bootstrapConfigGetter struct {
	jujuclient.ClientStore
}

func (g bootstrapConfigGetter) getBootstrapConfig(controllerName string) (*config.Config, error) {
	if _, err := g.ClientStore.ControllerByName(controllerName); err != nil {
		return nil, errors.Annotate(err, "resolving controller name")
	}
	bootstrapConfig, err := g.BootstrapConfigForController(controllerName)
	if err != nil {
		return nil, errors.Annotate(err, "getting bootstrap config")
	}
	cloudType, ok := bootstrapConfig.Config["type"].(string)
	if !ok {
		return nil, errors.NotFoundf("cloud type in bootstrap config")
	}

	var credential *cloud.Credential
	if bootstrapConfig.Credential != "" {
		credential, _, _, err = GetCredentials(
			g.ClientStore,
			bootstrapConfig.CloudRegion,
			bootstrapConfig.Credential,
			bootstrapConfig.Cloud,
			cloudType,
		)
		if err != nil {
			return nil, errors.Trace(err)
		}
	} else {
		// The credential was auto-detected; run auto-detection again.
		cloudCredential, err := DetectCredential(
			bootstrapConfig.Cloud, cloudType,
		)
		if err != nil {
			return nil, errors.Trace(err)
		}
		// DetectCredential ensures that there is only one credential
		// to choose from. It's still in a map, though, hence for..range.
		for _, one := range cloudCredential.AuthCredentials {
			credential = &one
		}
	}

	// Add attributes from the controller details.
	controllerDetails, err := g.ControllerByName(controllerName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	bootstrapConfig.Config[config.CACertKey] = controllerDetails.CACert
	bootstrapConfig.Config[config.UUIDKey] = controllerDetails.ControllerUUID
	bootstrapConfig.Config[config.ControllerUUIDKey] = controllerDetails.ControllerUUID

	cfg, err := config.New(config.UseDefaults, bootstrapConfig.Config)
	if err != nil {
		return nil, errors.Trace(err)
	}
	provider, err := environs.Provider(cfg.Type())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return provider.BootstrapConfig(environs.BootstrapConfigParams{
		cfg, *credential,
		bootstrapConfig.CloudRegion,
		bootstrapConfig.CloudEndpoint,
		bootstrapConfig.CloudStorageEndpoint,
	})
}
