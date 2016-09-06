// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelcmd

import (
	"fmt"
	"net/http"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon-bakery.v1/httpbakery"

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
	authOpts    AuthOpts
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

// SetFlags implements cmd.Command.SetFlags.
func (c *JujuCommandBase) SetFlags(f *gnuflag.FlagSet) {
	c.authOpts.SetFlags(f)
}

// SetModelApi sets the api used to access model information.
func (c *JujuCommandBase) SetModelApi(api ModelAPI) {
	c.modelApi = api
}

// SetAPIOpen sets the function used for opening an API connection.
func (c *JujuCommandBase) SetAPIOpen(apiOpen api.OpenFunc) {
	c.apiOpenFunc = apiOpen
}

func (c *JujuCommandBase) modelAPI(store jujuclient.ClientStore, controllerName string) (ModelAPI, error) {
	if c.modelApi != nil {
		return c.modelApi, nil
	}
	conn, err := c.NewAPIRoot(store, controllerName, "")
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
	controllerName, modelName string,
) (api.Connection, error) {
	accountDetails, err := store.AccountDetails(controllerName)
	if err != nil && !errors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}
	// If there are no account details or there's no logged-in
	// user or the user is external, then trigger macaroon authentication
	// by using an empty AccountDetails.
	if accountDetails == nil || accountDetails.User == "" {
		accountDetails = &jujuclient.AccountDetails{}
	} else {
		u := names.NewUserTag(accountDetails.User)
		if !u.IsLocal() {
			accountDetails = &jujuclient.AccountDetails{}
		}
	}
	param, err := c.NewAPIConnectionParams(
		store, controllerName, modelName, accountDetails,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	conn, err := juju.NewAPIConnection(param)
	if modelName != "" && params.ErrCode(err) == params.CodeModelNotFound {
		return nil, c.missingModelError(store, controllerName, modelName)
	}
	return conn, err
}

func (c *JujuCommandBase) missingModelError(store jujuclient.ClientStore, controllerName, modelName string) error {
	// First, we'll try and clean up the missing model from the local cache.
	err := store.RemoveModel(controllerName, modelName)
	if err != nil {
		logger.Warningf("cannot remove unknown model from cache: %v", err)
	}
	currentModel, err := store.CurrentModel(controllerName)
	if err != nil {
		logger.Warningf("cannot read current model: %v", err)
	} else if currentModel == modelName {
		if err := store.SetCurrentModel(controllerName, ""); err != nil {
			logger.Warningf("cannot reset current model: %v", err)
		}
	}
	errorMessage := "model %q has been removed from the controller, run 'juju models' and switch to one of them."
	modelInfoMessage := "\nThere are %d accessible models on controller %q."
	models, err := store.AllModels(controllerName)
	if err == nil {
		modelInfoMessage = fmt.Sprintf(modelInfoMessage, len(models), controllerName)
	} else {
		modelInfoMessage = ""
	}
	return errors.Errorf(errorMessage+modelInfoMessage, modelName)
}

// NewAPIConnectionParams returns a juju.NewAPIConnectionParams with the
// given arguments such that a call to juju.NewAPIConnection with the
// result behaves the same as a call to JujuCommandBase.NewAPIRoot with
// the same arguments.
func (c *JujuCommandBase) NewAPIConnectionParams(
	store jujuclient.ClientStore,
	controllerName, modelName string,
	accountDetails *jujuclient.AccountDetails,
) (juju.NewAPIConnectionParams, error) {
	if err := c.initAPIContext(); err != nil {
		return juju.NewAPIConnectionParams{}, errors.Trace(err)
	}
	return newAPIConnectionParams(
		store, controllerName, modelName,
		accountDetails, c.apiContext.BakeryClient,
		c.apiOpen,
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
func (c *JujuCommandBase) RefreshModels(store jujuclient.ClientStore, controllerName string) error {
	modelManager, err := c.modelAPI(store, controllerName)
	if err != nil {
		return errors.Trace(err)
	}
	defer modelManager.Close()

	accountDetails, err := store.AccountDetails(controllerName)
	if err != nil {
		return errors.Trace(err)
	}

	models, err := modelManager.ListModels(accountDetails.User)
	if err != nil {
		return errors.Trace(err)
	}
	for _, model := range models {
		modelDetails := jujuclient.ModelDetails{model.UUID}
		owner := names.NewUserTag(model.Owner)
		modelName := jujuclient.JoinOwnerModelName(owner, model.Name)
		if err := store.UpdateModel(controllerName, modelName, modelDetails); err != nil {
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
	apiContext, err := NewAPIContext(c.cmdContext, &c.authOpts)
	if err != nil {
		return errors.Trace(err)
	}
	c.apiContext = apiContext
	return nil
}

// APIContext returns the API context used by the command.
// It should only be called while the Run method is being called.
//
// The returned APIContext should not be closed (it will be
// closed when the Run method completes).
func (c *JujuCommandBase) APIContext() (*APIContext, error) {
	if err := c.initAPIContext(); err != nil {
		return nil, errors.Trace(err)
	}
	return c.apiContext, nil
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
	modelName string,
	accountDetails *jujuclient.AccountDetails,
	bakery *httpbakery.Client,
	apiOpen api.OpenFunc,
) (juju.NewAPIConnectionParams, error) {
	if controllerName == "" {
		return juju.NewAPIConnectionParams{}, errors.Trace(errNoNameSpecified)
	}
	var modelUUID string
	if modelName != "" {
		modelDetails, err := store.ModelByName(controllerName, modelName)
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
		Store:          store,
		ControllerName: controllerName,
		AccountDetails: accountDetails,
		ModelUUID:      modelUUID,
		DialOpts:       dialOpts,
		OpenAPI:        openAPI,
	}, nil
}

// NewGetBootstrapConfigParamsFunc returns a function that, given a controller name,
// returns the params needed to bootstrap a fresh copy of that controller in the given client store.
func NewGetBootstrapConfigParamsFunc(store jujuclient.ClientStore) func(string) (*jujuclient.BootstrapConfig, *environs.PrepareConfigParams, error) {
	return bootstrapConfigGetter{store}.getBootstrapConfigParams
}

type bootstrapConfigGetter struct {
	jujuclient.ClientStore
}

func (g bootstrapConfigGetter) getBootstrapConfig(controllerName string) (*config.Config, error) {
	bootstrapConfig, params, err := g.getBootstrapConfigParams(controllerName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	provider, err := environs.Provider(bootstrapConfig.CloudType)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return provider.PrepareConfig(*params)
}

func (g bootstrapConfigGetter) getBootstrapConfigParams(controllerName string) (*jujuclient.BootstrapConfig, *environs.PrepareConfigParams, error) {
	if _, err := g.ClientStore.ControllerByName(controllerName); err != nil {
		return nil, nil, errors.Annotate(err, "resolving controller name")
	}
	bootstrapConfig, err := g.BootstrapConfigForController(controllerName)
	if err != nil {
		return nil, nil, errors.Annotate(err, "getting bootstrap config")
	}

	var credential *cloud.Credential
	if bootstrapConfig.Credential != "" {
		credential, _, _, err = GetCredentials(
			g.ClientStore,
			bootstrapConfig.CloudRegion,
			bootstrapConfig.Credential,
			bootstrapConfig.Cloud,
			bootstrapConfig.CloudType,
		)
		if err != nil {
			return nil, nil, errors.Trace(err)
		}
	} else {
		// The credential was auto-detected; run auto-detection again.
		cloudCredential, err := DetectCredential(
			bootstrapConfig.Cloud,
			bootstrapConfig.CloudType,
		)
		if err != nil {
			return nil, nil, errors.Trace(err)
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
		return nil, nil, errors.Trace(err)
	}

	// TODO(wallyworld) - remove after beta18
	controllerModelUUID := bootstrapConfig.ControllerModelUUID
	if controllerModelUUID == "" {
		controllerModelUUID = controllerDetails.ControllerUUID
	}

	bootstrapConfig.Config[config.UUIDKey] = controllerModelUUID
	cfg, err := config.New(config.NoDefaults, bootstrapConfig.Config)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	return bootstrapConfig, &environs.PrepareConfigParams{
		environs.CloudSpec{
			bootstrapConfig.CloudType,
			bootstrapConfig.Cloud,
			bootstrapConfig.CloudRegion,
			bootstrapConfig.CloudEndpoint,
			bootstrapConfig.CloudIdentityEndpoint,
			bootstrapConfig.CloudStorageEndpoint,
			credential,
		},
		cfg,
	}, nil
}
