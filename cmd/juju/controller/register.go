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
	"github.com/juju/names"
	"github.com/juju/utils"
	"golang.org/x/crypto/nacl/secretbox"
	"golang.org/x/crypto/ssh/terminal"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/jujuclient"
)

// NewRegisterCommand returns a command to allow the user to register a controller.
func NewRegisterCommand() cmd.Command {
	cmd := &registerCommand{}
	cmd.apiOpen = cmd.APIOpen
	cmd.newAPIRoot = cmd.NewAPIRoot
	cmd.store = jujuclient.NewFileClientStore()
	return modelcmd.WrapBase(cmd)
}

// registerCommand logs in to a Juju controller and caches the connection
// information.
type registerCommand struct {
	modelcmd.JujuCommandBase
	apiOpen     api.OpenFunc
	newAPIRoot  modelcmd.OpenFunc
	store       jujuclient.ClientStore
	EncodedData string
}

var registerDoc = `
register connects to a Juju controller and completes the user registration
process started with "juju add-user". The user must supply the string that
was printed out by "juju add-user", which embeds the username to register,
a secret key, and the addresses of the Juju controller.

Executing "juju register" will prompt for a password, and set this as the
initial password for the user. After setting this password, the registration
string will be void and standard login and user management commands will
become usable.

See Also:
    juju help add-user
    juju help list-models
    juju help use-models
    juju help create-model
    juju help switch
`

// Info implements Command.Info
func (c *registerCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "register",
		Args:    "<name>",
		Purpose: "register with a Juju Controller",
		Doc:     registerDoc,
	}
}

// SetFlags implements Command.Init.
func (c *registerCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.New("registration data missing")
	}
	c.EncodedData, args = args[0], args[1:]
	if err := cmd.CheckEmpty(args); err != nil {
		return err
	}
	return nil
}

func (c *registerCommand) Run(ctx *cmd.Context) error {

	registrationParams, err := c.getParameters(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	legacyStore, err := configstore.Default()
	if err != nil {
		return errors.Trace(err)
	}
	controllerInfo, err := legacyStore.ReadInfo(registrationParams.controllerName)
	if err == nil {
		return errors.AlreadyExistsf("controller %q", registrationParams.controllerName)
	} else if !errors.IsNotFound(err) {
		return errors.Trace(err)
	}
	fullModelName := configstore.EnvironInfoName(
		registrationParams.controllerName,
		configstore.AdminModelName(registrationParams.controllerName),
	)
	controllerInfo = legacyStore.CreateInfo(fullModelName)

	// During registration we must set a new password. This has to be done
	// atomically with the clearing of the secret key.
	payloadBytes, err := json.Marshal(params.SecretKeyLoginRequestPayload{
		registrationParams.newPassword,
	})
	if err != nil {
		return errors.Trace(err)
	}

	// Make the registration call.
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
		return errors.Trace(err)
	}

	// Decrypt the response to authenticate the controller and
	// obtain its CA certificate.
	if len(resp.Nonce) != len(registrationParams.nonce) {
		return errors.NotValidf("response nonce")
	}
	var respNonce [24]byte
	copy(respNonce[:], resp.Nonce)
	payloadBytes, ok := secretbox.Open(nil, resp.PayloadCiphertext, &respNonce, &registrationParams.key)
	if !ok {
		return errors.NotValidf("response payload")
	}
	var responsePayload params.SecretKeyLoginResponsePayload
	if err := json.Unmarshal(payloadBytes, &responsePayload); err != nil {
		return errors.Annotate(err, "unmarshalling response payload")
	}

	// Ensure there's a skeleton legacy record written with basic minimum information.
	endpoint := controllerInfo.APIEndpoint()
	endpoint.ServerUUID = responsePayload.ControllerUUID
	endpoint.CACert = responsePayload.CACert
	controllerInfo.SetAPIEndpoint(endpoint)
	controllerInfo.SetAPICredentials(configstore.APICredentials{
		User:     registrationParams.userTag.Id(),
		Password: registrationParams.newPassword,
	})
	if err := controllerInfo.Write(); err != nil {
		return errors.Trace(err)
	}

	// Store the controller and account details.
	controllerDetails := jujuclient.ControllerDetails{
		APIEndpoints:   registrationParams.controllerAddrs,
		ControllerUUID: responsePayload.ControllerUUID,
		CACert:         responsePayload.CACert,
	}
	if err := c.store.UpdateController(registrationParams.controllerName, controllerDetails); err != nil {
		return errors.Trace(err)
	}
	accountDetails := jujuclient.AccountDetails{
		User:     registrationParams.userTag.Canonical(),
		Password: registrationParams.newPassword,
	}
	accountName := accountDetails.User
	if err := c.store.UpdateAccount(
		registrationParams.controllerName, accountName, accountDetails,
	); err != nil {
		return errors.Trace(err)
	}
	if err := c.store.SetCurrentAccount(
		registrationParams.controllerName, accountName,
	); err != nil {
		return errors.Trace(err)
	}

	// Log into the controller to verify the credentials, and
	// refresh the connection information.
	apiConn, err := c.newAPIRoot(c.store, registrationParams.controllerName, accountName, "")
	if err != nil {
		return errors.Trace(err)
	}
	if err := apiConn.Close(); err != nil {
		return errors.Trace(err)
	}
	if err := modelcmd.WriteCurrentController(registrationParams.controllerName); err != nil {
		return errors.Trace(err)
	}

	fmt.Fprintf(
		ctx.Stderr, "\nWelcome, %s. You are now logged into %q.\n",
		registrationParams.userTag.Id(), registrationParams.controllerName,
	)
	return nil
}

