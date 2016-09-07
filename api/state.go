// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"net"
	"strconv"

	"github.com/juju/errors"
	"github.com/juju/version"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/charmrevisionupdater"
	"github.com/juju/juju/api/cleaner"
	"github.com/juju/juju/api/discoverspaces"
	"github.com/juju/juju/api/imagemetadata"
	"github.com/juju/juju/api/instancepoller"
	"github.com/juju/juju/api/keyupdater"
	"github.com/juju/juju/api/reboot"
	"github.com/juju/juju/api/unitassigner"
	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/api/upgrader"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/network"
)

// Login authenticates as the entity with the given name and password
// or macaroons. Subsequent requests on the state will act as that entity.
// This method is usually called automatically by Open. The machine nonce
// should be empty unless logging in as a machine agent.
func (st *state) Login(tag names.Tag, password, nonce string, macaroons []macaroon.Slice) error {
	var result params.LoginResult
	request := &params.LoginRequest{
		AuthTag:     tagToString(tag),
		Credentials: password,
		Nonce:       nonce,
		Macaroons:   macaroons,
	}
	if tag == nil {
		// Add any macaroons from the cookie jar that might work for
		// authenticating the login request.
		request.Macaroons = append(request.Macaroons,
			httpbakery.MacaroonsForURL(st.bakeryClient.Client.Jar, st.cookieURL)...,
		)
	}
	err := st.APICall("Admin", 3, "", "Login", request, &result)
	if err != nil {
		var resp params.RedirectInfoResult
		if params.IsRedirect(err) {
			// We've been asked to redirect. Find out the redirection info.
			// If the rpc packet allowed us to return arbitrary information in
			// an error, we'd probably put this information in the Login response,
			// but we can't do that currently.
			if err := st.APICall("Admin", 3, "", "RedirectInfo", nil, &resp); err != nil {
				return errors.Annotatef(err, "cannot get redirect addresses")
			}
			return &RedirectError{
				Servers: params.NetworkHostsPorts(resp.Servers),
				CACert:  resp.CACert,
			}
		}
		return errors.Trace(err)
	}
	if result.DischargeRequired != nil {
		// The result contains a discharge-required
		// macaroon. We discharge it and retry
		// the login request with the original macaroon
		// and its discharges.
		if result.DischargeRequiredReason == "" {
			result.DischargeRequiredReason = "no reason given for discharge requirement"
		}
		if err := st.bakeryClient.HandleError(st.cookieURL, &httpbakery.Error{
			Message: result.DischargeRequiredReason,
			Code:    httpbakery.ErrDischargeRequired,
			Info: &httpbakery.ErrorInfo{
				Macaroon:     result.DischargeRequired,
				MacaroonPath: "/",
			},
		}); err != nil {
			return errors.Trace(err)
		}
		// Add the macaroons that have been saved by HandleError to our login request.
		request.Macaroons = httpbakery.MacaroonsForURL(st.bakeryClient.Client.Jar, st.cookieURL)
		result = params.LoginResult{} // zero result
		err = st.APICall("Admin", 3, "", "Login", request, &result)
		if err != nil {
			return errors.Trace(err)
		}
		if result.DischargeRequired != nil {
			return errors.Errorf("login with discharged macaroons failed: %s", result.DischargeRequiredReason)
		}
	}

	var controllerAccess string
	var modelAccess string
	if result.UserInfo != nil {
		tag, err = names.ParseTag(result.UserInfo.Identity)
		if err != nil {
			return errors.Trace(err)
		}
		controllerAccess = result.UserInfo.ControllerAccess
		modelAccess = result.UserInfo.ModelAccess
	}
	servers := params.NetworkHostsPorts(result.Servers)
	if err = st.setLoginResult(loginResultParams{
		tag:              tag,
		modelTag:         result.ModelTag,
		controllerTag:    result.ControllerTag,
		servers:          servers,
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
	servers          [][]network.HostPort
	facades          []params.FacadeVersions
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

	hostPorts, err := addAddress(p.servers, st.addr)
	if err != nil {
		if clerr := st.Close(); clerr != nil {
			err = errors.Annotatef(err, "error closing state: %v", clerr)
		}
		return err
	}
	st.hostPorts = hostPorts

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

// ModelAccess returns the access level of authorized user to the model.
func (st *state) ModelAccess() string {
	return st.modelAccess
}

// ControllerAccess returns the access level of authorized user to the model.
func (st *state) ControllerAccess() string {
	return st.controllerAccess
}

// slideAddressToFront moves the address at the location (serverIndex, addrIndex) to be
// the first address of the first server.
func slideAddressToFront(servers [][]network.HostPort, serverIndex, addrIndex int) {
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
func addAddress(servers [][]network.HostPort, addr string) ([][]network.HostPort, error) {
	for i, server := range servers {
		for j, hostPort := range server {
			if hostPort.NetAddr() == addr {
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
	result := make([][]network.HostPort, 0, len(servers)+1)
	result = append(result, network.NewHostPorts(port, host))
	result = append(result, servers...)
	return result, nil
}

// Client returns an object that can be used
// to access client-specific functionality.
func (st *state) Client() *Client {
	frontend, backend := base.NewClientFacade(st, "Client")
	return &Client{ClientFacade: frontend, facade: backend, st: st}
}

// UnitAssigner returns a version of the state that provides functionality
// required by the unitassigner worker.
func (st *state) UnitAssigner() unitassigner.API {
	return unitassigner.New(st)
}

// Uniter returns a version of the state that provides functionality
// required by the uniter worker.
func (st *state) Uniter() (*uniter.State, error) {
	unitTag, ok := st.authTag.(names.UnitTag)
	if !ok {
		return nil, errors.Errorf("expected UnitTag, got %T %v", st.authTag, st.authTag)
	}
	return uniter.NewState(st, unitTag), nil
}

// Upgrader returns access to the Upgrader API
func (st *state) Upgrader() *upgrader.State {
	return upgrader.NewState(st)
}

// Reboot returns access to the Reboot API
func (st *state) Reboot() (reboot.State, error) {
	switch tag := st.authTag.(type) {
	case names.MachineTag:
		return reboot.NewState(st, tag), nil
	default:
		return nil, errors.Errorf("expected names.MachineTag, got %T", tag)
	}
}

// DiscoverSpaces returns access to the DiscoverSpacesAPI.
func (st *state) DiscoverSpaces() *discoverspaces.API {
	return discoverspaces.NewAPI(st)
}

// KeyUpdater returns access to the KeyUpdater API
func (st *state) KeyUpdater() *keyupdater.State {
	return keyupdater.NewState(st)
}

// InstancePoller returns access to the InstancePoller API
func (st *state) InstancePoller() *instancepoller.API {
	return instancepoller.NewAPI(st)
}

// CharmRevisionUpdater returns access to the CharmRevisionUpdater API
func (st *state) CharmRevisionUpdater() *charmrevisionupdater.State {
	return charmrevisionupdater.NewState(st)
}

// Cleaner returns a version of the state that provides access to the cleaner API
func (st *state) Cleaner() *cleaner.API {
	return cleaner.NewAPI(st)
}

// ServerVersion holds the version of the API server that we are connected to.
// It is possible that this version is Zero if the server does not report this
// during login. The second result argument indicates if the version number is
// set.
func (st *state) ServerVersion() (version.Number, bool) {
	return st.serverVersion, st.serverVersion != version.Zero
}

// MetadataUpdater returns access to the imageMetadata API
func (st *state) MetadataUpdater() *imagemetadata.Client {
	return imagemetadata.NewClient(st)
}
