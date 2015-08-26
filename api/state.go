// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"net"
	"strconv"

	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/api/addresser"
	"github.com/juju/juju/api/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/charmrevisionupdater"
	"github.com/juju/juju/api/cleaner"
	"github.com/juju/juju/api/deployer"
	"github.com/juju/juju/api/diskmanager"
	"github.com/juju/juju/api/environment"
	"github.com/juju/juju/api/firewaller"
	"github.com/juju/juju/api/instancepoller"
	"github.com/juju/juju/api/keyupdater"
	apilogger "github.com/juju/juju/api/logger"
	"github.com/juju/juju/api/machiner"
	"github.com/juju/juju/api/networker"
	"github.com/juju/juju/api/provisioner"
	"github.com/juju/juju/api/reboot"
	"github.com/juju/juju/api/resumer"
	"github.com/juju/juju/api/rsyslog"
	"github.com/juju/juju/api/storageprovisioner"
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
func (st *State) Login(tag, password, nonce string) error {
	err := st.loginV2(tag, password, nonce)
	if params.IsCodeNotImplemented(err) {
		err = st.loginV1(tag, password, nonce)
		if params.IsCodeNotImplemented(err) {
			// TODO (cmars): remove fallback once we can drop v0 compatibility
			return st.loginV0(tag, password, nonce)
		}
	}
	return err
}

