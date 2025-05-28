// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"text/template"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v6"
	"golang.org/x/crypto/nacl/secretbox"
	"golang.org/x/crypto/ssh/terminal"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/client/modelmanager"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/internal/loginprovider"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/internal/cmd"
	jujuhttp "github.com/juju/juju/internal/http"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/proxy/factory"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/rpc/params"
)

var noModelsMessage = `
There are no models available. You can add models with
"juju add-model", or you can ask an administrator or owner
of a model to grant access to that model with "juju grant".
`

// NewRegisterCommand returns a command to allow the user to register a controller.
func NewRegisterCommand() cmd.Command {
	c := &registerCommand{}
	c.apiOpen = c.APIOpen
	c.listModelsFunc = c.listModels
	c.store = jujuclient.NewFileClientStore()
	c.CanClearCurrentModel = true
	return modelcmd.WrapBase(c)
}

// registerCommand logs in to a Juju controller and caches the connection
// information.
type registerCommand struct {
	modelcmd.CommandBase
	apiOpen        api.OpenFunc
	listModelsFunc func(_ context.Context, _ jujuclient.ClientStore, controller, user string) ([]base.UserModel, error)
	store          jujuclient.ClientStore

	arg     string
	replace bool

	// onRunError is executed if non-nil and there is an error at the end
	// of the Run method.
	onRunError func()
}

var usageRegisterSummary = `
Registers a controller.`[1:]

var usageRegisterDetails = `
The register command adds details of a controller to the local system.
This is done either by completing the user registration process that
began with the 'juju add-user' command, or by providing the DNS host
name of a public controller.

To complete the user registration process, you should have been provided
with a base64-encoded blob of data (the output of 'juju add-user')
which can be copied and pasted as the <string> argument to 'register'.
You will be prompted for a password, which, once set, causes the
registration string to be voided. In order to start using Juju the user
can now either add a model or wait for a model to be shared with them.
Some machine providers will require the user to be in possession of
certain credentials in order to add a model.

If a new controller has been spun up to replace an existing one, and you want 
to start using that replacement controller instead of the original one,
use the --replace option to overwrite any existing controller details based
on either a name or UUID match.

When adding a controller at a public address, authentication via some
external third party (for example Ubuntu SSO) will be required, usually
by using a web browser.

`

const usageRegisterExamples = `
    juju register MFATA3JvZDAnExMxMDQuMTU0LjQyLjQ0OjE3MDcwExAxMC4xMjguMC4yOjE3MDcwBCBEFCaXerhNImkKKabuX5ULWf2Bp4AzPNJEbXVWgraLrAA=

    juju register --replace MFATA3JvZDAnExMxMDQuMTU0LjQyLjQ0OjE3MDcwExAxMC4xMjguMC4yOjE3MDcwBCBEFCaXerhNImkKKabuX5ULWf2Bp4AzPNJEbXVWgraLrAA=

    juju register public-controller.example.com
`

// Info implements Command.Info
// `register` may seem generic, but is seen as simple and without potential
// naming collisions in any current or planned features.
func (c *registerCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "register",
		Args:     "<registration string>|<controller host name>",
		Purpose:  usageRegisterSummary,
		Doc:      usageRegisterDetails,
		Examples: usageRegisterExamples,
		SeeAlso: []string{
			"add-user",
			"change-user-password",
			"unregister",
		},
	})
}

// SetFlags implements Command.SetFlags.
func (c *registerCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	f.BoolVar(&c.replace, "replace", false, "replace any existing controller")
}

// Init implements Command.Init.
func (c *registerCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.New("registration data missing")
	}
	c.arg, args = args[0], args[1:]
	if err := cmd.CheckEmpty(args); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// SetClientStore implements Command.SetClientStore.
func (c *registerCommand) SetClientStore(store jujuclient.ClientStore) {
	c.store = store
}

