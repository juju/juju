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
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	corenetwork "github.com/juju/juju/core/network"
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

const (
	// Key for *all* addresses at which controllers are accessible.
	apiHostPortsKey = "apiHostPorts"
	// Key for addresses at which controllers are accessible by agents.
	apiHostPortsForAgentsKey = "apiHostPortsForAgents"
)

type apiHostPortsDoc struct {
	APIHostPorts [][]hostPort `bson:"apihostports"`
	TxnRevno     int64        `bson:"txn-revno"`
}

// SetAPIHostPorts sets the addresses, if changed, of two collections:
// - The list of *all* addresses at which the API is accessible.
// - The list of addresses at which the API can be accessed by agents according
//   to the controller management space configuration.
// Each server is represented by one element in the top level slice.
func (st *State) SetAPIHostPorts(newHostPorts [][]network.HostPort) error {
	controllers, closer := st.db().GetCollection(controllersC)
	defer closer()

	buildTxn := func(attempt int) ([]txn.Op, error) {
		ops, err := st.getOpsForHostPortsChange(controllers, apiHostPortsKey, newHostPorts)
		if err != nil {
			return nil, errors.Trace(err)
		}

		newHostPortsForAgents, err := st.filterHostPortsForManagementSpace(newHostPorts)
		if err != nil {
			return nil, errors.Trace(err)
		}
		agentAddrOps, err := st.getOpsForHostPortsChange(
			controllers, apiHostPortsForAgentsKey, newHostPortsForAgents)
		if err != nil {
			return nil, errors.Trace(err)
		}
		ops = append(ops, agentAddrOps...)

		if ops == nil || len(ops) == 0 {
			return nil, statetxn.ErrNoOperations
		}
		return ops, nil
	}

	if err := st.db().Run(buildTxn); err != nil {
		return errors.Annotate(err, "cannot set API addresses")
	}
	return nil
}

// getOpsForHostPortsChange returns a slice of operations used to update an
// API host/collection in the DB.
// If the current document indicates the same host/port collection as the
// input, no operations are returned.
func (st *State) getOpsForHostPortsChange(
	mc mongo.Collection,
	key string,
	newHostPorts [][]network.HostPort,
) ([]txn.Op, error) {
	var ops []txn.Op

	// Retrieve the current document. Return an insert operation if not found.
	var extantHostPortDoc apiHostPortsDoc
	err := mc.Find(bson.D{{"_id", key}}).One(&extantHostPortDoc)
	if err != nil {
		if err == mgo.ErrNotFound {
			return []txn.Op{{
				C:      controllersC,
				Id:     key,
				Insert: bson.D{{"apihostports", fromNetworkHostsPorts(newHostPorts)}},
			}}, nil
		}
		return ops, err
	}

	// Queue an update operation if the host/port collections differ.
	extantHostPorts := networkHostsPorts(extantHostPortDoc.APIHostPorts)
	if !hostsPortsEqual(newHostPorts, extantHostPorts) {
		ops = []txn.Op{{
			C:  controllersC,
			Id: key,
			Assert: bson.D{{
				"txn-revno", extantHostPortDoc.TxnRevno,
			}},
			Update: bson.D{{
				"$set", bson.D{{"apihostports", fromNetworkHostsPorts(newHostPorts)}},
			}},
		}}
		logger.Debugf("setting %s: %v", key, newHostPorts)
	}
	return ops, nil
}

// filterHostPortsForManagementSpace filters the collection of API addresses
// based on the configured management space for the controller.
// If there is no space configured, or if the one of the slices is filtered down
// to zero elements, just use the unfiltered slice for safety - we do not
// want to cut off communication to the controller based on erroneous config.
func (st *State) filterHostPortsForManagementSpace(apiHostPorts [][]network.HostPort) ([][]network.HostPort, error) {
	config, err := st.ControllerConfig()
	if err != nil {
		return nil, err
	}

	var hostPortsForAgents [][]network.HostPort
	if mgmtSpace := config.JujuManagementSpace(); mgmtSpace != "" {
		hostPortsForAgents = make([][]network.HostPort, len(apiHostPorts))
		sp := network.SpaceName(mgmtSpace)
		for i := range apiHostPorts {
			if filtered, ok := network.SelectHostPortsBySpaceNames(apiHostPorts[i], sp); ok {
				hostPortsForAgents[i] = filtered
			} else {
				hostPortsForAgents[i] = apiHostPorts[i]
			}
		}
	} else {
		hostPortsForAgents = apiHostPorts
	}
	return hostPortsForAgents, nil
}

