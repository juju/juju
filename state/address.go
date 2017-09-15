// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"net"
	"reflect"
	"sort"
	"strconv"

	"github.com/juju/errors"
	statetxn "github.com/juju/txn"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/mongo"
	"github.com/juju/juju/network"
)

// controllerAddresses returns the list of internal addresses of the state
// server machines.
func (st *State) controllerAddresses() ([]string, error) {
	cinfo, err := st.ControllerInfo()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var machines mongo.Collection
	var closer SessionCloser
	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	if model.ModelTag() == cinfo.ModelTag {
		machines, closer = st.db().GetCollection(machinesC)
	} else {
		machines, closer = st.db().GetCollectionFor(cinfo.ModelTag.Id(), machinesC)
	}
	defer closer()

	type addressMachine struct {
		Addresses []address
	}
	var allAddresses []addressMachine
	// TODO(rog) 2013/10/14 index machines on jobs.
	err = machines.Find(bson.D{{"jobs", JobManageModel}}).All(&allAddresses)
	if err != nil {
		return nil, err
	}
	if len(allAddresses) == 0 {
		return nil, errors.New("no controller machines found")
	}
	apiAddrs := make([]string, 0, len(allAddresses))
	for _, addrs := range allAddresses {
		naddrs := networkAddresses(addrs.Addresses)
		addr, ok := network.SelectControllerAddress(naddrs, false)
		if ok {
			apiAddrs = append(apiAddrs, addr.Value)
		}
	}
	if len(apiAddrs) == 0 {
		return nil, errors.New("no controller machines with addresses found")
	}
	return apiAddrs, nil
}

func appendPort(addrs []string, port int) []string {
	newAddrs := make([]string, len(addrs))
	for i, addr := range addrs {
		newAddrs[i] = net.JoinHostPort(addr, strconv.Itoa(port))
	}
	return newAddrs
}

// Addresses returns the list of cloud-internal addresses that
// can be used to connect to the state.
func (st *State) Addresses() ([]string, error) {
	addrs, err := st.controllerAddresses()
	if err != nil {
		return nil, errors.Trace(err)
	}
	config, err := st.ControllerConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return appendPort(addrs, config.StatePort()), nil
}

const apiHostPortsKey = "apiHostPorts"

type apiHostPortsDoc struct {
	APIHostPorts [][]hostPort `bson:"apihostports"`
	TxnRevno     int64        `bson:"txn-revno"`
}

// SetAPIHostPorts sets the addresses of the API server instances.
// Each server is represented by one element in the top level slice.
func (st *State) SetAPIHostPorts(netHostsPorts [][]network.HostPort) error {
	controllers, closer := st.db().GetCollection(controllersC)
	defer closer()
	doc := apiHostPortsDoc{
		APIHostPorts: fromNetworkHostsPorts(netHostsPorts),
	}
	buildTxn := func(attempt int) ([]txn.Op, error) {
		var existingDoc apiHostPortsDoc
		err := controllers.Find(bson.D{{"_id", apiHostPortsKey}}).One(&existingDoc)
		if err != nil {
			return nil, err
		}
		op := txn.Op{
			C:  controllersC,
			Id: apiHostPortsKey,
			Assert: bson.D{{
				"txn-revno", existingDoc.TxnRevno,
			}},
		}
		hostPorts := networkHostsPorts(existingDoc.APIHostPorts)
		if !hostsPortsEqual(netHostsPorts, hostPorts) {
			op.Update = bson.D{{
				"$set", bson.D{{"apihostports", doc.APIHostPorts}},
			}}
		} else {
			return nil, statetxn.ErrNoOperations
		}
		return []txn.Op{op}, nil
	}
	if err := st.db().Run(buildTxn); err != nil {
		return errors.Annotate(err, "cannot set API addresses")
	}
	logger.Debugf("setting API hostPorts: %v", netHostsPorts)
	return nil
}

// APIHostPorts returns the API addresses as set by SetAPIHostPorts.
func (st *State) APIHostPorts() ([][]network.HostPort, error) {
	var doc apiHostPortsDoc
	controllers, closer := st.db().GetCollection(controllersC)
	defer closer()
	err := controllers.Find(bson.D{{"_id", apiHostPortsKey}}).One(&doc)
	if err != nil {
		return nil, err
	}
	return networkHostsPorts(doc.APIHostPorts), nil
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
	Scope       string `bson:"networkscope,omitempty"`
	Origin      string `bson:"origin,omitempty"`
	SpaceName   string `bson:"spacename,omitempty"`
}

