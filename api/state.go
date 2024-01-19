// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"runtime/debug"
	"strconv"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/juju/errors"
	"github.com/juju/featureflag"
	"github.com/juju/names/v5"
	"github.com/juju/utils/v3"
	"github.com/juju/version/v2"

	"github.com/juju/juju/api/agent/keyupdater"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/params"
	jujuversion "github.com/juju/juju/version"
)

var (
	loginDeviceAPICall = func(st base.APICaller, request interface{}, response interface{}) error {
		return st.APICall("Admin", 4, "", "LoginDevice", request, response)
	}
	getAccessTokenAPICall = func(st base.APICaller, request interface{}, response interface{}) error {
		return st.APICall("Admin", 4, "", "GetAccessToken", request, response)
	}
	loginWithAccessTokenAPICall = func(st base.APICaller, request interface{}, response interface{}) error {
		return st.APICall("Admin", 4, "", "LoginWithAccessToken", request, response)
	}
	loginWithClientCredentialsAPICall = func(st base.APICaller, request interface{}, response interface{}) error {
		return st.APICall("Admin", 4, "", "LoginWithClientCredentials", request, response)
	}
)

func (st *state) loginWithDeviceFlow(ctx context.Context, showLoginDetails func(string) error) (params.LoginResult, error) {
	var result params.LoginResult

	type loginRequest struct {
		AccessToken string `json:"access-token"`
	}

	type deviceResponse struct {
		UserCode        string `json:"user-code"`
		VerificationURL string `json:"verification-url"`
	}
	var deviceResult deviceResponse
	err := loginDeviceAPICall(st, &loginRequest{}, &deviceResult)
	if err != nil {
		return result, errors.Trace(err)
	}

	if showLoginDetails == nil {
		return result, errors.New("device login flow not configured")
	}

	err = showLoginDetails(fmt.Sprintf("Please visit %s and enter code %s to log in.", deviceResult.VerificationURL, deviceResult.UserCode))
	if err != nil {
		return result, errors.Trace(err)
	}

	type loginResponse struct {
		AccessToken string `json:"access-token"`
	}
	var accessTokenResult loginResponse
	err = getAccessTokenAPICall(st, &loginRequest{}, &accessTokenResult)
	if err != nil {
		return result, errors.Trace(err)
	}

	// TODO (alesstimec) Persis access token in accounts.yaml.

	return st.loginWithAccessToken(ctx, accessTokenResult.AccessToken)
}

func (st *state) loginWithAccessToken(ctx context.Context, accessToken string) (params.LoginResult, error) {
	type loginRequest struct {
		AccessToken string `json:"access-token"`
	}

	var result params.LoginResult
	err := loginWithAccessTokenAPICall(
		st,
		&loginRequest{
			AccessToken: accessToken,
		},
		&result,
	)
	if err != nil {
		return result, errors.Trace(err)
	}
	return result, nil
}

func (st *state) loginWithClientCredentials(ctx context.Context, clientID, clientSecret string) (params.LoginResult, error) {
	type loginRequest struct {
		ClientID     string `json:"client-id"`
		ClientSecret string `json:"client-secret"`
	}

	var result params.LoginResult
	err := loginWithClientCredentialsAPICall(
		st,
		&loginRequest{
			ClientID:     clientID,
			ClientSecret: clientSecret,
		},
		&result,
	)
	if err != nil {
		return result, errors.Trace(err)
	}
	return result, nil
}

