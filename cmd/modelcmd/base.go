// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelcmd

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"golang.org/x/crypto/ssh/terminal"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/authentication"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/client/modelmanager"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/pki"
	k8sproxy "github.com/juju/juju/internal/provider/kubernetes/proxy"
	proxyerrors "github.com/juju/juju/internal/proxy/errors"
	"github.com/juju/juju/juju"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/rpc/params"
)

var errNoNameSpecified = errors.New("no name specified")
var errNotLogged = errors.New("not logged")

type modelMigratedError string

func newModelMigratedError(store jujuclient.ClientStore, modelName string, redirErr *api.RedirectError) error {
	// Check if this is a known controller
	allEndpoints := network.CollapseToHostPorts(redirErr.Servers).Strings()
	_, existingName, err := store.ControllerByAPIEndpoints(allEndpoints...)
	if err != nil && !errors.Is(err, errors.NotFound) {
		return err
	}

	if existingName != "" {
		mErr := fmt.Sprintf(`Model %q has been migrated to controller %q.
To access it run 'juju switch %s:%s'.`, modelName, existingName, existingName, modelName)

		return modelMigratedError(mErr)
	}

	// CACerts are always valid so no error checking is required here.
	fingerprint, _, err := pki.Fingerprint([]byte(redirErr.CACert))
	if err != nil {
		return err
	}

	ctrlAlias := "new-controller"
	if redirErr.ControllerAlias != "" {
		ctrlAlias = redirErr.ControllerAlias
	}

	var loginCmds []string
	for _, endpoint := range allEndpoints {
		loginCmds = append(loginCmds, fmt.Sprintf("  'juju login %s -c %s'", endpoint, ctrlAlias))
	}

	mErr := fmt.Sprintf(`Model %q has been migrated to another controller.
To access it run one of the following commands (you can replace the -c argument with your own preferred controller name):
%s

New controller fingerprint [%s]`, modelName, strings.Join(loginCmds, "\n"), fingerprint)

	return modelMigratedError(mErr)
}

func (e modelMigratedError) Error() string {
	return string(e)
}

// IsModelMigratedError returns true if err is of type modelMigratedError.
func IsModelMigratedError(err error) bool {
	_, ok := errors.Cause(err).(modelMigratedError)
	return ok
}

// Command extends cmd.Command with a closeContext method.
// It is implicitly implemented by any type that embeds CommandBase.
type Command interface {
	cmd.Command

	// SetAPIOpen sets the function used for opening an API connection.
	SetAPIOpen(opener api.OpenFunc)

	// SetModelAPI sets the api used to access model information.
	SetModelAPI(api ModelAPI)

	// SetEmbedded sets whether the command is being run inside a controller.
	SetEmbedded(bool)

	// closeAPIContexts closes any API contexts that have been opened.
	closeAPIContexts()
	initContexts(*cmd.Context)
	setRunStarted()
}

// ModelAPI provides access to the model client facade methods.
type ModelAPI interface {
	ListModels(ctx context.Context, user string) ([]base.UserModel, error)
	Close() error
}

// CommandBase is a convenience type for embedding that need
// an API connection.
type CommandBase struct {
	cmd.CommandBase
	FilesystemCommand
	cmdContext    *cmd.Context
	apiContexts   map[string]*apiContext
	modelAPI_     ModelAPI
	apiOpenFunc   api.OpenFunc
	authOpts      AuthOpts
	runStarted    bool
	refreshModels func(context.Context, jujuclient.ClientStore, string) error
	// sessionLoginFactory provides an session token based
	// login provider used to mock out oauth based login.
	sessionLoginFactory SessionLoginFactory

	// StdContext is the Go context.
	StdContext context.Context

	// CanClearCurrentModel indicates that this command can reset current model in local cache, aka client store.
	CanClearCurrentModel bool

	// Embedded is true if this command is being run inside a controller.
	Embedded bool
}

func (c *CommandBase) assertRunStarted() {
	if !c.runStarted {
		panic("inappropriate method called at init time")
	}
}

