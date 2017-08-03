// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelcmd

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"golang.org/x/crypto/ssh/terminal"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon-bakery.v1/httpbakery"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/authentication"
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

// Command extends cmd.Command with a closeContext method.
// It is implicitly implemented by any type that embeds CommandBase.
type Command interface {
	cmd.Command

	// SetAPIOpen sets the function used for opening an API connection.
	SetAPIOpen(opener api.OpenFunc)

	// SetModelAPI sets the api used to access model information.
	SetModelAPI(api ModelAPI)

	// closeAPIContexts closes any API contexts that have been opened.
	closeAPIContexts()
	initContexts(*cmd.Context)
	setRunStarted()
}

// ModelAPI provides access to the model client facade methods.
type ModelAPI interface {
	ListModels(user string) ([]base.UserModel, error)
	Close() error
}

// CommandBase is a convenience type for embedding that need
// an API connection.
type CommandBase struct {
	cmd.CommandBase
	cmdContext    *cmd.Context
	apiContexts   map[string]*apiContext
	modelAPI_     ModelAPI
	apiOpenFunc   api.OpenFunc
	authOpts      AuthOpts
	runStarted    bool
	refreshModels func(jujuclient.ClientStore, string) error
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
func (c *CommandBase) closeAPIContexts() {
	for name, ctx := range c.apiContexts {
		if err := ctx.Close(); err != nil {
			logger.Errorf("%v", err)
		}
		delete(c.apiContexts, name)
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
func (c *CommandBase) SetModelRefresh(refresh func(jujuclient.ClientStore, string) error) {
	c.refreshModels = refresh
}

func (c *CommandBase) modelAPI(store jujuclient.ClientStore, controllerName string) (ModelAPI, error) {
	c.assertRunStarted()
	if c.modelAPI_ != nil {
		return c.modelAPI_, nil
	}
	conn, err := c.NewAPIRoot(store, controllerName, "")
	if err != nil {
		return nil, errors.Trace(err)
	}
	c.modelAPI_ = modelmanager.NewClient(conn)
	return c.modelAPI_, nil
}

// NewAPIRoot returns a new connection to the API server for the given
// model or controller.
func (c *CommandBase) NewAPIRoot(
	store jujuclient.ClientStore,
	controllerName, modelName string,
) (api.Connection, error) {
	c.assertRunStarted()
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

func (c *CommandBase) missingModelError(store jujuclient.ClientStore, controllerName, modelName string) error {
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
	if c.cmdContext != nil {
		getPassword = func(username string) (string, error) {
			fmt.Fprintf(c.cmdContext.Stderr, "please enter password for %s on %s: ", username, controllerName)
			defer fmt.Fprintln(c.cmdContext.Stderr)
			return readPassword(c.cmdContext.Stdin)
		}
	} else {
		getPassword = func(username string) (string, error) {
			return "", errors.New("no context to prompt for password")
		}
	}

	return newAPIConnectionParams(
		store, controllerName, modelName,
		accountDetails,
		bakeryClient,
		c.apiOpen,
		getPassword,
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
func (c *CommandBase) APIOpen(info *api.Info, opts api.DialOpts) (api.Connection, error) {
	c.assertRunStarted()
	return c.apiOpen(info, opts)
}

// apiOpen establishes a connection to the API server using the
// the give api.Info and api.DialOpts.
func (c *CommandBase) apiOpen(info *api.Info, opts api.DialOpts) (api.Connection, error) {
	if c.apiOpenFunc != nil {
		return c.apiOpenFunc(info, opts)
	}
	return api.Open(info, opts)
}

// RefreshModels refreshes the local models cache for the current user
// on the specified controller.
func (c *CommandBase) RefreshModels(store jujuclient.ClientStore, controllerName string) error {
	if c.refreshModels == nil {
		return c.doRefreshModels(store, controllerName)
	}
	return c.refreshModels(store, controllerName)
}

func (c *CommandBase) doRefreshModels(store jujuclient.ClientStore, controllerName string) error {
	c.assertRunStarted()
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

// ModelUUIDs returns the model UUIDs for the given model names.
func (c *CommandBase) ModelUUIDs(store jujuclient.ClientStore, controllerName string, modelNames []string) ([]string, error) {
	var result []string
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
			return nil, errors.Trace(err)
		}
		result = append(result, model.ModelUUID)
	}
	return result, nil
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

// Run implements Command.Run.
func (w *baseCommandWrapper) Run(ctx *cmd.Context) error {
	defer w.closeAPIContexts()
	w.initContexts(ctx)
	w.setRunStarted()
	return w.Command.Run(ctx)
}

func newAPIConnectionParams(
	store jujuclient.ClientStore,
	controllerName,
	modelName string,
	accountDetails *jujuclient.AccountDetails,
	bakery *httpbakery.Client,
	apiOpen api.OpenFunc,
	getPassword func(string) (string, error),
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

	if accountDetails != nil {
		bakery.WebPageVisitor = httpbakery.NewMultiVisitor(
			authentication.NewVisitor(accountDetails.User, getPassword),
			bakery.WebPageVisitor,
		)
	}

	return juju.NewAPIConnectionParams{
		Store:          store,
		ControllerName: controllerName,
		AccountDetails: accountDetails,
		ModelUUID:      modelUUID,
		DialOpts:       dialOpts,
		OpenAPI:        apiOpen,
	}, nil
}

// NewGetBootstrapConfigParamsFunc returns a function that, given a controller name,
// returns the params needed to bootstrap a fresh copy of that controller in the given client store.
func NewGetBootstrapConfigParamsFunc(
	ctx *cmd.Context,
	store jujuclient.ClientStore,
	providerRegistry environs.ProviderRegistry,
) func(string) (*jujuclient.BootstrapConfig, *environs.PrepareConfigParams, error) {
	return bootstrapConfigGetter{ctx, store, providerRegistry}.getBootstrapConfigParams
}

type bootstrapConfigGetter struct {
	ctx      *cmd.Context
	store    jujuclient.ClientStore
	registry environs.ProviderRegistry
}

func (g bootstrapConfigGetter) getBootstrapConfigParams(controllerName string) (*jujuclient.BootstrapConfig, *environs.PrepareConfigParams, error) {
	controllerDetails, err := g.store.ControllerByName(controllerName)
	if err != nil {
		return nil, nil, errors.Annotate(err, "resolving controller name")
	}
	bootstrapConfig, err := g.store.BootstrapConfigForController(controllerName)
	if err != nil {
		return nil, nil, errors.Annotate(err, "getting bootstrap config")
	}

	var credential *cloud.Credential
	if bootstrapConfig.Credential != "" {
		bootstrapCloud := cloud.Cloud{
			Name:             bootstrapConfig.Cloud,
			Type:             bootstrapConfig.CloudType,
			Endpoint:         bootstrapConfig.CloudEndpoint,
			IdentityEndpoint: bootstrapConfig.CloudIdentityEndpoint,
		}
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
			return nil, nil, errors.Trace(err)
		}
	} else {
		// The credential was auto-detected; run auto-detection again.
		provider, err := g.registry.Provider(bootstrapConfig.CloudType)
		if err != nil {
			return nil, nil, errors.Trace(err)
		}
		cloudCredential, err := DetectCredential(bootstrapConfig.Cloud, provider)
		if err != nil {
			return nil, nil, errors.Trace(err)
		}
		// DetectCredential ensures that there is only one credential
		// to choose from. It's still in a map, though, hence for..range.
		for _, one := range cloudCredential.AuthCredentials {
			credential = &one
		}
		credential, err = provider.FinalizeCredential(
			g.ctx, environs.FinalizeCredentialParams{
				Credential:            *credential,
				CloudEndpoint:         bootstrapConfig.CloudEndpoint,
				CloudStorageEndpoint:  bootstrapConfig.CloudStorageEndpoint,
				CloudIdentityEndpoint: bootstrapConfig.CloudIdentityEndpoint,
			},
		)
		if err != nil {
			return nil, nil, errors.Trace(err)
		}
	}

	// Add attributes from the controller details.

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