func (st *state) loginV3(p LoginParams) (params.LoginResult, error) {
	var result params.LoginResult
	request := &params.LoginRequest{
		AuthTag:       tagToString(p.Tag),
		Credentials:   p.Password,
		Nonce:         p.Nonce,
		Macaroons:     p.Macaroons,
		BakeryVersion: bakery.LatestVersion,
		CLIArgs:       utils.CommandString(os.Args...),
		ClientVersion: jujuversion.Current.String(),
	}
	// If we are in developer mode, add the stack location as user data to the
	// login request. This will allow the apiserver to connect connection ids
	// to the particular place that initiated the connection.
	if featureflag.Enabled(feature.DeveloperMode) {
		request.UserData = string(debug.Stack())
	}

	if p.Password == "" {
		// Add any macaroons from the cookie jar that might work for
		// authenticating the login request.
		request.Macaroons = append(request.Macaroons,
			httpbakery.MacaroonsForURL(st.bakeryClient.Client.Jar, st.cookieURL)...,
		)
	}
	err := st.APICall("Admin", 3, "", "Login", request, &result)
	if err != nil {
		if !params.IsRedirect(err) {
			return result, errors.Trace(err)
		}

		if rpcErr, ok := errors.Cause(err).(*rpc.RequestError); ok {
			var redirInfo params.RedirectErrorInfo
			err := rpcErr.UnmarshalInfo(&redirInfo)
			if err == nil && redirInfo.CACert != "" && len(redirInfo.Servers) != 0 {
				var controllerTag names.ControllerTag
				if redirInfo.ControllerTag != "" {
					if controllerTag, err = names.ParseControllerTag(redirInfo.ControllerTag); err != nil {
						return result, errors.Trace(err)
					}
				}

				return result, &RedirectError{
					Servers:         params.ToMachineHostsPorts(redirInfo.Servers),
					CACert:          redirInfo.CACert,
					ControllerTag:   controllerTag,
					ControllerAlias: redirInfo.ControllerAlias,
					FollowRedirect:  false, // user-action required
				}
			}
		}

		// We've been asked to redirect. Find out the redirection info.
		// If the rpc packet allowed us to return arbitrary information in
		// an error, we'd probably put this information in the Login response,
		// but we can't do that currently.
		var resp params.RedirectInfoResult
		if err := st.APICall("Admin", 3, "", "RedirectInfo", nil, &resp); err != nil {
			return result, errors.Annotatef(err, "cannot get redirect addresses")
		}
		return result, &RedirectError{
			Servers:        params.ToMachineHostsPorts(resp.Servers),
			CACert:         resp.CACert,
			FollowRedirect: true, // JAAS-type redirect
		}
	}
	if result.DischargeRequired != nil || result.BakeryDischargeRequired != nil {
		// The result contains a discharge-required
		// macaroon. We discharge it and retry
		// the login request with the original macaroon
		// and its discharges.
		if result.DischargeRequiredReason == "" {
			result.DischargeRequiredReason = "no reason given for discharge requirement"
		}
		// Prefer the newer bakery.v2 macaroon.
		dcMac := result.BakeryDischargeRequired
		if dcMac == nil {
			dcMac, err = bakery.NewLegacyMacaroon(result.DischargeRequired)
			if err != nil {
				return result, errors.Trace(err)
			}
		}
		if err := st.bakeryClient.HandleError(st.ctx, st.cookieURL, &httpbakery.Error{
			Message: result.DischargeRequiredReason,
			Code:    httpbakery.ErrDischargeRequired,
			Info: &httpbakery.ErrorInfo{
				Macaroon:     dcMac,
				MacaroonPath: "/",
			},
		}); err != nil {
			cause := errors.Cause(err)
			if httpbakery.IsInteractionError(cause) {
				// Just inform the user of the reason for the
				// failure, e.g. because the username/password
				// they presented was invalid.
				err = cause.(*httpbakery.InteractionError).Reason
			}
			return result, errors.Trace(err)
		}
		// Add the macaroons that have been saved by HandleError to our login request.
		request.Macaroons = httpbakery.MacaroonsForURL(st.bakeryClient.Client.Jar, st.cookieURL)
		result = params.LoginResult{} // zero result
		err = st.APICall("Admin", 3, "", "Login", request, &result)
		if err != nil {
			return result, errors.Trace(err)
		}
		if result.DischargeRequired != nil {
			return result, errors.Errorf("login with discharged macaroons failed: %s", result.DischargeRequiredReason)
		}
	}
	return result, nil
}