type registrationParams struct {
	userTag         names.UserTag
	controllerName  string
	controllerAddrs []string
	key             [32]byte
	nonce           [24]byte
	newPassword     string
}

// getParameters gets all of the parameters required for registering, prompting
// the user as necessary.
func (c *registerCommand) getParameters(ctx *cmd.Context) (*registrationParams, error) {

	// Decode key, username, controller addresses from the string supplied
	// on the command line.
	decodedData, err := base64.URLEncoding.DecodeString(c.EncodedData)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var info jujuclient.RegistrationInfo
	if _, err := asn1.Unmarshal(decodedData, &info); err != nil {
		return nil, errors.Trace(err)
	}

	params := registrationParams{
		controllerAddrs: info.Addrs,
		userTag:         names.NewUserTag(info.User),
	}
	if len(info.SecretKey) != len(params.key) {
		return nil, errors.NotValidf("secret key")
	}
	copy(params.key[:], info.SecretKey)

	// Prompt the user for the controller name.
	controllerName, err := c.promptControllerName(ctx.Stderr, ctx.Stdin)
	if err != nil {
		return nil, errors.Trace(err)
	}
	params.controllerName = controllerName

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
	buf, err := json.Marshal(&request)
	if err != nil {
		return nil, errors.Annotate(err, "marshalling request")
	}
	r := bytes.NewReader(buf)

	// Determine which address to use by attempting to open an API
	// connection with each of the addresses. Note that we don't
	// set a username/password, so no login will be attempted; and
	// we skip verification, because we don't know the CA certificate
	// and we do not send anything sensitive.
	opts := api.DefaultDialOpts()
	opts.InsecureSkipVerify = true
	conn, err := c.apiOpen(&api.Info{Addrs: addrs}, opts)
	if err != nil {
		return nil, errors.Trace(err)
	}
	apiAddr := conn.Addr()
	if err := conn.Close(); err != nil {
		return nil, errors.Trace(err)
	}

	// Using the address we connected to above, perform the request.
	urlString := fmt.Sprintf("https://%s/register", apiAddr)
	httpReq, err := http.NewRequest("POST", urlString, r)
	if err != nil {
		return nil, errors.Trace(err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpClient := utils.GetNonValidatingHTTPClient()
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
		return nil, errors.Trace(err)
	}
	return &resp, nil
}

func (c *registerCommand) promptNewPassword(stderr io.Writer, stdin io.Reader) (string, error) {
	password, err := c.readPassword("Enter password: ", stderr, stdin)
	if err != nil {
		return "", errors.Trace(err)
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

func (c *registerCommand) promptControllerName(stderr io.Writer, stdin io.Reader) (string, error) {
	fmt.Fprintf(stderr, "Please set a name for this controller: ")
	defer stderr.Write([]byte{'\n'})
	name, err := c.readLine(stdin)
	if err != nil {
		return "", errors.Trace(err)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.NewNotValid(nil, "you must specify a non-empty controller name")
	}
	return name, nil
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
