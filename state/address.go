// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"labix.org/v2/mgo/bson"
	"labix.org/v2/mgo/txn"

	"launchpad.net/juju-core/instance"
)

// stateServerAddresses returns the list of internal addresses of the state
// server machines.
func (st *State) stateServerAddresses() ([]string, error) {
	type addressMachine struct {
		Addresses []address
	}
	var allAddresses []addressMachine
	// TODO(rog) 2013/10/14 index machines on jobs.
	err := st.machines.Find(bson.D{{"jobs", JobManageEnviron}}).All(&allAddresses)
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

// APIAddressesFromMachines returns the list of cloud-internal addresses that
// can be used to connect to the state API server.
// This method will be deprecated when API addresses are
// stored independently in their own document.
func (st *State) APIAddressesFromMachines() ([]string, error) {
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

const apiHostPortsKey = "apiHostPorts"

type apiHostPortsDoc struct {
	APIHostPorts [][]hostPort
}

// SetAPIHostPorts sets the addresses of the API server
// instances. Each server is represented by one element
// in the top level slice.
func (st *State) SetAPIHostPorts(hps [][]instance.HostPort) error {
	doc := apiHostPortsDoc{
		APIHostPorts: instanceHostPortsToHostPorts(hps),
	}
	// We need to insert the document if it does not already
	// exist to make this method work even on old environments
	// where the document was not created by Initialize.
	ops := []txn.Op{{
		C:  st.stateServers.Name,
		Id: apiHostPortsKey,
		Update: bson.D{{"$set", bson.D{
			{"apihostports", doc.APIHostPorts},
		}}},
	}}
	if err := st.runTransaction(ops); err != nil {
		return fmt.Errorf("cannot set API addresses: %v", err)
	}
	return nil
}

// APIHostPorts returns the API addresses as set by SetAPIHostPorts.
func (st *State) APIHostPorts() ([][]instance.HostPort, error) {
	var doc apiHostPortsDoc
	err := st.stateServers.Find(bson.D{{"_id", apiHostPortsKey}}).One(&doc)
	if err != nil {
		return nil, err
	}
	return hostPortsToInstanceHostPorts(doc.APIHostPorts), nil
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

// address represents the location of a machine, including metadata
// about what kind of location the address describes.
//
// TODO(dimitern) Make sure we integrate this with other networking
// stuff at some point. We want to use juju-specific network names
// that point to existing documents in the networks collection.
type address struct {
	Value        string
	AddressType  instance.AddressType
	NetworkName  string                `bson:",omitempty"`
	NetworkScope instance.NetworkScope `bson:",omitempty"`
}

// TODO(dimitern) Make sure we integrate this with other networking
// stuff at some point. We want to use juju-specific network names
// that point to existing documents in the networks collection.
type hostPort struct {
	Value        string
	AddressType  instance.AddressType
	NetworkName  string                `bson:",omitempty"`
	NetworkScope instance.NetworkScope `bson:",omitempty"`
	Port         int
}

func newAddress(addr instance.Address) address {
	return address{
		Value:        addr.Value,
		AddressType:  addr.Type,
		NetworkName:  addr.NetworkName,
		NetworkScope: addr.NetworkScope,
	}
}

func (addr *address) InstanceAddress() instance.Address {
	return instance.Address{
		Value:        addr.Value,
		Type:         addr.AddressType,
		NetworkName:  addr.NetworkName,
		NetworkScope: addr.NetworkScope,
	}
}

func newHostPort(hp instance.HostPort) hostPort {
	return hostPort{
		Value:        hp.Value,
		AddressType:  hp.Type,
		NetworkName:  hp.NetworkName,
		NetworkScope: hp.NetworkScope,
		Port:         hp.Port,
	}
}

func (hp *hostPort) InstanceHostPort() instance.HostPort {
	return instance.HostPort{
		Address: instance.Address{
			Value:        hp.Value,
			Type:         hp.AddressType,
			NetworkName:  hp.NetworkName,
			NetworkScope: hp.NetworkScope,
		},
		Port: hp.Port,
	}
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

func hostPortsToInstanceHostPorts(insts [][]hostPort) [][]instance.HostPort {
	instanceHostPorts := make([][]instance.HostPort, len(insts))
	for i, hps := range insts {
		instanceHostPorts[i] = make([]instance.HostPort, len(hps))
		for j, hp := range hps {
			instanceHostPorts[i][j] = hp.InstanceHostPort()
		}
	}
	return instanceHostPorts
}

func instanceHostPortsToHostPorts(instanceHostPorts [][]instance.HostPort) [][]hostPort {
	hps := make([][]hostPort, len(instanceHostPorts))
	for i, instanceHps := range instanceHostPorts {
		hps[i] = make([]hostPort, len(instanceHps))
		for j, hp := range instanceHps {
			hps[i][j] = newHostPort(hp)
		}
	}
	return hps
}