func (c *CommandBase) setRunStarted() {
	c.runStarted = true
}

// closeAPIContexts closes any API contexts that have
// been created.
//
//nolint:unused
func (c *CommandBase) closeAPIContexts() {
	for name, ctx := range c.apiContexts {
		if err := ctx.Close(); err != nil {
			logger.Errorf(context.TODO(), "%v", err)
		}
		delete(c.apiContexts, name)
	}
}

// SetEmbedded sets whether the command is embedded.
func (c *CommandBase) SetEmbedded(embedded bool) {
	c.Embedded = embedded
	if embedded {
		c.filesystem = restrictedFilesystem{}
	} else {
		c.filesystem = osFilesystem{}
	}
}

// SetFlags implements cmd.Command.SetFlags.
func (c *CommandBase) SetFlags(f *gnuflag.FlagSet) {
	c.authOpts.SetFlags(f)
}

// SetModelAPI sets the api used to access model information.
func (c *CommandBase) SetModelAPI(api ModelAPI) {
	c.modelAPI_ = api
}

// SetAPIOpen sets the function used for opening an API connection.
func (c *CommandBase) SetAPIOpen(apiOpen api.OpenFunc) {
	c.apiOpenFunc = apiOpen
}

// SetModelRefresh sets the function used for refreshing models.
func (c *CommandBase) SetModelRefresh(refresh func(context.Context, jujuclient.ClientStore, string) error) {
	c.refreshModels = refresh
}

func (c *CommandBase) SetSessionLoginFactory(loginFactory SessionLoginFactory) {
	c.sessionLoginFactory = loginFactory
}

func (c *CommandBase) modelAPI(ctx context.Context, store jujuclient.ClientStore, controllerName string) (ModelAPI, error) {
	c.assertRunStarted()
	if c.modelAPI_ != nil {
		return c.modelAPI_, nil
	}
	conn, err := c.NewAPIRoot(ctx, store, controllerName, "")
	if err != nil {
		return nil, errors.Trace(err)
	}
	c.modelAPI_ = modelmanager.NewClient(conn)
	return c.modelAPI_, nil
}

// NewAPIRoot returns a new connection to the API server for the given
// model or controller.
func (c *CommandBase) NewAPIRoot(
	ctx context.Context,
	store jujuclient.ClientStore,
	controllerName, modelName string,
) (api.Connection, error) {
	return c.NewAPIRootWithDialOpts(ctx, store, controllerName, modelName, nil, nil)
}

