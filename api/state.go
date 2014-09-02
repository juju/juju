// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"net"
	"strconv"

	"github.com/juju/names"

	"github.com/juju/juju/api/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/charmrevisionupdater"
	"github.com/juju/juju/api/deployer"
	"github.com/juju/juju/api/environment"
	"github.com/juju/juju/api/firewaller"
	"github.com/juju/juju/api/keyupdater"
	apilogger "github.com/juju/juju/api/logger"
	"github.com/juju/juju/api/machiner"
	"github.com/juju/juju/api/networker"
	"github.com/juju/juju/api/provisioner"
	"github.com/juju/juju/api/rsyslog"
	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/api/upgrader"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/network"
)

// Login authenticates as the entity with the given name and password.
// Subsequent requests on the state will act as that entity.  This
// method is usually called automatically by Open. The machine nonce
// should be empty unless logging in as a machine agent.
func (st *State) Login(tag, password, nonce string) error {
	var result params.LoginResult
	err := st.APICall("Admin", 0, "", "Login", &params.Creds{
		AuthTag:  tag,
		Password: password,
		Nonce:    nonce,
	}, &result)
	if err == nil {
		authtag, err := names.ParseTag(tag)
		if err != nil {
			return err
		}
		st.authTag = authtag
		hostPorts, err := addAddress(result.Servers, st.addr)
		if err != nil {
			st.Close()
			return err
		}
		st.hostPorts = hostPorts
		st.environTag = result.EnvironTag
		st.facadeVersions = make(map[string][]int, len(result.Facades))
		for _, facade := range result.Facades {
			st.facadeVersions[facade.Name] = facade.Versions
		}
	}
	return err
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
	hostPort := network.HostPort{
		Address: network.NewAddress(host, network.ScopeUnknown),
		Port:    port,
	}
	result := make([][]network.HostPort, 0, len(servers)+1)
	result = append(result, []network.HostPort{hostPort})
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

// Networker returns a version of the state that provides functionality
// required by the networker worker.
func (st *State) Networker() *networker.State {
	return networker.NewState(st)
}

// Provisioner returns a version of the state that provides functionality
// required by the provisioner worker.
func (st *State) Provisioner() *provisioner.State {
	return provisioner.NewState(st)
}

// Uniter returns a version of the state that provides functionality
// required by the uniter worker.
func (st *State) Uniter() *uniter.State {
	// TODO(dfc) yes, this can panic, we never checked before
	return uniter.NewState(st, st.authTag.(names.UnitTag))
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

// Deployer returns access to the Deployer API
func (st *State) Deployer() *deployer.State {
	return deployer.NewState(st)
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

// CharmRevisionUpdater returns access to the CharmRevisionUpdater API
func (st *State) CharmRevisionUpdater() *charmrevisionupdater.State {
	return charmrevisionupdater.NewState(st)
}

// Rsyslog returns access to the Rsyslog API
func (st *State) Rsyslog() *rsyslog.State {
	return rsyslog.NewState(st)
}
