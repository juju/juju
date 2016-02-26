// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"net"
	"strconv"

	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/macaroon-bakery.v1/httpbakery"

	"github.com/juju/juju/api/addresser"
	"github.com/juju/juju/api/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/charmrevisionupdater"
	"github.com/juju/juju/api/cleaner"
	"github.com/juju/juju/api/deployer"
	"github.com/juju/juju/api/discoverspaces"
	"github.com/juju/juju/api/firewaller"
	"github.com/juju/juju/api/imagemetadata"
	"github.com/juju/juju/api/instancepoller"
	"github.com/juju/juju/api/keyupdater"
	"github.com/juju/juju/api/machiner"
	"github.com/juju/juju/api/provisioner"
	"github.com/juju/juju/api/reboot"
	"github.com/juju/juju/api/unitassigner"
	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/api/upgrader"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/network"
	"github.com/juju/juju/version"
)

// Login authenticates as the entity with the given name and password.
// Subsequent requests on the state will act as that entity.  This
// method is usually called automatically by Open. The machine nonce
// should be empty unless logging in as a machine agent.
func (st *state) Login(tag names.Tag, password, nonce string) error {
	err := st.loginV3(tag, password, nonce)
	return errors.Trace(err)
}

// loginV2 is retained for testing logins from older clients.
func (st *state) loginV2(tag names.Tag, password, nonce string) error {
	return st.loginForVersion(tag, password, nonce, 2)
}

func (st *state) loginV3(tag names.Tag, password, nonce string) error {
	return st.loginForVersion(tag, password, nonce, 3)
}

func (st *state) loginForVersion(tag names.Tag, password, nonce string, vers int) error {
	var result params.LoginResultV1
	request := &params.LoginRequest{
		AuthTag:     tagToString(tag),
		Credentials: password,
		Nonce:       nonce,
	}
	if tag == nil {
		// Add any macaroons that might work for authenticating the login request.
		request.Macaroons = httpbakery.MacaroonsForURL(st.bakeryClient.Client.Jar, st.cookieURL)
	}
	err := st.APICall("Admin", vers, "", "Login", request, &result)
	if err != nil {
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
		result = params.LoginResultV1{} // zero result
		err = st.APICall("Admin", vers, "", "Login", request, &result)
		if err != nil {
			return errors.Trace(err)
		}
		if result.DischargeRequired != nil {
			return errors.Errorf("login with discharged macaroons failed: %s", result.DischargeRequiredReason)
		}
	}

	servers := params.NetworkHostsPorts(result.Servers)
	err = st.setLoginResult(tag, result.ModelTag, result.ControllerTag, servers, result.Facades)
	if err != nil {
		return errors.Trace(err)
	}
	st.serverVersion, err = version.Parse(result.ServerVersion)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (st *state) setLoginResult(tag names.Tag, modelTag, controllerTag string, servers [][]network.HostPort, facades []params.FacadeVersions) error {
	st.authTag = tag
	st.modelTag = modelTag
	st.controllerTag = controllerTag

	hostPorts, err := addAddress(servers, st.addr)
	if err != nil {
		if clerr := st.Close(); clerr != nil {
			err = errors.Annotatef(err, "error closing state: %v", clerr)
		}
		return err
	}
	st.hostPorts = hostPorts

	st.facadeVersions = make(map[string][]int, len(facades))
	for _, facade := range facades {
		st.facadeVersions[facade.Name] = facade.Versions
	}

	st.setLoggedIn()
	return nil
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

// Machiner returns a version of the state that provides functionality
// required by the machiner worker.
func (st *state) Machiner() *machiner.State {
	return machiner.NewState(st)
}

// UnitAssigner returns a version of the state that provides functionality
// required by the unitassigner worker.
func (st *state) UnitAssigner() unitassigner.API {
	return unitassigner.New(st)
}

// Provisioner returns a version of the state that provides functionality
// required by the provisioner worker.
func (st *state) Provisioner() *provisioner.State {
	return provisioner.NewState(st)
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

// Firewaller returns a version of the state that provides functionality
// required by the firewaller worker.
func (st *state) Firewaller() *firewaller.State {
	return firewaller.NewState(st)
}

// Agent returns a version of the state that provides
// functionality required by the agent code.
func (st *state) Agent() *agent.State {
	return agent.NewState(st)
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

// Deployer returns access to the Deployer API
func (st *state) Deployer() *deployer.State {
	return deployer.NewState(st)
}

// Addresser returns access to the Addresser API.
func (st *state) Addresser() *addresser.API {
	return addresser.NewAPI(st)
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