// Run implements Command.Run.
func (c *registerCommand) Run(ctx *cmd.Context) error {
	err := c.run(ctx)
	if err != nil && c.onRunError != nil {
		c.onRunError()
	}
	return err
}

func (c *registerCommand) run(ctx *cmd.Context) error {
	c.store = modelcmd.QualifyingClientStore{c.store}
	registrationParams, err := c.getParameters(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	// Check if user is trying to register an already known controller by
	// comparing its public and private addresses to all known addresses.
	if registrationParams.publicHost != "" {
		if err := c.ensureNotKnownPublicEndpoint(registrationParams.publicHost); err != nil {
			return errors.Trace(err)
		}
	} else {
		if err := c.ensureNotKnownEndpoint(registrationParams.userTag.Id(), registrationParams.controllerAddrs); err != nil {
			return errors.Trace(err)
		}
	}

	// If we're not registering to a public controller, we need to prompt for a password.
	if registrationParams.publicHost == "" {
		newPassword, err := c.promptNewPassword(ctx.Stderr, ctx.Stdin)
		if err != nil {
			return errors.Trace(err)
		}
		registrationParams.newPassword = newPassword
	}

	controllerName, err := c.promptControllerName(registrationParams.defaultControllerName, ctx.Stderr, ctx.Stdin)
	if err != nil {
		return errors.Trace(err)
	}

	controllerDetails, accountDetails, err := c.controllerDetails(ctx, registrationParams, controllerName)
	if err != nil {
		return errors.Trace(err)
	}
	if err := c.updateController(
		c.store,
		controllerName,
		controllerDetails,
		accountDetails,
	); err != nil {
		return errors.Trace(err)
	}
	// Log into the controller to verify the credentials, and
	// list the models available.
	models, err := c.listModelsFunc(ctx, c.store, controllerName, accountDetails.User)
	if err != nil {
		return errors.Trace(err)
	}
	if err := c.SetControllerModels(c.store, controllerName, models); err != nil {
		return errors.Annotate(err, "storing model details")
	}
	if err := c.store.SetCurrentController(controllerName); err != nil {
		return errors.Trace(err)
	}

	fmt.Fprintf(
		ctx.Stderr, "\nWelcome, %s. You are now logged into %q.\n",
		friendlyUserName(accountDetails.User), controllerName,
	)
	return c.maybeSetCurrentModel(ctx, c.store, controllerName, accountDetails.User, models)
}

func friendlyUserName(user string) string {
	u := names.NewUserTag(user)
	if u.IsLocal() {
		return u.Name()
	}
	return u.Id()
}

// controllerDetails returns controller and account details to be registered for the
// given registration parameters.
func (c *registerCommand) controllerDetails(ctx *cmd.Context, p *registrationParams, controllerName string) (jujuclient.ControllerDetails, jujuclient.AccountDetails, error) {
	if p.publicHost != "" {
		return c.publicControllerDetails(ctx, p.publicHost, controllerName)
	}
	return c.nonPublicControllerDetails(ctx, p, controllerName)
}

func cookieURL(host string) (*url.URL, error) {
	if strings.Contains(host, ":") {
		var err error
		host, _, err = net.SplitHostPort(host)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	return url.Parse(host)
}

// publicControllerDetails returns controller and account details to be registered
// for the given public controller host name.
func (c *registerCommand) publicControllerDetails(ctx *cmd.Context, host, controllerName string) (jujuclient.ControllerDetails, jujuclient.AccountDetails, error) {
	errRet := func(err error) (jujuclient.ControllerDetails, jujuclient.AccountDetails, error) {
		return jujuclient.ControllerDetails{}, jujuclient.AccountDetails{}, err
	}
	apiAddr := host
	if !strings.Contains(apiAddr, ":") {
		apiAddr += ":443"
	}

	// Make a direct API connection because we don't yet know the
	// controller UUID so can't store the thus-incomplete controller
	// details to make a conventional connection.
	//
	// Unfortunately this means we'll connect twice to the controller
	// but it's probably best to go through the conventional path the
	// second time.
	bclient, err := c.BakeryClient(c.store, controllerName)
	if err != nil {
		return errRet(errors.Trace(err))
	}

	cookieURL, err := cookieURL(host)
	if err != nil {
		return errRet(err)
	}

	dialOpts := api.DefaultDialOpts()
	dialOpts.BakeryClient = bclient

	var supportsOIDCLogin bool
	var sessionToken string

	// we set up a login provider that will first try to log in using
	// oauth device flow, failing that it will try to log in using
	// user-pass or macaroons.
	dialOpts.LoginProvider = loginprovider.NewTryInOrderLoginProvider(
		internallogger.GetLogger("juju.cmd.loginprovider"),
		api.NewSessionTokenLoginProvider(
			"",
			ctx.Stderr,
			func(t string) {
				supportsOIDCLogin = true
				sessionToken = t
			},
		),
		api.NewLegacyLoginProvider(names.UserTag{}, "", "", nil, bclient, cookieURL),
	)

	conn, err := c.apiOpen(ctx, &api.Info{
		Addrs: []string{apiAddr},
	}, dialOpts)
	if err != nil {
		return errRet(errors.Trace(err))
	}
	defer conn.Close()
	user, ok := conn.AuthTag().(names.UserTag)
	if !ok {
		return errRet(errors.Errorf("logged in as %v, not a user", conn.AuthTag()))
	}
	// If we get to here, then we have a cached macaroon for the registered
	// user. If we encounter an error after here, we need to clear it.
	c.onRunError = func() {
		if err := c.ClearControllerMacaroons(c.store, controllerName); err != nil {
			logger.Errorf(context.TODO(), "failed to clear macaroon: %v", err)
		}
	}
	return jujuclient.ControllerDetails{
			APIEndpoints:   []string{apiAddr},
			ControllerUUID: conn.ControllerTag().Id(),
			OIDCLogin:      supportsOIDCLogin,
		}, jujuclient.AccountDetails{
			User:            user.Id(),
			LastKnownAccess: conn.ControllerAccess(),
			SessionToken:    sessionToken,
		}, nil
}

func getProxier(proxyConfig params.Proxy) (*jujuclient.ProxyConfWrapper, error) {
	f, err := factory.NewDefaultFactory()
	if err != nil {
		return nil, errors.Annotate(err, "cannot create default proxy factory")
	}
	proxier, err := f.ProxierFromConfig(proxyConfig.Type, proxyConfig.Config)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &jujuclient.ProxyConfWrapper{Proxier: proxier}, nil
}

// nonPublicControllerDetails returns controller and account details to be registered with
// respect to the given registration parameters.
func (c *registerCommand) nonPublicControllerDetails(ctx *cmd.Context, registrationParams *registrationParams, controllerName string) (jujuclient.ControllerDetails, jujuclient.AccountDetails, error) {
	errRet := func(err error) (jujuclient.ControllerDetails, jujuclient.AccountDetails, error) {
		return jujuclient.ControllerDetails{}, jujuclient.AccountDetails{}, err
	}
	// During registration we must set a new password. This has to be done
	// atomically with the clearing of the secret key.
	payloadBytes, err := json.Marshal(params.SecretKeyLoginRequestPayload{
		Password: registrationParams.newPassword,
	})
	if err != nil {
		return errRet(errors.Trace(err))
	}

	controllerDetails := jujuclient.ControllerDetails{
		APIEndpoints: registrationParams.controllerAddrs,
	}

	if registrationParams.proxyConfig != "" {
		var proxy jujuclient.ProxyConfWrapper
		if err := yaml.Unmarshal([]byte(registrationParams.proxyConfig), &proxy); err != nil {
			return errRet(errors.Trace(err))
		}
		controllerDetails.Proxy = &proxy
	}

	// Make the registration call. If this is successful, the client's
	// cookie jar will be populated with a macaroon that may be used
	// to log in below without the user having to type in the password
	// again.
	req := params.SecretKeyLoginRequest{
		Nonce: registrationParams.nonce[:],
		User:  registrationParams.userTag.String(),
		PayloadCiphertext: secretbox.Seal(
			nil, payloadBytes,
			&registrationParams.nonce,
			&registrationParams.key,
		),
	}
	resp, err := c.secretKeyLogin(ctx, controllerDetails, req, controllerName)
	if err != nil {
		return errRet(errors.Trace(err))
	}

	// Decrypt the response to authenticate the controller and
	// obtain its CA certificate.
	if len(resp.Nonce) != len(registrationParams.nonce) {
		return errRet(errors.NotValidf("response nonce"))
	}
	var respNonce [24]byte
	copy(respNonce[:], resp.Nonce)
	payloadBytes, ok := secretbox.Open(nil, resp.PayloadCiphertext, &respNonce, &registrationParams.key)
	if !ok {
		return errRet(errors.NotValidf("response payload"))
	}
	var responsePayload params.SecretKeyLoginResponsePayload
	if err := json.Unmarshal(payloadBytes, &responsePayload); err != nil {
		return errRet(errors.Annotate(err, "unmarshalling response payload"))
	}
	controllerDetails.ControllerUUID = responsePayload.ControllerUUID
	controllerDetails.CACert = responsePayload.CACert

	if responsePayload.ProxyConfig != nil {
		if controllerDetails.Proxy, err = getProxier(*responsePayload.ProxyConfig); err != nil {
			return errRet(errors.Annotate(err, "creating proxier from config"))
		}
	}

	user := registrationParams.userTag.Id()
	ctx.Infof("Initial password successfully set for %s.", friendlyUserName(user))
	// If we get to here, then we have a cached macaroon for the registered
	// user. If we encounter an error after here, we need to clear it.
	c.onRunError = func() {
		if err := c.ClearControllerMacaroons(c.store, controllerName); err != nil {
			logger.Errorf(context.TODO(), "failed to clear macaroon: %v", err)
		}
	}
	return controllerDetails, jujuclient.AccountDetails{
		User:            user,
		LastKnownAccess: string(permission.LoginAccess),
	}, nil
}

// updateController prompts for a controller name and updates the
// controller and account details in the given client store.
func (c *registerCommand) updateController(
	store jujuclient.ClientStore,
	controllerName string,
	controllerDetails jujuclient.ControllerDetails,
	accountDetails jujuclient.AccountDetails,
) error {
	// Check that we're not trying to register an existing controller under a new name unless we've specified --replace.
	// Also check that we're not trying to --replace a controller that we're currently logged into.
	all, err := store.AllControllers()
	if err != nil {
		return errors.Trace(err)
	}
	for name, ctl := range all {
		if ctl.ControllerUUID == controllerDetails.ControllerUUID {
			if !c.replace || controllerName != name {
				return genAlreadyRegisteredError(name, accountDetails.User)
			}

			// We are replacing a known controller - reject the request if this client is logged into that controller.
			existingLogin, err := c.store.AccountDetails(controllerName)
			if err != nil {
				if errors.Is(err, errors.NotFound) {
					// Not logged in. Continue to update.
					break
				}
				return errors.Trace(err)
			}
			return genReplacingLoggedInControllerError(controllerName, existingLogin.User, accountDetails.User)
		}
	}
	if c.replace {
		if err := store.UpdateController(controllerName, controllerDetails); err != nil {
			if !errors.Is(err, errors.NotFound) {
				return errors.Trace(err)
			}
			if err := store.AddController(controllerName, controllerDetails); err != nil {
				return errors.Trace(err)
			}
		}
	} else {
		if err := store.AddController(controllerName, controllerDetails); err != nil {
			return errors.Trace(err)
		}
	}
	if err := store.UpdateAccount(controllerName, accountDetails); err != nil {
		return errors.Annotatef(err, "cannot update account information: %v", err)
	}
	return nil
}

func (c *registerCommand) listModels(ctx context.Context, store jujuclient.ClientStore, controllerName, userName string) ([]base.UserModel, error) {
	api, err := c.NewAPIRoot(ctx, store, controllerName, "")
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer api.Close()
	mm := modelmanager.NewClient(api)
	return mm.ListModels(ctx, userName)
}

func (c *registerCommand) maybeSetCurrentModel(ctx *cmd.Context, store jujuclient.ClientStore, controllerName, userName string, models []base.UserModel) error {
	if len(models) == 0 {
		fmt.Fprint(ctx.Stderr, noModelsMessage)
		return nil
	}

	// If we get to here, there is at least one model.
	if len(models) == 1 {
		// There is exactly one model shared,
		// so set it as the current model.
		model := models[0]
		modelName := jujuclient.QualifyModelName(model.Qualifier, model.Name)
		err := store.SetCurrentModel(controllerName, modelName)
		if err != nil {
			return errors.Trace(err)
		}
		fmt.Fprintf(ctx.Stderr, "\nCurrent model set to %q.\n", modelName)
		return nil
	}
	fmt.Fprintf(ctx.Stderr, `
There are %d models available. Use "juju switch" to select
one of them:
`, len(models))
	ownerModelNames := make(set.Strings)
	otherModelNames := make(set.Strings)
	for _, model := range models {
		if model.Qualifier == userName {
			ownerModelNames.Add(model.Name)
			continue
		}
		modelName := common.UserQualifiedModelName(model.Name, model.Qualifier, userName)
		otherModelNames.Add(modelName)
	}
	for _, modelName := range ownerModelNames.SortedValues() {
		fmt.Fprintf(ctx.Stderr, "  - juju switch %s\n", modelName)
	}
	for _, modelName := range otherModelNames.SortedValues() {
		fmt.Fprintf(ctx.Stderr, "  - juju switch %s\n", modelName)
	}
	return nil
}

type registrationParams struct {
	// publicHost holds the host name of a public controller.
	// If this is set, all other fields will be empty.
	publicHost string

	defaultControllerName string
	userTag               names.UserTag
	controllerAddrs       []string
	key                   [32]byte
	nonce                 [24]byte
	newPassword           string
	proxyConfig           string
}

// getParameters gets all of the parameters required for registering, prompting
// the user as necessary.
func (c *registerCommand) getParameters(ctx *cmd.Context) (*registrationParams, error) {
	var params registrationParams
	if strings.Contains(c.arg, ".") || c.arg == "localhost" {
		// Looks like a host name - no URL-encoded base64 string should
		// contain a dot and every public controller name should.
		// Allow localhost for development purposes.
		params.publicHost = c.arg
		// No need for password shenanigans if we're using a public controller.
		return &params, nil
	}
	// Decode key, username, controller addresses from the string supplied
	// on the command line.
	decodedData, err := base64.URLEncoding.DecodeString(c.arg)
	if err != nil {
		return nil, errors.Annotatef(err, "invalid registration token")
	}
	var info jujuclient.RegistrationInfo
	if _, err := asn1.Unmarshal(decodedData, &info); err != nil {
		return nil, errors.Trace(err)
	}

	params.controllerAddrs = info.Addrs
	params.proxyConfig = info.ProxyConfig
	params.userTag = names.NewUserTag(info.User)
	if len(info.SecretKey) != len(params.key) {
		return nil, errors.NotValidf("secret key")
	}
	copy(params.key[:], info.SecretKey)
	params.defaultControllerName = info.ControllerName

	// Generate a random nonce for encrypting the request.
	if _, err := rand.Read(params.nonce[:]); err != nil {
		return nil, errors.Trace(err)
	}

	return &params, nil
}

func (c *registerCommand) secretKeyLogin(
	ctx context.Context,
	controllerDetails jujuclient.ControllerDetails, request params.SecretKeyLoginRequest,
	controllerName string,
) (_ *params.SecretKeyLoginResponse, err error) {
	cookieJar, err := c.CookieJar(c.store, controllerName)
	if err != nil {
		return nil, errors.Annotatef(err, "internal error: cannot get API context")
	}

	buf, err := json.Marshal(&request)
	if err != nil {
		return nil, errors.Annotatef(err, "internal error: cannot marshell controller api request")
	}
	r := bytes.NewReader(buf)

	// Determine which address to use by attempting to open an API
	// connection with each of the addresses. Note that we do not
	// know the CA certificate yet, so we do not want to send any
	// sensitive information. We make no attempt to log in until
	// we can verify the server's identity.
	opts := api.DefaultDialOpts()
	opts.InsecureSkipVerify = true
	apiInfo := &api.Info{
		Addrs:     controllerDetails.APIEndpoints,
		SkipLogin: true,
	}
	if controllerDetails.Proxy != nil {
		apiInfo.Proxier = controllerDetails.Proxy.Proxier
	}
	conn, err := c.apiOpen(ctx, apiInfo, opts)
	if err != nil {
		logger.Infof(context.TODO(), "opening api connection: %s", err)
		return nil, controllerUnreachableError(controllerName, controllerDetails.APIEndpoints)
	}
	defer func() {
		if closeErr := conn.Close(); closeErr != nil {
			if err == nil {
				err = closeErr
			} else {
				logger.Warningf(context.TODO(), "error closing API connection: %v", closeErr)
			}
		}
	}()

	// Using the address we connected to above, perform the request.
	// A success response will include a macaroon cookie that we can
	// use to log in with.
	url := conn.Addr()
	url.Scheme = "https"
	url.Path = path.Join(url.Path, "register")
	httpReq, err := http.NewRequest("POST", url.String(), r)
	if err != nil {
		return nil, errors.Annotatef(err, "internal error: creating new http request")
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set(httpbakery.BakeryProtocolHeader, fmt.Sprint(bakery.LatestVersion))
	httpClient := jujuhttp.NewClient(
		jujuhttp.WithSkipHostnameVerification(true),
		jujuhttp.WithCookieJar(cookieJar),
		jujuhttp.WithLogger(logger.Child("http", corelogger.HTTP)),
	)
	httpResp, err := httpClient.Do(httpReq)
	if err != nil {
		logger.Infof(context.TODO(), "connecting to controller: %s", err)
		return nil, controllerUnreachableError(controllerName, controllerDetails.APIEndpoints)
	}
	defer func() { _ = httpResp.Body.Close() }()

	if httpResp.StatusCode != http.StatusOK {
		var resp params.ErrorResult
		if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
			return nil, errors.Annotatef(err, "internal error: cannot decode http response")
		}
		logger.Infof(context.TODO(), "error response, %s", resp.Error)
		return nil, errors.Errorf("Provided registration token may have expired."+
			"\nA controller administrator must reset your user to issue a new token.\nSee %q for more information.", "juju help change-user-password")
	}

	var resp params.SecretKeyLoginResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		return nil, errors.Annotatef(err, "internal error: cannot decode controller response")
	}
	return &resp, nil
}

func (c *registerCommand) promptNewPassword(stderr io.Writer, stdin io.Reader) (string, error) {
	password, err := c.readPassword("Enter a new password: ", stderr, stdin)
	if err != nil {
		return "", errors.Annotatef(err, "cannot read password")
	}
	if password == "" {
		return "", errors.NewNotValid(nil, "you must specify a non-empty password")
	}
	passwordConfirmation, err := c.readPassword("Confirm password: ", stderr, stdin)
	if err != nil {
		return "", errors.Trace(err)
	}
	if password != passwordConfirmation {
		return "", errors.Errorf("passwords do not match")
	}
	return password, nil
}

func (c *registerCommand) promptControllerName(suggestedName string, stderr io.Writer, stdin io.Reader) (string, error) {
	if suggestedName != "" {
		if _, err := c.store.ControllerByName(suggestedName); err == nil && !c.replace {
			suggestedName = ""
		}
	}
	for {
		var setMsg string
		setMsg = "Enter a name for this controller: "
		if suggestedName != "" {
			replace := ""
			if c.replace {
				replace = "replace "
			}
			setMsg = fmt.Sprintf("Enter a name for this controller [%s%s]: ", replace, suggestedName)
		}
		fmt.Fprint(stderr, setMsg)
		name, err := c.readLine(stdin)
		if err != nil {
			return "", errors.Trace(err)
		}
		name = strings.TrimSpace(name)
		if name == "" {
			if suggestedName == "" {
				fmt.Fprintln(stderr, "You must specify a non-empty controller name.")
				continue
			}
			name = suggestedName
		}
		_, err = c.store.ControllerByName(name)
		if err == nil && !c.replace {
			fmt.Fprintf(stderr, "Controller %q already exists.\n", name)
			continue
		}
		return name, nil
	}
}

func (c *registerCommand) readPassword(prompt string, stderr io.Writer, stdin io.Reader) (string, error) {
	fmt.Fprintf(stderr, "%s", prompt)
	defer stderr.Write([]byte{'\n'})
	if f, ok := stdin.(*os.File); ok && terminal.IsTerminal(int(f.Fd())) {
		password, err := terminal.ReadPassword(int(f.Fd()))
		if err != nil {
			return "", errors.Trace(err)
		}
		return string(password), nil
	}
	return c.readLine(stdin)
}

func (c *registerCommand) readLine(stdin io.Reader) (string, error) {
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

func (r byteAtATimeReader) Read(out []byte) (int, error) {
	return r.Reader.Read(out[:1])
}

// ensureNotKnownPublicEndpoint checks whether any controllers in the local client
// cache contain the provided hostname endpoint and returns an error if that is the
// case.
func (c *registerCommand) ensureNotKnownPublicEndpoint(publicHost string) error {
	existingDetails, existingName, err := c.store.ControllerByAPIEndpoints(publicHost)
	if err != nil && !errors.IsNotFound(err) {
		return errors.Trace(err)
	}

	if existingDetails == nil {
		return nil
	}

	// Check if we know the username for this controller
	accountDetails, err := c.store.AccountDetails(existingName)
	if err != nil && !errors.IsNotFound(err) {
		return errors.Trace(err)
	}

	if accountDetails != nil {
		return genAlreadyRegisteredPublicTokenStillValidError(existingName, accountDetails.User)
	}

	return errors.Errorf(`A controller with the same hostname has already been registered on this client as %q.
To login run 'juju login -c %s'.`, existingName, existingName)
}

// ensureNotKnownEndpoint checks whether any controllers in the local client
// cache contain any of the provided address endpoints and returns an error
// if that is the case.
func (c *registerCommand) ensureNotKnownEndpoint(user string, controllerAddrs []string) error {
	allAddresses := set.NewStrings(controllerAddrs...)
	existingDetails, existingName, err := c.store.ControllerByAPIEndpoints(allAddresses.Values()...)
	if err != nil && !errors.Is(err, errors.NotFound) {
		return errors.Trace(err)
	}

	if existingDetails == nil {
		return nil
	}

	// Check if we know the username for this controller
	accountDetails, err := c.store.AccountDetails(existingName)
	if err != nil && !errors.Is(err, errors.NotFound) {
		return errors.Trace(err)
	}

	if accountDetails != nil {
		return genAlreadyRegisteredTokenStillValidError(existingName, accountDetails.User, user)
	}

	if c.replace {
		return nil
	}

	return errors.Errorf(`A controller with the same address has already been registered on this client as %q.
To login run 'juju login -c %s'.`, existingName, existingName)
}

var alreadyRegisteredMessageT = template.Must(template.New("").Parse(`
This controller has already been registered on this client as "{{.ControllerName}}".
To login as user "{{.UserName}}" run 'juju login -u {{.UserName}} -c {{.ControllerName}}'.
To update controller details and login as user "{{.UserName}}":
    1. run 'juju unregister {{.ControllerName}}'
    2. request from your controller admin another registration string, i.e
       output from 'juju change-user-password {{.UserName}} --reset'
    3. re-run 'juju register' with the registration string from (2) above.
`[1:]))

func genAlreadyRegisteredError(controller, user string) error {
	var buf bytes.Buffer
	if err := alreadyRegisteredMessageT.Execute(
		&buf,
		struct {
			ControllerName string
			UserName       string
		}{controller, user},
	); err != nil {
		return err
	}
	return errors.New(buf.String())
}

var alreadyRegisteredMessageTokenStillValidT = template.Must(template.New("").Parse(`
A controller with the same address has already been registered on this client as "{{.ControllerName}}".
You are already logged in as user "{{.LoggedInUserName}}".
To update controller details and login as user "{{.UserName}}":
    1. run 'juju logout'
    2. re-run 'juju register --replace' with your registration string.
`[1:]))

// genAlreadyRegisteredTokenStillValidError generates an error message for the case where the controller is already
// registered, but the registration token is still valid and can be used again.
func genAlreadyRegisteredTokenStillValidError(controller, loggedInUser, user string) error {
	var buf bytes.Buffer
	if err := alreadyRegisteredMessageTokenStillValidT.Execute(
		&buf,
		struct {
			ControllerName   string
			UserName         string
			LoggedInUserName string
		}{ControllerName: controller, UserName: user, LoggedInUserName: loggedInUser},
	); err != nil {
		return err
	}
	return errors.New(buf.String())
}

var alreadyRegisteredPublicMessageTokenStillValidT = template.Must(template.New("").Parse(`
A controller with the same address has already been registered on this client as "{{.ControllerName}}".
You are already logged in as user "{{.LoggedInUserName}}".
To update controller details:
    1. run 'juju logout'
    2. re-run 'juju register --replace' with your registration string.
`[1:]))

// genAlreadyRegisteredPublicTokenStillValidError generates an error message for the case where
// a public controller is already registered, but the registration token is still valid and can be used again.
func genAlreadyRegisteredPublicTokenStillValidError(controller, loggedInUser string) error {
	var buf bytes.Buffer
	if err := alreadyRegisteredPublicMessageTokenStillValidT.Execute(
		&buf,
		struct {
			ControllerName   string
			LoggedInUserName string
		}{ControllerName: controller, LoggedInUserName: loggedInUser},
	); err != nil {
		return err
	}
	return errors.New(buf.String())
}

var replacingLoggedInControllerT = template.Must(template.New("").Parse(`
User "{{.LoggedInUserName}}" is currently logged into controller "{{.ControllerName}}".
Cannot replace a controller we're currently logged into.
To register and replace this controller:
    1. run 'juju logout -c {{.ControllerName}}'
    2. request from your controller admin another registration string, i.e
       output from 'juju change-user-password {{.UserName}} --reset'
    3. re-run 'juju register TOKEN --replace' with the registration string from (2) above.
`[1:]))

func genReplacingLoggedInControllerError(controller, loggedInUser, user string) error {
	var buf bytes.Buffer
	if err := replacingLoggedInControllerT.Execute(&buf, struct {
		ControllerName   string
		UserName         string
		LoggedInUserName string
	}{ControllerName: controller, UserName: user, LoggedInUserName: loggedInUser}); err != nil {
		return err
	}
	return errors.New(buf.String())
}

func controllerUnreachableError(name string, endpoints []string) error {
	return fmt.Errorf("Cannot reach controller %q at: %s."+
		"\nCheck that the controller ip is reachable from your network.",
		name, strings.Join(endpoints, ", "))
}
