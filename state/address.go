// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"github.com/juju/errors"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/network"
)

// stateServerAddresses returns the list of internal addresses of the state
// server machines.
func (st *State) stateServerAddresses() ([]string, error) {
	type addressMachine struct {
		Addresses []address
	}
	var allAddresses []addressMachine
	// TODO(rog) 2013/10/14 index machines on jobs.
	machines, closer := st.getCollection(machinesC)
	defer closer()
	err := machines.Find(bson.D{{"jobs", JobManageEnviron}}).All(&allAddresses)
	if err != nil {
		return nil, err
	}
	if len(allAddresses) == 0 {
		return nil, fmt.Errorf("no state server machines found")
	}
	apiAddrs := make([]string, 0, len(allAddresses))
	for _, addrs := range allAddresses {
		instAddrs := addressesToInstanceAddresses(addrs.Addresses)
		addr := network.SelectInternalAddress(instAddrs, false)
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

// SetAPIHostPorts sets the addresses of the API server instances.
// Each server is represented by one element in the top level slice.
func (st *State) SetAPIHostPorts(hps [][]network.HostPort) error {
	doc := apiHostPortsDoc{
		APIHostPorts: instanceHostPortsToHostPorts(hps),
	}
	buildTxn := func(attempt int) ([]txn.Op, error) {
		existing, err := st.APIHostPorts()
		if err != nil {
			return nil, err
		}
		op := txn.Op{
			C:  stateServersC,
			Id: apiHostPortsKey,
			Assert: bson.D{{
				"apihostports", instanceHostPortsToHostPorts(existing),
			}},
		}
		if !hostPortsEqual(hps, existing) {
			op.Update = bson.D{{
				"$set", bson.D{{"apihostports", doc.APIHostPorts}},
			}}
		}
		return []txn.Op{op}, nil
	}
	if err := st.run(buildTxn); err != nil {
		return errors.Annotate(err, "cannot set API addresses")
	}
	return nil
}

// APIHostPorts returns the API addresses as set by SetAPIHostPorts.
func (st *State) APIHostPorts() ([][]network.HostPort, error) {
	var doc apiHostPortsDoc
	stateServers, closer := st.getCollection(stateServersC)
	defer closer()
	err := stateServers.Find(bson.D{{"_id", apiHostPortsKey}}).One(&doc)
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
	Value       string
	AddressType network.AddressType
	NetworkName string        `bson:",omitempty"`
	Scope       network.Scope `bson:"networkscope,omitempty"`
}

// TODO(dimitern) Make sure we integrate this with other networking
// stuff at some point. We want to use juju-specific network names
// that point to existing documents in the networks collection.
type hostPort struct {
	Value       string
	AddressType network.AddressType
	NetworkName string        `bson:",omitempty"`
	Scope       network.Scope `bson:"networkscope,omitempty"`
	Port        int
}

func newAddress(addr network.Address) address {
	return address{
		Value:       addr.Value,
		AddressType: addr.Type,
		NetworkName: addr.NetworkName,
		Scope:       addr.Scope,
	}
}

func (addr *address) InstanceAddress() network.Address {
	return network.Address{
		Value:       addr.Value,
		Type:        addr.AddressType,
		NetworkName: addr.NetworkName,
		Scope:       addr.Scope,
	}
}

func newHostPort(hp network.HostPort) hostPort {
	return hostPort{
		Value:       hp.Value,
		AddressType: hp.Type,
		NetworkName: hp.NetworkName,
		Scope:       hp.Scope,
		Port:        hp.Port,
	}
}

func (hp *hostPort) InstanceHostPort() network.HostPort {
	return network.HostPort{
		Address: network.Address{
			Value:       hp.Value,
			Type:        hp.AddressType,
			NetworkName: hp.NetworkName,
			Scope:       hp.Scope,
		},
		Port: hp.Port,
	}
}

func addressesToInstanceAddresses(addrs []address) []network.Address {
	instanceAddrs := make([]network.Address, len(addrs))
	for i, addr := range addrs {
		instanceAddrs[i] = addr.InstanceAddress()
	}
	return instanceAddrs
}

func instanceAddressesToAddresses(instanceAddrs []network.Address) []address {
	addrs := make([]address, len(instanceAddrs))
	for i, addr := range instanceAddrs {
		addrs[i] = newAddress(addr)
	}
	return addrs
}

func hostPortsToInstanceHostPorts(insts [][]hostPort) [][]network.HostPort {
	instanceHostPorts := make([][]network.HostPort, len(insts))
	for i, hps := range insts {
		instanceHostPorts[i] = make([]network.HostPort, len(hps))
		for j, hp := range hps {
			instanceHostPorts[i][j] = hp.InstanceHostPort()
		}
	}
	return instanceHostPorts
}

func instanceHostPortsToHostPorts(instanceHostPorts [][]network.HostPort) [][]hostPort {
	hps := make([][]hostPort, len(instanceHostPorts))
	for i, instanceHps := range instanceHostPorts {
		hps[i] = make([]hostPort, len(instanceHps))
		for j, hp := range instanceHps {
			hps[i][j] = newHostPort(hp)
		}
	}
	return hps
}

func addressesEqual(a, b []network.Address) bool {
	if len(a) != len(b) {
		return false
	}
	for i, addrA := range a {
		if addrA != b[i] {
			return false
		}
	}
	return true
}

func hostPortsEqual(a, b [][]network.HostPort) bool {
	if len(a) != len(b) {
		return false
	}
	for i, hpA := range a {
		if len(hpA) != len(b[i]) {
			return false
		}
		for j := range hpA {
			if hpA[j] != b[i][j] {
				return false
			}
		}
	}
	return true
}
