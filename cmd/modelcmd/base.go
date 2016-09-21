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
	bakeryClient, err := c.BakeryClient()
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
func (c *JujuCommandBase) HTTPClient() (*http.Client, error) {
	bakeryClient, err := c.BakeryClient()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return bakeryClient.Client, nil
}

// BakeryClient returns a macaroon bakery client that
// uses the same HTTP client returned by HTTPClient.
func (c *JujuCommandBase) BakeryClient() (*httpbakery.Client, error) {
	if err := c.initAPIContext(); err != nil {
		return nil, errors.Trace(err)
	}
	return c.apiContext.NewBakeryClient(), nil
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
func NewGetBootstrapConfigParamsFunc(ctx *cmd.Context, store jujuclient.ClientStore) func(string) (*jujuclient.BootstrapConfig, *environs.PrepareConfigParams, error) {
	return bootstrapConfigGetter{ctx, store}.getBootstrapConfigParams
}

type bootstrapConfigGetter struct {
	ctx   *cmd.Context
	store jujuclient.ClientStore
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
	if _, err := g.store.ControllerByName(controllerName); err != nil {
		return nil, nil, errors.Annotate(err, "resolving controller name")
	}
	bootstrapConfig, err := g.store.BootstrapConfigForController(controllerName)
	if err != nil {
		return nil, nil, errors.Annotate(err, "getting bootstrap config")
	}

	var credential *cloud.Credential
	if bootstrapConfig.Credential != "" {
		bootstrapCloud := cloud.Cloud{
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
				CloudName:      bootstrapConfig.Cloud,
				CloudRegion:    bootstrapConfig.CloudRegion,
				CredentialName: bootstrapConfig.Credential,
			},
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
	controllerDetails, err := g.store.ControllerByName(controllerName)
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