// Origin specifies where an address comes from, whether it was reported by a
// provider or by a machine.
type Origin string

const (
	// Address origin unknown.
	OriginUnknown Origin = ""
	// Address comes from a provider.
	OriginProvider Origin = "provider"
	// Address comes from a machine.
	OriginMachine Origin = "machine"
)

// fromNetworkAddress is a convenience helper to create a state type
// out of the network type, here for Address with a given Origin.
func fromNetworkAddress(netAddr network.Address, origin Origin) address {
	return address{
		Value:       netAddr.Value,
		AddressType: string(netAddr.Type),
		Scope:       string(netAddr.Scope),
		Origin:      string(origin),
		SpaceName:   string(netAddr.SpaceName),
	}
}

// networkAddress is a convenience helper to return the state type
// as network type, here for Address.
func (addr *address) networkAddress() network.Address {
	return network.Address{
		Value:     addr.Value,
		Type:      network.AddressType(addr.AddressType),
		Scope:     network.Scope(addr.Scope),
		SpaceName: network.SpaceName(addr.SpaceName),
	}
}

// fromNetworkAddresses is a convenience helper to create a state type
// out of the network type, here for a slice of Address with a given origin.
func fromNetworkAddresses(netAddrs []network.Address, origin Origin) []address {
	addrs := make([]address, len(netAddrs))
	for i, netAddr := range netAddrs {
		addrs[i] = fromNetworkAddress(netAddr, origin)
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
	Scope       string `bson:"networkscope,omitempty"`
	Port        int    `bson:"port"`
	SpaceName   string `bson:"spacename,omitempty"`
}

// fromNetworkHostPort is a convenience helper to create a state type
// out of the network type, here for HostPort.
func fromNetworkHostPort(netHostPort network.HostPort) hostPort {
	return hostPort{
		Value:       netHostPort.Value,
		AddressType: string(netHostPort.Type),
		Scope:       string(netHostPort.Scope),
		Port:        netHostPort.Port,
		SpaceName:   string(netHostPort.SpaceName),
	}
}

// networkHostPort is a convenience helper to return the state type
// as network type, here for HostPort.
func (hp *hostPort) networkHostPort() network.HostPort {
	return network.HostPort{
		Address: network.Address{
			Value:     hp.Value,
			Type:      network.AddressType(hp.AddressType),
			Scope:     network.Scope(hp.Scope),
			SpaceName: network.SpaceName(hp.SpaceName),
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

func dupeAndSort(a [][]network.HostPort) [][]network.HostPort {
	var result [][]network.HostPort

	for _, val := range a {
		var inner []network.HostPort
		for _, hp := range val {
			inner = append(inner, hp)
		}
		network.SortHostPorts(inner)
		result = append(result, inner)
	}
	sort.Sort(hostsPortsSlice(result))
	return result
}

type hostsPortsSlice [][]network.HostPort

func (hp hostsPortsSlice) Len() int      { return len(hp) }
func (hp hostsPortsSlice) Swap(i, j int) { hp[i], hp[j] = hp[j], hp[i] }
func (hp hostsPortsSlice) Less(i, j int) bool {
	lhs := (hostPortsSlice)(hp[i]).String()
	rhs := (hostPortsSlice)(hp[j]).String()
	return lhs < rhs
}

type hostPortsSlice []network.HostPort

func (hp hostPortsSlice) String() string {
	var result string
	for _, val := range hp {
		result += fmt.Sprintf("%s-%d ", val.Address, val.Port)
	}
	return result
}

// hostsPortsEqual checks that two arrays of network hostports are equal.
func hostsPortsEqual(a, b [][]network.HostPort) bool {
	// Make a copy of all the values so we don't mutate the args in order
	// to determine if they are the same while we mutate the slice to order them.
	aPrime := dupeAndSort(a)
	bPrime := dupeAndSort(b)
	return reflect.DeepEqual(aPrime, bPrime)
}