// NewAPIRootWithDialOpts returns a new connection to the API server for the
// given model or controller (the default dial options will be overridden if
// dialOpts is not nil).
func (c *CommandBase) NewAPIRootWithDialOpts(
	ctx context.Context,
	store jujuclient.ClientStore,
	controllerName, modelName string,
	addressOverride []string,
	dialOpts *api.DialOpts,
) (api.Connection, error) {
	c.assertRunStarted()
	accountDetails, err := store.AccountDetails(controllerName)
	if err != nil && !errors.Is(err, errors.NotFound) {
		return nil, errors.Trace(err)
	}

	param, err := c.NewAPIConnectionParams(
		store, controllerName, modelName, accountDetails,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	param.APIEndpoints = addressOverride
	if dialOpts != nil {
		param.DialOpts = *dialOpts
	}
	conn, err := juju.NewAPIConnection(ctx, param)
	if modelName != "" && params.ErrCode(err) == params.CodeModelNotFound {
		return nil, c.missingModelError(store, controllerName, modelName)
	}
	// Update the account details after each successful login.
	// Some login providers, for example, refresh a user's token.
	if err == nil && param.AccountDetails != nil {
		param.AccountDetails.LastKnownAccess = conn.ControllerAccess()
		err := store.UpdateAccount(controllerName, *param.AccountDetails)
		if err != nil {
			logger.Errorf(context.TODO(), "cannot update account information: %v", err)
		}
	}
	if redirErr, ok := errors.Cause(err).(*api.RedirectError); ok {
		return nil, newModelMigratedError(store, modelName, redirErr)
	}
	if juju.IsNoAddressesError(err) {
		return nil, errors.New("no controller API addresses; is bootstrap still in progress?")
	}
	if proxyerrors.IsProxyConnectError(err) {
		logger.Debugf(context.TODO(), "proxy connection error: %v", err)
		if proxyerrors.ProxyType(err) == k8sproxy.ProxierTypeKey {
			return nil, errors.Annotate(err, "cannot connect to k8s api server; try running 'juju update-k8s --client <k8s cloud name>'")
		}
		return nil, errors.Annotate(err, "cannot connect to api server proxy")
	}
	return conn, errors.Trace(err)
}

// RemoveModelFromClientStore removes given model from client cache, store,
// for a given controller.
// If this model has also been cached as current, it will be reset if
// the requesting command can modify current model.
// For example, commands such as add/destroy-model, login/register, etc.
// If the model was cached as current but the command is not expected to
// change current model, this call will still remove model details from the client cache
// but will keep current model name intact to allow subsequent calls to try to resolve
// model details on the controller.
func (c *CommandBase) RemoveModelFromClientStore(store jujuclient.ClientStore, controllerName, modelName string) {
	err := store.RemoveModel(controllerName, modelName)
	if err != nil && !errors.Is(err, errors.NotFound) {
		logger.Warningf(context.TODO(), "cannot remove unknown model from cache: %v", err)
	}
	if c.CanClearCurrentModel {
		currentModel, err := store.CurrentModel(controllerName)
		if err != nil {
			logger.Warningf(context.TODO(), "cannot read current model: %v", err)
		} else if currentModel == modelName {
			if err := store.SetCurrentModel(controllerName, ""); err != nil {
				logger.Warningf(context.TODO(), "cannot reset current model: %v", err)
			}
		}
	}
}

func (c *CommandBase) missingModelError(store jujuclient.ClientStore, controllerName, modelName string) error {
	// First, we'll try and clean up the missing model from the local cache.
	c.RemoveModelFromClientStore(store, controllerName, modelName)
	return errors.Errorf("model %q has been removed from the controller, run 'juju models' and switch to one of them.", modelName)
}

// NewAPIConnectionParams returns a juju.NewAPIConnectionParams with the
// given arguments such that a call to juju.NewAPIConnection with the
// result behaves the same as a call to CommandBase.NewAPIRoot with
// the same arguments.
func (c *CommandBase) NewAPIConnectionParams(
	store jujuclient.ClientStore,
	controllerName, modelName string,
	accountDetails *jujuclient.AccountDetails,
) (juju.NewAPIConnectionParams, error) {
	c.assertRunStarted()
	bakeryClient, err := c.BakeryClient(store, controllerName)
	if err != nil {
		return juju.NewAPIConnectionParams{}, errors.Trace(err)
	}
	var getPassword func(username string) (string, error)
	var cmdOut io.Writer
	if c.cmdContext != nil {
		getPassword = func(username string) (string, error) {
			fmt.Fprintf(c.cmdContext.Stderr, "please enter password for %s on %s: ", username, controllerName)
			defer fmt.Fprintln(c.cmdContext.Stderr)
			return readPassword(c.cmdContext.Stdin)
		}
		cmdOut = c.cmdContext.Stderr
	} else {
		getPassword = func(username string) (string, error) {
			return "", errors.New("no context to prompt for password")
		}
	}

	return newAPIConnectionParams(
		store, controllerName, modelName,
		accountDetails,
		c.Embedded,
		bakeryClient,
		c.apiOpen,
		getPassword,
		cmdOut,
		c.sessionTokenLoginFactory(),
	)
}

// HTTPClient returns an http.Client that contains the loaded
// persistent cookie jar. Note that this client is not good for
// connecting to the Juju API itself because it does not
// have the correct TLS setup - use api.Connection.HTTPClient
// for that.
func (c *CommandBase) HTTPClient(store jujuclient.ClientStore, controllerName string) (*http.Client, error) {
	c.assertRunStarted()
	bakeryClient, err := c.BakeryClient(store, controllerName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return bakeryClient.Client, nil
}

// BakeryClient returns a macaroon bakery client that
// uses the same HTTP client returned by HTTPClient.
func (c *CommandBase) BakeryClient(store jujuclient.CookieStore, controllerName string) (*httpbakery.Client, error) {
	c.assertRunStarted()
	ctx, err := c.getAPIContext(store, controllerName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return ctx.NewBakeryClient(), nil
}

// APIOpen establishes a connection to the API server using the
// the given api.Info and api.DialOpts, and associating any stored
// authorization tokens with the given controller name.
func (c *CommandBase) APIOpen(ctx context.Context, info *api.Info, opts api.DialOpts) (api.Connection, error) {
	c.assertRunStarted()
	return c.apiOpen(ctx, info, opts)
}

// apiOpen establishes a connection to the API server using the
// the give api.Info and api.DialOpts.
func (c *CommandBase) apiOpen(ctx context.Context, info *api.Info, opts api.DialOpts) (api.Connection, error) {
	if c.apiOpenFunc != nil {
		return c.apiOpenFunc(ctx, info, opts)
	}
	return api.Open(ctx, info, opts)
}

// sessionTokenLoginFactory returns a session token based login
// object that is used for tests to enable mocking out OAuth login flows.
func (c *CommandBase) SessionTokenLoginFactory() SessionLoginFactory {
	c.assertRunStarted()
	return c.sessionTokenLoginFactory()
}

func (c *CommandBase) sessionTokenLoginFactory() SessionLoginFactory {
	if c.sessionLoginFactory != nil {
		return c.sessionLoginFactory
	}
	return api.SessionTokenLoginFactory{}
}

// RefreshModels refreshes the local models cache for the current user
// on the specified controller.
func (c *CommandBase) RefreshModels(ctx context.Context, store jujuclient.ClientStore, controllerName string) error {
	if c.refreshModels == nil {
		return c.doRefreshModels(ctx, store, controllerName)
	}
	return c.refreshModels(ctx, store, controllerName)
}

func (c *CommandBase) doRefreshModels(ctx context.Context, store jujuclient.ClientStore, controllerName string) error {
	c.assertRunStarted()
	modelManager, err := c.modelAPI(ctx, store, controllerName)
	if err != nil {
		return errors.Trace(err)
	}
	defer func() { _ = modelManager.Close() }()

	accountDetails, err := store.AccountDetails(controllerName)
	if err != nil {
		return errors.Trace(err)
	}

	models, err := modelManager.ListModels(ctx, accountDetails.User)
	if err != nil {
		return errors.Trace(err)
	}
	if err := c.SetControllerModels(store, controllerName, models); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (c *CommandBase) SetControllerModels(store jujuclient.ClientStore, controllerName string, models []base.UserModel) error {
	modelsToStore := make(map[string]jujuclient.ModelDetails, len(models))
	for _, model := range models {
		modelDetails := jujuclient.ModelDetails{ModelUUID: model.UUID, ModelType: model.Type}
		modelName := jujuclient.QualifyModelName(model.Qualifier.String(), model.Name)
		modelsToStore[modelName] = modelDetails
	}
	if err := store.SetModels(controllerName, modelsToStore); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// ModelUUIDs returns the model UUIDs for the given model names.
func (c *CommandBase) ModelUUIDs(ctx context.Context, store jujuclient.ClientStore, controllerName string, modelNames []string) ([]string, error) {
	var result []string
	for _, modelName := range modelNames {
		model, err := store.ModelByName(controllerName, modelName)
		if errors.Is(err, errors.NotFound) {
			// The model isn't known locally, so query the models available in the controller.
			logger.Infof(context.TODO(), "model %q not cached locally, refreshing models from controller", modelName)
			if err := c.RefreshModels(ctx, store, controllerName); err != nil {
				return nil, errors.Annotatef(err, "refreshing model %q", modelName)
			}
			model, err = store.ModelByName(controllerName, modelName)
		}
		if err != nil {
			return nil, errors.Trace(err)
		}
		result = append(result, model.ModelUUID)
	}
	return result, nil
}

// ControllerUUID returns the controller UUID for specified controller name.
func (c *CommandBase) ControllerUUID(store jujuclient.ClientStore, controllerName string) (string, error) {
	ctrl, err := store.ControllerByName(controllerName)
	if err != nil {
		return "", errors.Annotate(err, "resolving controller name")
	}
	return ctrl.ControllerUUID, nil
}

// getAPIContext returns an apiContext for the given controller.
// It will return the same context if called twice for the same controller.
// The context will be closed when closeAPIContexts is called.
func (c *CommandBase) getAPIContext(store jujuclient.CookieStore, controllerName string) (*apiContext, error) {
	c.assertRunStarted()
	if ctx := c.apiContexts[controllerName]; ctx != nil {
		return ctx, nil
	}
	if controllerName == "" {
		return nil, errors.New("cannot get API context from empty controller name")
	}
	c.authOpts.Embedded = c.Embedded
	ctx, err := newAPIContext(c.cmdContext, &c.authOpts, store, controllerName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	c.apiContexts[controllerName] = ctx
	return ctx, nil
}

// CookieJar returns the cookie jar that is used to store auth credentials
// when connecting to the API.
func (c *CommandBase) CookieJar(store jujuclient.CookieStore, controllerName string) (http.CookieJar, error) {
	ctx, err := c.getAPIContext(store, controllerName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return ctx.CookieJar(), nil
}

// ClearControllerMacaroons will remove all macaroons stored
// for the given controller from the persistent cookie jar.
// This is called both from 'juju logout' and a failed 'juju register'.
func (c *CommandBase) ClearControllerMacaroons(store jujuclient.CookieStore, controllerName string) error {
	ctx, err := c.getAPIContext(store, controllerName)
	if err != nil {
		return errors.Trace(err)
	}
	ctx.jar.RemoveAll()
	return nil
}

func (c *CommandBase) initContexts(ctx *cmd.Context) {
	c.StdContext = context.Background()
	c.cmdContext = ctx
	c.apiContexts = make(map[string]*apiContext)
}

// WrapBase wraps the specified Command. This should be
// used by any command that embeds CommandBase.
func WrapBase(c Command) Command {
	return &baseCommandWrapper{
		Command: c,
	}
}

type baseCommandWrapper struct {
	Command
}

// inner implements wrapper.inner.
func (w *baseCommandWrapper) inner() cmd.Command {
	return w.Command
}

type hasClientStore interface {
	SetClientStore(store jujuclient.ClientStore)
}

// SetClientStore sets the client store to use.
func (w *baseCommandWrapper) SetClientStore(store jujuclient.ClientStore) {
	if csc, ok := w.Command.(hasClientStore); ok {
		csc.SetClientStore(store)
	}
}

// SetEmbedded implements the ModelCommand interface.
func (c *baseCommandWrapper) SetEmbedded(embedded bool) {
	c.Command.SetEmbedded(embedded)
}

// Run implements Command.Run.
func (w *baseCommandWrapper) Run(ctx *cmd.Context) error {
	defer w.closeAPIContexts()
	w.initContexts(ctx)
	w.setRunStarted()
	return w.Command.Run(ctx)
}

type SessionLoginFactory interface {
	NewLoginProvider(token string, output io.Writer, tokenCallback func(token string)) api.LoginProvider
}

func newAPIConnectionParams(
	store jujuclient.ClientStore,
	controllerName,
	modelName string,
	accountDetails *jujuclient.AccountDetails,
	embedded bool,
	bakery *httpbakery.Client,
	apiOpen api.OpenFunc,
	getPassword func(string) (string, error),
	cmdOut io.Writer,
	sessionLoginFactory SessionLoginFactory,
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

	if accountDetails == nil {
		return juju.NewAPIConnectionParams{}, errors.Annotatef(errNotLogged, "controller %q", controllerName)
	}

	controllerDetails, err := store.ControllerByName(controllerName)
	if err != nil {
		return juju.NewAPIConnectionParams{}, errors.Annotatef(err, "getting controller %q", controllerName)
	}

	if controllerDetails.OIDCLogin {
		dialOpts.LoginProvider = sessionLoginFactory.NewLoginProvider(
			accountDetails.SessionToken,
			cmdOut,
			func(sessionToken string) {
				accountDetails.SessionToken = sessionToken
			},
		)
	}

	// Embedded clients with macaroons cannot discharge.
	if accountDetails != nil && !embedded {
		bakery.InteractionMethods = []httpbakery.Interactor{
			authentication.NewInteractor(accountDetails.User, getPassword),
			httpbakery.WebBrowserInteractor{},
		}
	}

	return juju.NewAPIConnectionParams{
		ControllerStore: store,
		ControllerName:  controllerName,
		AccountDetails:  accountDetails,
		ModelUUID:       modelUUID,
		DialOpts:        dialOpts,
		OpenAPI:         OpenAPIFuncWithMacaroons(apiOpen, store, controllerName),
	}, nil
}

// OpenAPIFuncWithMacaroons is a middleware to ensure that we have a set of
// macaroons for a given open request.
func OpenAPIFuncWithMacaroons(apiOpen api.OpenFunc, store jujuclient.ClientStore, controllerName string) api.OpenFunc {
	return func(ctx context.Context, info *api.Info, dialOpts api.DialOpts) (api.Connection, error) {
		// When attempting to connect to the non websocket fronted HTTPS
		// endpoints, we need to ensure that we have a series of macaroons
		// correctly set if there isn't a password.
		if info != nil && info.Password == "" && len(info.Macaroons) == 0 {
			cookieJar, err := store.CookieJar(controllerName)
			if err != nil {
				return nil, errors.Trace(err)
			}

			cookieURL := api.CookieURLFromHost(api.PreferredHost(info))
			info.Macaroons = httpbakery.MacaroonsForURL(cookieJar, cookieURL)
		}

		return apiOpen(ctx, info, dialOpts)
	}
}

// NewGetBootstrapConfigParamsFunc returns a function that, given a controller name,
// returns the params needed to bootstrap a fresh copy of that controller in the given client store.
func NewGetBootstrapConfigParamsFunc(
	ctx *cmd.Context,
	store jujuclient.ClientStore,
	providerRegistry environs.ProviderRegistry,
) func(string) (*jujuclient.BootstrapConfig, *environscloudspec.CloudSpec, *config.Config, error) {
	return bootstrapConfigGetter{ctx, store, providerRegistry}.getBootstrapConfigParams
}

type bootstrapConfigGetter struct {
	ctx      *cmd.Context
	store    jujuclient.ClientStore
	registry environs.ProviderRegistry
}

func (g bootstrapConfigGetter) getBootstrapConfigParams(controllerName string) (*jujuclient.BootstrapConfig, *environscloudspec.CloudSpec, *config.Config, error) {
	controllerDetails, err := g.store.ControllerByName(controllerName)
	if err != nil {
		return nil, nil, nil, errors.Annotate(err, "resolving controller name")
	}
	bootstrapConfig, err := g.store.BootstrapConfigForController(controllerName)
	if err != nil {
		return nil, nil, nil, errors.Annotate(err, "getting bootstrap config")
	}

	var credential *cloud.Credential
	bootstrapCloud := cloud.Cloud{
		Name:             bootstrapConfig.Cloud,
		Type:             bootstrapConfig.CloudType,
		Endpoint:         bootstrapConfig.CloudEndpoint,
		IdentityEndpoint: bootstrapConfig.CloudIdentityEndpoint,
	}
	if bootstrapConfig.Credential != "" {
		if bootstrapConfig.CloudRegion != "" {
			bootstrapCloud.Regions = []cloud.Region{{
				Name:             bootstrapConfig.CloudRegion,
				Endpoint:         bootstrapConfig.CloudEndpoint,
				IdentityEndpoint: bootstrapConfig.CloudIdentityEndpoint,
			}}
		}
		credential, _, _, err = GetCredentials(
			g.ctx, g.store,
			GetCredentialsParams{
				Cloud:          bootstrapCloud,
				CloudRegion:    bootstrapConfig.CloudRegion,
				CredentialName: bootstrapConfig.Credential,
			},
		)
		if err != nil {
			return nil, nil, nil, errors.Trace(err)
		}
	} else {
		// The credential was auto-detected; run auto-detection again.
		provider, err := g.registry.Provider(bootstrapConfig.CloudType)
		if err != nil {
			return nil, nil, nil, errors.Trace(err)
		}
		cloudCredential, err := DetectCredential(bootstrapConfig.Cloud, provider)
		if err != nil {
			return nil, nil, nil, errors.Trace(err)
		}
		// DetectCredential ensures that there is only one credential
		// to choose from. It's still in a map, though, hence for..range.
		var credentialName string
		for name, v := range cloudCredential.AuthCredentials {
			one := v
			credential = &one
			credentialName = name
			break
		}
		credential, err = FinalizeFileContent(credential, provider)
		if err != nil {
			return nil, nil, nil, AnnotateWithFinalizationError(err, credentialName, bootstrapCloud.Name)
		}
		credential, err = provider.FinalizeCredential(
			g.ctx, environs.FinalizeCredentialParams{
				Credential:            *credential,
				CloudName:             bootstrapConfig.Cloud,
				CloudEndpoint:         bootstrapConfig.CloudEndpoint,
				CloudStorageEndpoint:  bootstrapConfig.CloudStorageEndpoint,
				CloudIdentityEndpoint: bootstrapConfig.CloudIdentityEndpoint,
			},
		)
		if err != nil {
			return nil, nil, nil, errors.Trace(err)
		}
	}

	// Add attributes from the controller details.
	bootstrapConfig.Config[config.UUIDKey] = bootstrapConfig.ControllerModelUUID
	cfg, err := config.New(config.NoDefaults, bootstrapConfig.Config)
	if err != nil {
		return nil, nil, nil, errors.Trace(err)
	}
	return bootstrapConfig, &environscloudspec.CloudSpec{
			Type:              bootstrapConfig.CloudType,
			Name:              bootstrapConfig.Cloud,
			Region:            bootstrapConfig.CloudRegion,
			Endpoint:          bootstrapConfig.CloudEndpoint,
			IdentityEndpoint:  bootstrapConfig.CloudIdentityEndpoint,
			StorageEndpoint:   bootstrapConfig.CloudStorageEndpoint,
			Credential:        credential,
			CACertificates:    bootstrapConfig.CloudCACertificates,
			SkipTLSVerify:     bootstrapConfig.SkipTLSVerify,
			IsControllerCloud: bootstrapConfig.Cloud == controllerDetails.Cloud,
		},
		cfg, nil
}

// TODO(axw) this is now in three places: change-password,
// register, and here. Refactor and move to a common location.
func readPassword(stdin io.Reader) (string, error) {
	if f, ok := stdin.(*os.File); ok && terminal.IsTerminal(int(f.Fd())) {
		password, err := terminal.ReadPassword(int(f.Fd()))
		return string(password), err
	}
	return readLine(stdin)
}

func readLine(stdin io.Reader) (string, error) {
	// Read one byte at a time to avoid reading beyond the delimiter.
	line, err := bufio.NewReader(byteAtATimeReader{stdin}).ReadString('\n')
	if err != nil {
		return "", errors.Trace(err)
	}
	return line[:len(line)-1], nil
}

type byteAtATimeReader struct {
	io.Reader
}

// Read is part of the io.Reader interface.
func (r byteAtATimeReader) Read(out []byte) (int, error) {
	return r.Reader.Read(out[:1])
}

// wrapper is implemented by types that wrap a command.
type wrapper interface {
	inner() cmd.Command
}

// InnerCommand returns the command that has been wrapped
// by one of the Wrap functions. This is useful for
// tests that wish to inspect internal details of a command
// instance. If c isn't wrapping anything, it returns c.
func InnerCommand(c cmd.Command) cmd.Command {
	for {
		c1, ok := c.(wrapper)
		if !ok {
			return c
		}
		c = c1.inner()
	}
}