// APIHostPortsForClients returns the collection of *all* known API addresses.
func (st *State) APIHostPortsForClients() ([][]network.HostPort, error) {
	isCAASCtrl, err := st.isCAASController()
	if err != nil {
		return nil, errors.Trace(err)
	}
	if isCAASCtrl {
		// TODO(caas): add test for this once we have the replacement for Jujuconnsuite.
		return st.apiHostPortsForCAAS(true)
	}

	hp, err := st.apiHostPortsForKey(apiHostPortsKey)
	if err != nil {
		err = errors.Trace(err)
	}
	return hp, err
}

// APIHostPortsForAgents returns the collection of API addresses that should
// be used by agents.
// If the controller model is CAAS type, the return will be the controller
// k8s service addresses in cloud service.
// If there is no management network space configured for the controller,
// or if the space is misconfigured, the return will be the same as
// APIHostPortsForClients.
// Otherwise the returned addresses will correspond with the management net space.
// If there is no document at all, we simply fall back to APIHostPortsForClients.
func (st *State) APIHostPortsForAgents() ([][]network.HostPort, error) {
	isCAASCtrl, err := st.isCAASController()
	if err != nil {
		return nil, errors.Trace(err)
	}
	if isCAASCtrl {
		// TODO(caas): add test for this once we have the replacement for Jujuconnsuite.
		return st.apiHostPortsForCAAS(false)
	}

	hps, err := st.apiHostPortsForKey(apiHostPortsForAgentsKey)
	if err != nil {
		if err == mgo.ErrNotFound {
			logger.Debugf("No document for %s; using %s", apiHostPortsForAgentsKey, apiHostPortsKey)
			return st.APIHostPortsForClients()
		}
		return nil, errors.Trace(err)
	}
	return hps, nil
}

func (st *State) isCAASController() (bool, error) {
	m := &Model{st: st}
	if err := m.refresh(st.ControllerModelUUID()); err != nil {
		return false, errors.Trace(err)
	}
	return m.IsControllerModel() && m.Type() == ModelTypeCAAS, nil
}

func (st *State) apiHostPortsForCAAS(public bool) (addresses [][]network.HostPort, err error) {
	defer func() {
		logger.Debugf("getting api hostports for CAAS: public %t, addresses %v", public, addresses)
	}()

	ctrlSt, err := st.newStateNoWorkers(st.ControllerModelUUID())
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer ctrlSt.Close()

	controllerConfig, err := ctrlSt.ControllerConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}

	apiPort := controllerConfig.APIPort()
	svc, err := ctrlSt.CloudService(controllerConfig.ControllerUUID())
	if err != nil {
		return nil, errors.Trace(err)
	}
	addrs := svc.Addresses()

	addrsToHostPorts := func(addrs ...network.Address) [][]network.HostPort {
		return [][]network.HostPort{network.AddressesWithPort(addrs, apiPort)}
	}

	// select public address.
	if public {
		addr, _ := network.SelectPublicAddress(addrs)
		return addrsToHostPorts(addr), nil
	}
	// select internal address.
	return addrsToHostPorts(network.SelectInternalAddresses(addrs, false)...), nil
}

// apiHostPortsForKey returns API addresses extracted from the document
// identified by the input key.
func (st *State) apiHostPortsForKey(key string) ([][]network.HostPort, error) {
	var doc apiHostPortsDoc
	controllers, closer := st.db().GetCollection(controllersC)
	defer closer()
	err := controllers.Find(bson.D{{"_id", key}}).One(&doc)
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
	Value           string `bson:"value"`
	AddressType     string `bson:"addresstype"`
	Scope           string `bson:"networkscope,omitempty"`
	Origin          string `bson:"origin,omitempty"`
	SpaceName       string `bson:"spacename,omitempty"`
	SpaceProviderId string `bson:"spaceid,omitempty"`
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
		Value:           netAddr.Value,
		AddressType:     string(netAddr.Type),
		Scope:           string(netAddr.Scope),
		Origin:          string(origin),
		SpaceName:       string(netAddr.SpaceName),
		SpaceProviderId: string(netAddr.SpaceProviderId),
	}
}

// networkAddress is a convenience helper to return the state type
// as network type, here for Address.
func (addr *address) networkAddress() network.Address {
	return network.Address{
		Value:           addr.Value,
		Type:            network.AddressType(addr.AddressType),
		Scope:           network.Scope(addr.Scope),
		SpaceName:       network.SpaceName(addr.SpaceName),
		SpaceProviderId: corenetwork.Id(addr.SpaceProviderId),
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
