// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelcmd

import (
	"net/http"
	"net/url"
	"os"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/persistent-cookiejar"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/modelmanager"
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
	setVisitWebPage(func(*url.URL) error)
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
	apiContext   *apiContext
	modelApi     ModelAPI
	visitWebPage func(*url.URL) error
}

// closeContext closes the command's API context
// if it has actually been created.
func (c *JujuCommandBase) closeContext() {
	if c.apiContext != nil {
		if err := c.apiContext.close(); err != nil {
			logger.Errorf("%v", err)
		}
	}
}

// SetModelApi sets the api used to access model information.
func (c *JujuCommandBase) SetModelApi(api ModelAPI) {
	c.modelApi = api
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
	return c.apiContext.newAPIConnectionParams(store, controllerName, accountName, modelName)
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
	return c.apiContext.httpClient(), nil
}

// APIOpen establishes a connection to the API server using the
// the give api.Info and api.DialOpts.
func (c *JujuCommandBase) APIOpen(info *api.Info, opts api.DialOpts) (api.Connection, error) {
	if err := c.initAPIContext(); err != nil {
		return nil, errors.Trace(err)
	}
	return c.apiContext.apiOpen(info, opts)
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
	if c.visitWebPage == nil {
		c.visitWebPage = httpbakery.OpenWebBrowser
	}
	ctxt, err := newAPIContext(c.visitWebPage)
	if err != nil {
		return errors.Trace(err)
	}
	c.apiContext = ctxt
	return nil
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
	*apiContext
}

// Run implements Command.Run.
func (w *baseCommandWrapper) Run(ctx *cmd.Context) error {
	defer w.closeContext()
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

// cookieFile returns the path to the cookie used to store authorization
// macaroons. The returned value can be overridden by setting the
// JUJU_COOKIEFILE or GO_COOKIEFILE environment variables.
func cookieFile() string {
	if file := os.Getenv("JUJU_COOKIEFILE"); file != "" {
		return file
	}
	return cookiejar.DefaultCookieFile()
}

func (c *JujuCommandBase) setVisitWebPage(f func(*url.URL) error) {
	c.visitWebPage = f
}

// newAPIContext returns a new api context, which should be closed
// when done with.
func newAPIContext(f func(*url.URL) error) (*apiContext, error) {
	jar, err := cookiejar.New(&cookiejar.Options{
		Filename: cookieFile(),
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	client := httpbakery.NewClient()
	client.Jar = jar
	client.VisitWebPage = f

	return &apiContext{
		jar:    jar,
		client: client,
	}, nil
}

// apiContext is a convenience type that can be embedded wherever
// we need an API connection.
// It also stores a bakery bakery client allowing the API
// to be used using macaroons to authenticate. It stores
// obtained macaroons and discharges in a cookie jar file.
type apiContext struct {
	jar    *cookiejar.Jar
	client *httpbakery.Client
}

// Close saves the embedded cookie jar.
func (c *apiContext) close() error {
	if err := c.jar.Save(); err != nil {
		return errors.Annotatef(err, "cannot save cookie jar")
	}
	return nil
}

// apiOpen establishes a connection to the API server using the
// the give api.Info and api.DialOpts.
func (ctx *apiContext) apiOpen(info *api.Info, opts api.DialOpts) (api.Connection, error) {
	return api.Open(info, opts)
}

func (ctx *apiContext) newAPIConnectionParams(
	store jujuclient.ClientStore,
	controllerName,
	accountName,
	modelName string,
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
	dialOpts.BakeryClient = ctx.client
	return juju.NewAPIConnectionParams{
		Store:           store,
		ControllerName:  controllerName,
		BootstrapConfig: NewGetBootstrapConfigFunc(store),
		AccountDetails:  accountDetails,
		ModelUUID:       modelUUID,
		DialOpts:        dialOpts,
	}, nil
}

// httpClient returns an http.Client that contains the loaded
// persistent cookie jar.
func (ctx *apiContext) httpClient() *http.Client {
	return ctx.client.Client
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
	controllerName, err := ResolveControllerName(g.ClientStore, controllerName)
	if err != nil {
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