func (st *State) loginV2(tag, password, nonce string) error {
	var result params.LoginResultV1
	request := &params.LoginRequest{
		AuthTag:     tag,
		Credentials: password,
		Nonce:       nonce,
	}
	err := st.APICall("Admin", 2, "", "Login", request, &result)
	if err != nil {
		// If the server complains about an empty tag it may be that we are
		// talking to an older server version that does not understand facades and
		// expects a params.Creds request instead of a params.LoginRequest. We
		// return a CodNotImplemented error to force login down to V1, which
		// supports older server logins. This may mask an actual empty tag in
		// params.LoginRequest, but that would be picked up in loginV1. V1 will
		// also produce a warning that we are ignoring an invalid API, so we do not
		// need to add one here.
		if err.Error() == `"" is not a valid tag` {
			return &params.Error{
				Message: err.Error(),
				Code:    params.CodeNotImplemented,
			}
		}
		return errors.Trace(err)
	}
	servers := params.NetworkHostsPorts(result.Servers)
	err = st.setLoginResult(tag, result.EnvironTag, result.ServerTag, servers, result.Facades)
	if err != nil {
		return errors.Trace(err)
	}
	st.serverVersion, err = version.Parse(result.ServerVersion)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (st *State) loginV1(tag, password, nonce string) error {
	var result struct {
		// TODO (cmars): remove once we can drop 1.18 login compatibility
		params.LoginResult

		params.LoginResultV1
	}
	err := st.APICall("Admin", 1, "", "Login", &params.LoginRequestCompat{
		LoginRequest: params.LoginRequest{
			AuthTag:     tag,
			Credentials: password,
			Nonce:       nonce,
		},
		// TODO (cmars): remove once we can drop 1.18 login compatibility
		Creds: params.Creds{
			AuthTag:  tag,
			Password: password,
			Nonce:    nonce,
		},
	}, &result)
	if err != nil {
		return err
	}

	// We've either logged into an Admin v1 facade, or a pre-facade (1.18) API
	// server.  The JSON field names between the structures are disjoint, so only
	// one should have an environ tag set.

	var environTag string
	var serverTag string
	var servers [][]network.HostPort
	var facades []params.FacadeVersions
	// For quite old servers, it is possible that they don't send down
	// the environTag.
	if result.LoginResult.EnvironTag != "" {
		environTag = result.LoginResult.EnvironTag
		// If the server doesn't support login v1, it doesn't support
		// multiple environments, so don't store a server tag.
		servers = params.NetworkHostsPorts(result.LoginResult.Servers)
		facades = result.LoginResult.Facades
	} else if result.LoginResultV1.EnvironTag != "" {
		environTag = result.LoginResultV1.EnvironTag
		serverTag = result.LoginResultV1.ServerTag
		servers = params.NetworkHostsPorts(result.LoginResultV1.Servers)
		facades = result.LoginResultV1.Facades
	}

	err = st.setLoginResult(tag, environTag, serverTag, servers, facades)
	if err != nil {
		return err
	}
	return nil
}

func (st *State) setLoginResult(tag, environTag, serverTag string, servers [][]network.HostPort, facades []params.FacadeVersions) error {
	authtag, err := names.ParseTag(tag)
	if err != nil {
		return err
	}
	st.authTag = authtag
	st.environTag = environTag
	st.serverTag = serverTag

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
	return nil
}

func (st *State) loginV0(tag, password, nonce string) error {
	var result params.LoginResult
	err := st.APICall("Admin", 0, "", "Login", &params.Creds{
		AuthTag:  tag,
		Password: password,
		Nonce:    nonce,
	}, &result)
	if err != nil {
		return err
	}
	servers := params.NetworkHostsPorts(result.Servers)
	// Don't set a server tag.
	if err = st.setLoginResult(tag, result.EnvironTag, "", servers, result.Facades); err != nil {
		return err
	}
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
func (st *State) Client() *Client {
	frontend, backend := base.NewClientFacade(st, "Client")
	return &Client{ClientFacade: frontend, facade: backend, st: st}
}

// Machiner returns a version of the state that provides functionality
// required by the machiner worker.
func (st *State) Machiner() *machiner.State {
	return machiner.NewState(st)
}

// Resumer returns a version of the state that provides functionality
// required by the resumer worker.
func (st *State) Resumer() *resumer.API {
	return resumer.NewAPI(st)
}

// Networker returns a version of the state that provides functionality
// required by the networker worker.
func (st *State) Networker() networker.State {
	return networker.NewState(st)
}

// Provisioner returns a version of the state that provides functionality
// required by the provisioner worker.
func (st *State) Provisioner() *provisioner.State {
	return provisioner.NewState(st)
}

// Uniter returns a version of the state that provides functionality
// required by the uniter worker.
func (st *State) Uniter() (*uniter.State, error) {
	unitTag, ok := st.authTag.(names.UnitTag)
	if !ok {
		return nil, errors.Errorf("expected UnitTag, got %T %v", st.authTag, st.authTag)
	}
	return uniter.NewState(st, unitTag), nil
}

// DiskManager returns a version of the state that provides functionality
// required by the diskmanager worker.
func (st *State) DiskManager() (*diskmanager.State, error) {
	machineTag, ok := st.authTag.(names.MachineTag)
	if !ok {
		return nil, errors.Errorf("expected MachineTag, got %#v", st.authTag)
	}
	return diskmanager.NewState(st, machineTag), nil
}

// StorageProvisioner returns a version of the state that provides
// functionality required by the storageprovisioner worker.
// The scope tag defines the type of storage that is provisioned, either
// either attached directly to a specified machine (machine scoped),
// or provisioned on the underlying cloud for use by any machine in a
// specified environment (environ scoped).
func (st *State) StorageProvisioner(scope names.Tag) *storageprovisioner.State {
	return storageprovisioner.NewState(st, scope)
}

// Firewaller returns a version of the state that provides functionality
// required by the firewaller worker.
func (st *State) Firewaller() *firewaller.State {
	return firewaller.NewState(st)
}

// Agent returns a version of the state that provides
// functionality required by the agent code.
func (st *State) Agent() *agent.State {
	return agent.NewState(st)
}

// Upgrader returns access to the Upgrader API
func (st *State) Upgrader() *upgrader.State {
	return upgrader.NewState(st)
}

// Reboot returns access to the Reboot API
func (st *State) Reboot() (*reboot.State, error) {
	switch tag := st.authTag.(type) {
	case names.MachineTag:
		return reboot.NewState(st, tag), nil
	default:
		return nil, errors.Errorf("expected names.MachineTag, got %T", tag)
	}
}

// Deployer returns access to the Deployer API
func (st *State) Deployer() *deployer.State {
	return deployer.NewState(st)
}

// Addresser returns access to the Addresser API.
func (st *State) Addresser() *addresser.API {
	return addresser.NewAPI(st)
}

// Environment returns access to the Environment API
func (st *State) Environment() *environment.Facade {
	return environment.NewFacade(st)
}

// Logger returns access to the Logger API
func (st *State) Logger() *apilogger.State {
	return apilogger.NewState(st)
}

// KeyUpdater returns access to the KeyUpdater API
func (st *State) KeyUpdater() *keyupdater.State {
	return keyupdater.NewState(st)
}

// InstancePoller returns access to the InstancePoller API
func (st *State) InstancePoller() *instancepoller.API {
	return instancepoller.NewAPI(st)
}

// CharmRevisionUpdater returns access to the CharmRevisionUpdater API
func (st *State) CharmRevisionUpdater() *charmrevisionupdater.State {
	return charmrevisionupdater.NewState(st)
}

// Cleaner returns a version of the state that provides access to the cleaner API
func (st *State) Cleaner() *cleaner.API {
	return cleaner.NewAPI(st)
}

// Rsyslog returns access to the Rsyslog API
func (st *State) Rsyslog() *rsyslog.State {
	return rsyslog.NewState(st)
}

// ServerVersion holds the version of the API server that we are connected to.
// It is possible that this version is Zero if the server does not report this
// during login. The second result argument indicates if the version number is
// set.
func (st *State) ServerVersion() (version.Number, bool) {
	return st.serverVersion, st.serverVersion != version.Zero
}
