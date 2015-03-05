// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"reflect"

	"github.com/juju/errors"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/network"
)

// stateServerAddresses returns the list of internal addresses of the state
// server machines.
func (st *State) stateServerAddresses() ([]string, error) {
	ssState := st
	env, err := st.StateServerEnvironment()
	if err != nil {
		return nil, errors.Trace(err)
	}
	if st.EnvironTag() != env.EnvironTag() {
		// We are not using the state server environment, so get one.
		logger.Debugf("getting a state server state connection, current env: %s", st.EnvironTag())
		ssState, err = st.ForEnviron(env.EnvironTag())
		if err != nil {
			return nil, errors.Trace(err)
		}
		defer ssState.Close()
		logger.Debugf("ssState env: %s", ssState.EnvironTag())
	}

	type addressMachine struct {
		Addresses []address
	}
	var allAddresses []addressMachine
	// TODO(rog) 2013/10/14 index machines on jobs.
	machines, closer := ssState.getCollection(machinesC)
	defer closer()
	err = machines.Find(bson.D{{"jobs", JobManageEnviron}}).All(&allAddresses)
	if err != nil {
		return nil, err
	}
	if len(allAddresses) == 0 {
		return nil, errors.New("no state server machines found")
	}
	apiAddrs := make([]string, 0, len(allAddresses))
	for _, addrs := range allAddresses {
		naddrs := networkAddresses(addrs.Addresses)
		addr := network.SelectInternalAddress(naddrs, false)
		if addr != "" {
			apiAddrs = append(apiAddrs, addr)
		}
	}
	if len(apiAddrs) == 0 {
		return nil, errors.New("no state server machines with addresses found")
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
		return nil, errors.Trace(err)
	}
	config, err := st.EnvironConfig()
	if err != nil {
		return nil, errors.Trace(err)
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
		return nil, errors.Trace(err)
	}
	config, err := st.EnvironConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return appendPort(addrs, config.APIPort()), nil
}

const apiHostPortsKey = "apiHostPorts"

type apiHostPortsDoc struct {
	APIHostPorts [][]hostPort `bson:"apihostports"`
}

// SetAPIHostPorts sets the addresses of the API server instances.
// Each server is represented by one element in the top level slice.
func (st *State) SetAPIHostPorts(netHostsPorts [][]network.HostPort) error {
	doc := apiHostPortsDoc{
		APIHostPorts: fromNetworkHostsPorts(netHostsPorts),
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
				"apihostports", fromNetworkHostsPorts(existing),
			}},
		}
		if !hostsPortsEqual(netHostsPorts, existing) {
			op.Update = bson.D{{
				"$set", bson.D{{"apihostports", doc.APIHostPorts}},
			}}
		}
		return []txn.Op{op}, nil
	}
	if err := st.run(buildTxn); err != nil {
		return errors.Annotate(err, "cannot set API addresses")
	}
	logger.Debugf("setting API hostPorts: %v", netHostsPorts)
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
	return networkHostsPorts(doc.APIHostPorts), nil
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
		return nil, errors.Trace(err)
	}
	config, err := st.EnvironConfig()
	if err != nil {
		return nil, errors.Trace(err)
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
	Value       string `bson:"value"`
	AddressType string `bson:"addresstype"`
	NetworkName string `bson:"networkname,omitempty"`
	Scope       string `bson:"networkscope,omitempty"`
}

// fromNetworkAddress is a convenience helper to create a state type
// out of the network type, here for Address.
func fromNetworkAddress(netAddr network.Address) address {
	return address{
		Value:       netAddr.Value,
		AddressType: string(netAddr.Type),
		NetworkName: netAddr.NetworkName,
		Scope:       string(netAddr.Scope),
	}
}

// networkAddress is a convenience helper to return the state type
// as network type, here for Address.
func (addr *address) networkAddress() network.Address {
	return network.Address{
		Value:       addr.Value,
		Type:        network.AddressType(addr.AddressType),
		NetworkName: addr.NetworkName,
		Scope:       network.Scope(addr.Scope),
	}
}

// fromNetworkAddresses is a convenience helper to create a state type
// out of the network type, here for a slice of Address.
func fromNetworkAddresses(netAddrs []network.Address) []address {
	addrs := make([]address, len(netAddrs))
	for i, netAddr := range netAddrs {
		addrs[i] = fromNetworkAddress(netAddr)
	}
	return addrs
}

// networkAddresses is a convenience helper to return the state type
// as network type, here for a slice of Address.
func networkAddresses(addrs []address) []network.Address {
	netAddrs := make([]network.Address, len(addrs))
	for i, addr := range addrs {
		netAddrs[i] = addr.networkAddress()
	}
	return netAddrs
}

// hostPort associates an address with a port. See also network.HostPort,
// from/to which this is transformed.
//
// TODO(dimitern) Make sure we integrate this with other networking
// stuff at some point. We want to use juju-specific network names
// that point to existing documents in the networks collection.
type hostPort struct {
	Value       string `bson:"value"`
	AddressType string `bson:"addresstype"`
	NetworkName string `bson:"networkname,omitempty"`
	Scope       string `bson:"networkscope,omitempty"`
	Port        int    `bson:"port"`
}

// fromNetworkHostPort is a convenience helper to create a state type
// out of the network type, here for HostPort.
func fromNetworkHostPort(netHostPort network.HostPort) hostPort {
	return hostPort{
		Value:       netHostPort.Value,
		AddressType: string(netHostPort.Type),
		NetworkName: netHostPort.NetworkName,
		Scope:       string(netHostPort.Scope),
		Port:        netHostPort.Port,
	}
}

// networkHostPort is a convenience helper to return the state type
// as network type, here for HostPort.
func (hp *hostPort) networkHostPort() network.HostPort {
	return network.HostPort{
		Address: network.Address{
			Value:       hp.Value,
			Type:        network.AddressType(hp.AddressType),
			NetworkName: hp.NetworkName,
			Scope:       network.Scope(hp.Scope),
		},
		Port: hp.Port,
	}
}

// fromNetworkHostsPorts is a helper to create a state type
// out of the network type, here for a nested slice of HostPort.
func fromNetworkHostsPorts(netHostsPorts [][]network.HostPort) [][]hostPort {
	hsps := make([][]hostPort, len(netHostsPorts))
	for i, netHostPorts := range netHostsPorts {
		hsps[i] = make([]hostPort, len(netHostPorts))
		for j, netHostPort := range netHostPorts {
			hsps[i][j] = fromNetworkHostPort(netHostPort)
		}
	}
	return hsps
}

// networkHostsPorts is a convenience helper to return the state type
// as network type, here for a nested slice of HostPort.
func networkHostsPorts(hsps [][]hostPort) [][]network.HostPort {
	netHostsPorts := make([][]network.HostPort, len(hsps))
	for i, hps := range hsps {
		netHostsPorts[i] = make([]network.HostPort, len(hps))
		for j, hp := range hps {
			netHostsPorts[i][j] = hp.networkHostPort()
		}
	}
	return netHostsPorts
}

// addressEqual checks that two slices of network addresses are equal.
func addressesEqual(a, b []network.Address) bool {
	return reflect.DeepEqual(a, b)
}

// hostsPortsEqual checks that two arrays of network hostports are equal.
func hostsPortsEqual(a, b [][]network.HostPort) bool {
	return reflect.DeepEqual(a, b)
}
