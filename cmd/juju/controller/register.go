// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/utils"
	"github.com/juju/utils/set"
	"golang.org/x/crypto/nacl/secretbox"
	"golang.org/x/crypto/ssh/terminal"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/modelmanager"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/permission"
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
	return modelcmd.WrapBase(c)
}

// registerCommand logs in to a Juju controller and caches the connection
// information.
type registerCommand struct {
	modelcmd.CommandBase
	apiOpen        api.OpenFunc
	listModelsFunc func(_ jujuclient.ClientStore, controller, user string) ([]base.UserModel, error)
	store          jujuclient.ClientStore
	Arg            string

	// onRunError is executed if non-nil if there is an error at the end
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

When adding a controller at a public address, authentication via some
external third party (for example Ubuntu SSO) will be required, usually
by using a web browser.

Examples:

    juju register MFATA3JvZDAnExMxMDQuMTU0LjQyLjQ0OjE3MDcwExAxMC4xMjguMC4yOjE3MDcwBCBEFCaXerhNImkKKabuX5ULWf2Bp4AzPNJEbXVWgraLrAA=

    juju register public-controller.example.com

See also: 
    add-user
    change-user-password
    unregister`

// Info implements Command.Info
// `register` may seem generic, but is seen as simple and without potential
// naming collisions in any current or planned features.
func (c *registerCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "register",
		Args:    "<registration string>|<controller host name>",
		Purpose: usageRegisterSummary,
		Doc:     usageRegisterDetails,
	}
}

// SetFlags implements Command.Init.
func (c *registerCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.New("registration data missing")
	}
	c.Arg, args = args[0], args[1:]
	if err := cmd.CheckEmpty(args); err != nil {
		return errors.Trace(err)
	}
	return nil
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
	store := modelcmd.QualifyingClientStore{c.store}
	registrationParams, err := c.getParameters(ctx, store)
	if err != nil {
		return errors.Trace(err)
	}
	controllerDetails, accountDetails, err := c.controllerDetails(ctx, registrationParams)
	if err != nil {
		return errors.Trace(err)
	}
	controllerName, err := c.updateController(
		ctx,
		store,
		registrationParams.defaultControllerName,
		controllerDetails,
		accountDetails,
	)
	if err != nil {
		return errors.Trace(err)
	}
	// Log into the controller to verify the credentials, and
	// list the models available.
	models, err := c.listModelsFunc(store, controllerName, accountDetails.User)
	if err != nil {
		return errors.Trace(err)
	}
	for _, model := range models {
		owner := names.NewUserTag(model.Owner)
		if err := store.UpdateModel(
			controllerName,
			jujuclient.JoinOwnerModelName(owner, model.Name),
			jujuclient.ModelDetails{model.UUID},
		); err != nil {
			return errors.Annotate(err, "storing model details")
		}
	}
	if err := store.SetCurrentController(controllerName); err != nil {
		return errors.Trace(err)
	}

	fmt.Fprintf(
		ctx.Stderr, "\nWelcome, %s. You are now logged into %q.\n",
		friendlyUserName(accountDetails.User), controllerName,
	)
	return c.maybeSetCurrentModel(ctx, store, controllerName, accountDetails.User, models)
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
func (c *registerCommand) controllerDetails(ctx *cmd.Context, p *registrationParams) (jujuclient.ControllerDetails, jujuclient.AccountDetails, error) {
	if p.publicHost != "" {
		return c.publicControllerDetails(p.publicHost)
	}
	return c.nonPublicControllerDetails(ctx, p)
}

// publicControllerDetails returns controller and account details to be registered
// for the given public controller host name.
func (c *registerCommand) publicControllerDetails(host string) (jujuclient.ControllerDetails, jujuclient.AccountDetails, error) {
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
	bclient, err := c.BakeryClient()
	if err != nil {
		return errRet(errors.Trace(err))
	}
	dialOpts := api.DefaultDialOpts()
	dialOpts.BakeryClient = bclient
	conn, err := c.apiOpen(&api.Info{
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
		if err := c.ClearControllerMacaroons([]string{apiAddr}); err != nil {
			logger.Errorf("failed to clear macaroon: %v", err)
		}
	}
	return jujuclient.ControllerDetails{
			APIEndpoints:   []string{apiAddr},
			ControllerUUID: conn.ControllerTag().Id(),
		}, jujuclient.AccountDetails{
			User:            user.Id(),
			LastKnownAccess: conn.ControllerAccess(),
		}, nil
}

// nonPublicControllerDetails returns controller and account details to be registered with
// respect to the given registration parameters.
func (c *registerCommand) nonPublicControllerDetails(ctx *cmd.Context, registrationParams *registrationParams) (jujuclient.ControllerDetails, jujuclient.AccountDetails, error) {
	errRet := func(err error) (jujuclient.ControllerDetails, jujuclient.AccountDetails, error) {
		return jujuclient.ControllerDetails{}, jujuclient.AccountDetails{}, err
	}
	// During registration we must set a new password. This has to be done
	// atomically with the clearing of the secret key.
	payloadBytes, err := json.Marshal(params.SecretKeyLoginRequestPayload{
		registrationParams.newPassword,
	})
	if err != nil {
		return errRet(errors.Trace(err))
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
	resp, err := c.secretKeyLogin(registrationParams.controllerAddrs, req)
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
	user := registrationParams.userTag.Id()
	ctx.Infof("Initial password successfully set for %s.", friendlyUserName(user))
	// If we get to here, then we have a cached macaroon for the registered
	// user. If we encounter an error after here, we need to clear it.
	c.onRunError = func() {
		if err := c.ClearControllerMacaroons(registrationParams.controllerAddrs); err != nil {
			logger.Errorf("failed to clear macaroon: %v", err)
		}
	}
	return jujuclient.ControllerDetails{
			APIEndpoints:   registrationParams.controllerAddrs,
			ControllerUUID: responsePayload.ControllerUUID,
			CACert:         responsePayload.CACert,
		}, jujuclient.AccountDetails{
			User:            user,
			LastKnownAccess: string(permission.LoginAccess),
		}, nil
}

// updateController prompts for a controller name and updates the
// controller and account details in the given client store.
// It returns the name of the updated controller.
func (c *registerCommand) updateController(
	ctx *cmd.Context,
	store jujuclient.ClientStore,
	defaultControllerName string,
	controllerDetails jujuclient.ControllerDetails,
	accountDetails jujuclient.AccountDetails,
) (string, error) {
	// Check that the same controller isn't already stored, so that we
	// can avoid needlessly asking for a controller name in that case.
	all, err := store.AllControllers()
	if err != nil {
		return "", errors.Trace(err)
	}
	for name, ctl := range all {
		if ctl.ControllerUUID == controllerDetails.ControllerUUID {
			// TODO(rogpeppe) lp#1614010 Succeed but override the account details in this case?
			return "", errors.Errorf("controller is already registered as %q", name)
		}
	}
	controllerName, err := c.promptControllerName(store, defaultControllerName, ctx.Stderr, ctx.Stdin)
	if err != nil {
		return "", errors.Trace(err)
	}

	if err := store.AddController(controllerName, controllerDetails); err != nil {
		return "", errors.Trace(err)
	}
	if err := store.UpdateAccount(controllerName, accountDetails); err != nil {
		return "", errors.Annotatef(err, "cannot update account information: %v", err)
	}
	return controllerName, nil
}

func (c *registerCommand) listModels(store jujuclient.ClientStore, controllerName, userName string) ([]base.UserModel, error) {
	api, err := c.NewAPIRoot(store, controllerName, "")
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer api.Close()
	mm := modelmanager.NewClient(api)
	return mm.ListModels(userName)
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
		owner := names.NewUserTag(model.Owner)
		modelName := jujuclient.JoinOwnerModelName(owner, model.Name)
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
	user := names.NewUserTag(userName)
	ownerModelNames := make(set.Strings)
	otherModelNames := make(set.Strings)
	for _, model := range models {
		if model.Owner == userName {
			ownerModelNames.Add(model.Name)
			continue
		}
		owner := names.NewUserTag(model.Owner)
		modelName := common.OwnerQualifiedModelName(model.Name, owner, user)
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
}

// getParameters gets all of the parameters required for registering, prompting
// the user as necessary.
func (c *registerCommand) getParameters(ctx *cmd.Context, store jujuclient.ClientStore) (*registrationParams, error) {
	var params registrationParams
	if strings.Contains(c.Arg, ".") || c.Arg == "localhost" {
		// Looks like a host name - no URL-encoded base64 string should
		// contain a dot and every public controller name should.
		// Allow localhost for development purposes.
		params.publicHost = c.Arg
		// No need for password shenanigans if we're using a public controller.
		return &params, nil
	}
	// Decode key, username, controller addresses from the string supplied
	// on the command line.
	decodedData, err := base64.URLEncoding.DecodeString(c.Arg)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var info jujuclient.RegistrationInfo
	if _, err := asn1.Unmarshal(decodedData, &info); err != nil {
		return nil, errors.Trace(err)
	}

	params.controllerAddrs = info.Addrs
	params.userTag = names.NewUserTag(info.User)
	if len(info.SecretKey) != len(params.key) {
		return nil, errors.NotValidf("secret key")
	}
	copy(params.key[:], info.SecretKey)
	params.defaultControllerName = info.ControllerName

	// Prompt the user for the new password to set.
	newPassword, err := c.promptNewPassword(ctx.Stderr, ctx.Stdin)
	if err != nil {
		return nil, errors.Trace(err)
	}
	params.newPassword = newPassword

	// Generate a random nonce for encrypting the request.
	if _, err := rand.Read(params.nonce[:]); err != nil {
		return nil, errors.Trace(err)
	}

	return &params, nil
}

func (c *registerCommand) secretKeyLogin(addrs []string, request params.SecretKeyLoginRequest) (*params.SecretKeyLoginResponse, error) {
	apiContext, err := c.APIContext()
	if err != nil {
		return nil, errors.Annotate(err, "getting API context")
	}

	buf, err := json.Marshal(&request)
	if err != nil {
		return nil, errors.Annotate(err, "marshalling request")
	}
	r := bytes.NewReader(buf)

	// Determine which address to use by attempting to open an API
	// connection with each of the addresses. Note that we do not
	// know the CA certificate yet, so we do not want to send any
	// sensitive information. We make no attempt to log in until
	// we can verify the server's identity.
	opts := api.DefaultDialOpts()
	opts.InsecureSkipVerify = true
	conn, err := c.apiOpen(&api.Info{
		Addrs:     addrs,
		SkipLogin: true,
	}, opts)
	if err != nil {
		return nil, errors.Trace(err)
	}
	apiAddr := conn.Addr()
	if err := conn.Close(); err != nil {
		return nil, errors.Trace(err)
	}

	// Using the address we connected to above, perform the request.
	// A success response will include a macaroon cookie that we can
	// use to log in with.
	urlString := fmt.Sprintf("https://%s/register", apiAddr)
	httpReq, err := http.NewRequest("POST", urlString, r)
	if err != nil {
		return nil, errors.Trace(err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpClient := utils.GetNonValidatingHTTPClient()
	httpClient.Jar = apiContext.CookieJar()
	httpResp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		var resp params.ErrorResult
		if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
			return nil, errors.Trace(err)
		}
		return nil, resp.Error
	}

	var resp params.SecretKeyLoginResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		return nil, errors.Annotatef(err, "cannot decode login response")
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

func (c *registerCommand) promptControllerName(store jujuclient.ClientStore, suggestedName string, stderr io.Writer, stdin io.Reader) (string, error) {
	if suggestedName != "" {
		if _, err := store.ControllerByName(suggestedName); err == nil {
			suggestedName = ""
		}
	}
	for {
		var setMsg string
		setMsg = "Enter a name for this controller: "
		if suggestedName != "" {
			setMsg = fmt.Sprintf("Enter a name for this controller [%s]: ", suggestedName)
		}
		fmt.Fprintf(stderr, setMsg)
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
		_, err = store.ControllerByName(name)
		if err == nil {
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
