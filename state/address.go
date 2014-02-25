// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"launchpad.net/juju-core/instance"
)

// address represents the location of a machine, including metadata about what
// kind of location the address describes.
type address struct {
	Value        string
	AddressType  instance.AddressType
	NetworkName  string                `bson:",omitempty"`
	NetworkScope instance.NetworkScope `bson:",omitempty"`
}

func newAddress(addr instance.Address) address {
	stateaddr := address{
		Value:        addr.Value,
		AddressType:  addr.Type,
		NetworkName:  addr.NetworkName,
		NetworkScope: addr.NetworkScope,
	}
	return stateaddr
}

func (addr *address) InstanceAddress() instance.Address {
	instanceaddr := instance.Address{
		Value:        addr.Value,
		Type:         addr.AddressType,
		NetworkName:  addr.NetworkName,
		NetworkScope: addr.NetworkScope,
	}
	return instanceaddr
}

func addressesToInstanceAddresses(addrs []address) []instance.Address {
	instanceAddrs := make([]instance.Address, len(addrs))
	for i, addr := range addrs {
		instanceAddrs[i] = addr.InstanceAddress()
	}
	return instanceAddrs
}

func instanceAddressesToAddresses(instanceAddrs []instance.Address) []address {
	addrs := make([]address, len(instanceAddrs))
	for i, addr := range instanceAddrs {
		addrs[i] = newAddress(addr)
	}
	return addrs
}

// stateServerAddresses returns the list of internal addresses of the state
// server machines.
func (st *State) stateServerAddresses() ([]string, error) {
	type addressMachine struct {
		Addresses []address
	}
	var allAddresses []addressMachine
	// TODO(rog) 2013/10/14 index machines on jobs.
	err := st.machines.Find(D{{"jobs", JobManageEnviron}}).All(&allAddresses)
	if err != nil {
		return nil, err
	}
	if len(allAddresses) == 0 {
		return nil, fmt.Errorf("no state server machines found")
	}
	apiAddrs := make([]string, 0, len(allAddresses))
	for _, addrs := range allAddresses {
		instAddrs := addressesToInstanceAddresses(addrs.Addresses)
		addr := instance.SelectInternalAddress(instAddrs, false)
		if addr != "" {
			apiAddrs = append(apiAddrs, addr)
		}
	}
	if len(apiAddrs) == 0 {
		return nil, fmt.Errorf("no state server machines with addresses found")
	}
	return apiAddrs, nil
}

func appendPort(addrs []string, port int) []string {
	newAddrs := make([]string, len(addrs))
	for i, addr := range addrs {
		newAddrs[i] = fmt.Sprintf("%s:%d", addr, port)
	}
	return newAddrs
}

// Addresses returns the list of cloud-internal addresses that
// can be used to connect to the state.
func (st *State) Addresses() ([]string, error) {
	addrs, err := st.stateServerAddresses()
	if err != nil {
		return nil, err
	}
	config, err := st.EnvironConfig()
	if err != nil {
		return nil, err
	}
	return appendPort(addrs, config.StatePort()), nil
}

// APIAddresses returns the list of cloud-internal addresses that
// can be used to connect to the state API server.
func (st *State) APIAddresses() ([]string, error) {
	addrs, err := st.stateServerAddresses()
	if err != nil {
		return nil, err
	}
	config, err := st.EnvironConfig()
	if err != nil {
		return nil, err
	}
	return appendPort(addrs, config.APIPort()), nil
}

type DeployerConnectionValues struct {
	StateAddresses []string
	APIAddresses   []string
}

// DeployerConnectionInfo returns the address information necessary for the deployer.
// The function does the expensive operations (getting stuff from mongo) just once.
func (st *State) DeployerConnectionInfo() (*DeployerConnectionValues, error) {
	addrs, err := st.stateServerAddresses()
	if err != nil {
		return nil, err
	}
	config, err := st.EnvironConfig()
	if err != nil {
		return nil, err
	}
	return &DeployerConnectionValues{
		StateAddresses: appendPort(addrs, config.StatePort()),
		APIAddresses:   appendPort(addrs, config.APIPort()),
	}, nil
}