// Login authenticates as the entity with the given name and password
// or macaroons. Subsequent requests on the state will act as that entity.
// This method is usually called automatically by Open. The machine nonce
// should be empty unless logging in as a machine agent.
func (st *state) Login(p LoginParams) error {
	var result params.LoginResult
	var err error

	switch {
	case p.AccessToken == "" && p.ClientID == "" && p.ClientSecret == "" && p.Password == "" && p.Macaroons == nil:
		result, err = st.loginWithDeviceFlow(context.Background(), p.ShowLoginDetails)
		if err != nil {
			if params.IsCodeNotImplemented(err) || params.IsCodeNotSupported(err) {
				result, err = st.loginV3(p)
				if err != nil {
					return errors.Trace(err)
				}
			} else {
				return errors.Trace(err)
			}
		}
	case p.AccessToken != "":
		result, err = st.loginWithAccessToken(context.Background(), p.AccessToken)
		if err != nil {
			return errors.Trace(err)
		}
	case p.ClientID != "" && p.ClientSecret != "":
		result, err = st.loginWithClientCredentials(context.Background(), p.ClientID, p.ClientSecret)
		if err != nil {
			return errors.Trace(err)
		}
	default:
		result, err = st.loginV3(p)
		if err != nil {
			return errors.Trace(err)
		}
	}

	var controllerAccess string
	var modelAccess string
	if result.UserInfo != nil {
		p.Tag, err = names.ParseTag(result.UserInfo.Identity)
		if err != nil {
			return errors.Trace(err)
		}
		controllerAccess = result.UserInfo.ControllerAccess
		modelAccess = result.UserInfo.ModelAccess
	}
	servers := params.ToMachineHostsPorts(result.Servers)
	if err = st.setLoginResult(loginResultParams{
		tag:              p.Tag,
		modelTag:         result.ModelTag,
		controllerTag:    result.ControllerTag,
		servers:          servers,
		publicDNSName:    result.PublicDNSName,
		facades:          result.Facades,
		modelAccess:      modelAccess,
		controllerAccess: controllerAccess,
	}); err != nil {
		return errors.Trace(err)
	}
	st.serverVersion, err = version.Parse(result.ServerVersion)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

type loginResultParams struct {
	tag              names.Tag
	modelTag         string
	controllerTag    string
	modelAccess      string
	controllerAccess string
	servers          []network.MachineHostPorts
	facades          []params.FacadeVersions
	publicDNSName    string
}

func (st *state) setLoginResult(p loginResultParams) error {
	st.authTag = p.tag
	var modelTag names.ModelTag
	if p.modelTag != "" {
		var err error
		modelTag, err = names.ParseModelTag(p.modelTag)
		if err != nil {
			return errors.Annotatef(err, "invalid model tag in login result")
		}
	}
	if modelTag.Id() != st.modelTag.Id() {
		return errors.Errorf("mismatched model tag in login result (got %q want %q)", modelTag.Id(), st.modelTag.Id())
	}
	ctag, err := names.ParseControllerTag(p.controllerTag)
	if err != nil {
		return errors.Annotatef(err, "invalid controller tag %q returned from login", p.controllerTag)
	}
	st.controllerTag = ctag
	st.controllerAccess = p.controllerAccess
	st.modelAccess = p.modelAccess

	hostPorts := p.servers
	// if the connection is not proxied then we will add the connection address
	// to host ports
	if !st.IsProxied() {
		hostPorts, err = addAddress(p.servers, st.addr)
		if err != nil {
			if clerr := st.Close(); clerr != nil {
				err = errors.Annotatef(err, "error closing state: %v", clerr)
			}
			return err
		}
	}
	st.hostPorts = hostPorts

	if err != nil {
		if clerr := st.Close(); clerr != nil {
			err = errors.Annotatef(err, "error closing state: %v", clerr)
		}
		return err
	}
	st.hostPorts = hostPorts

	st.publicDNSName = p.publicDNSName

	st.facadeVersions = make(map[string][]int, len(p.facades))
	for _, facade := range p.facades {
		st.facadeVersions[facade.Name] = facade.Versions
	}

	st.setLoggedIn()
	return nil
}

// AuthTag returns the tag of the authorized user of the state API connection.
func (st *state) AuthTag() names.Tag {
	return st.authTag
}

// ControllerAccess returns the access level of authorized user to the model.
func (st *state) ControllerAccess() string {
	return st.controllerAccess
}

// CookieURL returns the URL that HTTP cookies for the API will be
// associated with.
func (st *state) CookieURL() *url.URL {
	copy := *st.cookieURL
	return &copy
}

// slideAddressToFront moves the address at the location (serverIndex, addrIndex) to be
// the first address of the first server.
func slideAddressToFront(servers []network.MachineHostPorts, serverIndex, addrIndex int) {
	server := servers[serverIndex]
	hostPort := server[addrIndex]
	// Move the matching address to be the first in this server
	for ; addrIndex > 0; addrIndex-- {
		server[addrIndex] = server[addrIndex-1]
	}
	server[0] = hostPort
	for ; serverIndex > 0; serverIndex-- {
		servers[serverIndex] = servers[serverIndex-1]
	}
	servers[0] = server
}

// addAddress appends a new server derived from the given
// address to servers if the address is not already found
// there.
func addAddress(servers []network.MachineHostPorts, addr string) ([]network.MachineHostPorts, error) {
	for i, server := range servers {
		for j, hostPort := range server {
			if network.DialAddress(hostPort) == addr {
				slideAddressToFront(servers, i, j)
				return servers, nil
			}
		}
	}
	host, portString, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(portString)
	if err != nil {
		return nil, err
	}
	result := make([]network.MachineHostPorts, 0, len(servers)+1)
	result = append(result, network.NewMachineHostPorts(port, host))
	result = append(result, servers...)
	return result, nil
}

// KeyUpdater returns access to the KeyUpdater API
func (st *state) KeyUpdater() *keyupdater.State {
	return keyupdater.NewState(st)
}

// ServerVersion holds the version of the API server that we are connected to.
// It is possible that this version is Zero if the server does not report this
// during login. The second result argument indicates if the version number is
// set.
func (st *state) ServerVersion() (version.Number, bool) {
	return st.serverVersion, st.serverVersion != version.Zero
}
