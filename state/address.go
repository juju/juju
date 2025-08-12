// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"net"
	"reflect"
	"strconv"

	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"
	jujutxn "github.com/juju/txn/v3"

	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/mongo"
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
		addr, ok := networkAddresses(addrs.Addresses).OneMatchingScope(network.ScopeMatchCloudLocal)
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
//   - The list of *all* addresses at which the API is accessible.
//   - The list of addresses at which the API can be accessed by agents according
//     to the controller management space configuration.
//
// Each server is represented by one element in the top level slice.
func (st *State) SetAPIHostPorts(newHostPorts []network.SpaceHostPorts) error {
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
		agentAddrOps, err := st.getOpsForHostPortsChange(controllers, apiHostPortsForAgentsKey, newHostPortsForAgents)
		if err != nil {
			return nil, errors.Trace(err)
		}
		ops = append(ops, agentAddrOps...)

		if len(ops) == 0 {
			return nil, jujutxn.ErrNoOperations
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
	mc mongo.Collection, key string, newHostPorts []network.SpaceHostPorts,
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
	newHostPortData := networkHostsPortsForCompare(newHostPorts)
	if !hostsPortsEqual(newHostPortData, extantHostPorts) {
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
// If there is no space configured, or if one of the slices is filtered down
// to zero elements, just use the unfiltered slice for safety - we do not
// want to cut off communication to the controller based on erroneous config.
func (st *State) filterHostPortsForManagementSpace(
	apiHostPorts []network.SpaceHostPorts,
) ([]network.SpaceHostPorts, error) {
	config, err := st.ControllerConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var hostPortsForAgents []network.SpaceHostPorts
	if mgmtSpace := config.JujuManagementSpace(); mgmtSpace != "" {
		sp, err := st.SpaceByName(mgmtSpace)
		if err != nil {
			return nil, errors.Trace(err)
		}
		spaceInfo, err := sp.NetworkSpace()
		if err != nil {
			return nil, errors.Trace(err)
		}

		hostPortsForAgents = make([]network.SpaceHostPorts, len(apiHostPorts))
		for i := range apiHostPorts {
			if filtered, ok := apiHostPorts[i].InSpaces(spaceInfo); ok {
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
func (st *State) APIHostPortsForClients() ([]network.SpaceHostPorts, error) {
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
func (st *State) APIHostPortsForAgents() ([]network.SpaceHostPorts, error) {
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

func (st *State) apiHostPortsForCAAS(public bool) (addresses []network.SpaceHostPorts, err error) {
	defer func() {
		logger.Tracef("getting api hostports for CAAS: public %t, addresses %v", public, addresses)
	}()

	if st.ModelUUID() != st.controllerModelTag.Id() {
		return nil, errors.Errorf("CAAS API host ports only available on the controller model, not %q", st.ModelUUID())
	}

	controllerConfig, err := st.ControllerConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}

	apiPort := controllerConfig.APIPort()
	svc, err := st.CloudService(controllerConfig.ControllerUUID())
	if err != nil {
		return nil, errors.Trace(err)
	}
	addrs := svc.Addresses()

	addrsToHostPorts := func(addrs ...network.SpaceAddress) []network.SpaceHostPorts {
		return []network.SpaceHostPorts{network.SpaceAddressesWithPort(addrs, apiPort)}
	}

	// Return all publicly available addresses for clients.
	// Scope matching falls back through a hierarchy,
	// so these may actually be local-cloud addresses.
	publicAddrs := addrs.AllMatchingScope(network.ScopeMatchPublic)
	if public {
		return addrsToHostPorts(publicAddrs...), nil
	}

	var hostAddresses network.SpaceAddresses
	// Add in the FQDN of the controller service for agents to use as an option.
	controllerName := controllerConfig.ControllerName()
	if controllerName != "" {
		hostAddresses = append(
			hostAddresses, network.NewSpaceAddress(
				fmt.Sprintf(k8sconstants.ControllerServiceFQDNTemplate, controllerName),
				network.WithScope(network.ScopeCloudLocal),
			))
	}

	// TODO(wallyworld) - for now, return all addresses for agents to try, public last.

	// If we are after local-cloud addresses and those were all that public
	// matching turned up, just return those.
	if len(publicAddrs) > 0 && publicAddrs[0].Scope == network.ScopeCloudLocal {
		return addrsToHostPorts(append(hostAddresses, publicAddrs...)...), nil
	}

	localAddrs := addrs.AllMatchingScope(network.ScopeMatchCloudLocal)

	// If there were no local-cloud addresses, return the public ones.
	if len(localAddrs) == 0 || localAddrs[0].Scope == network.ScopePublic {
		return addrsToHostPorts(append(hostAddresses, publicAddrs...)...), nil
	}

	// Otherwise return everything, local-cloud first.
	hostAddresses = append(hostAddresses, localAddrs...)
	hostAddresses = append(hostAddresses, publicAddrs...)
	return addrsToHostPorts(hostAddresses...), nil
}

// apiHostPortsForKey returns API addresses extracted from the document
// identified by the input key.
func (st *State) apiHostPortsForKey(key string) ([]network.SpaceHostPorts, error) {
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
	Value       string `bson:"value"`
	AddressType string `bson:"addresstype"`
	Scope       string `bson:"networkscope,omitempty"`
	Origin      string `bson:"origin,omitempty"`
	SpaceID     string `bson:"spaceid,omitempty"`
	CIDR        string `bson:"cidr,omitempty"`
}

// fromNetworkAddress is a convenience helper to create a state type
// out of the network type, here for Address with a given Origin.
func fromNetworkAddress(netAddr network.SpaceAddress, origin network.Origin) address {
	return address{
		Value:       netAddr.Value,
		AddressType: string(netAddr.Type),
		Scope:       string(netAddr.Scope),
		Origin:      string(origin),
		SpaceID:     netAddr.SpaceID,
		CIDR:        netAddr.CIDR,
	}
}

// networkAddress is a convenience helper to return the state type
// as network type, here for Address.
func (addr *address) networkAddress() network.SpaceAddress {
	return network.SpaceAddress{
		MachineAddress: network.MachineAddress{
			Value: addr.Value,
			Type:  network.AddressType(addr.AddressType),
			Scope: network.Scope(addr.Scope),
			CIDR:  addr.CIDR,
		},
		SpaceID: addr.SpaceID,
	}
}

// fromNetworkAddresses is a convenience helper to create a state type
// out of the network type, here for a slice of Address with a given origin.
func fromNetworkAddresses(netAddrs network.SpaceAddresses, origin network.Origin) []address {
	addrs := make([]address, len(netAddrs))
	for i, netAddr := range netAddrs {
		addrs[i] = fromNetworkAddress(netAddr, origin)
	}
	return addrs
}

// networkAddresses is a convenience helper to return the state type
// as network type, here for a slice of Address.
func networkAddresses(addrs []address) network.SpaceAddresses {
	netAddrs := make(network.SpaceAddresses, len(addrs))
	for i, addr := range addrs {
		netAddrs[i] = addr.networkAddress()
	}
	return netAddrs
}

// hostPort associates an address with a port. See also network.SpaceHostPort,
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
	SpaceID     string `bson:"spaceid,omitempty"`
}

// fromNetworkHostPort is a convenience helper to create a state type
// out of the network type, here for SpaceHostPort.
func fromNetworkHostPort(netHostPort network.SpaceHostPort) hostPort {
	return hostPort{
		Value:       netHostPort.Value,
		AddressType: string(netHostPort.Type),
		Scope:       string(netHostPort.Scope),
		Port:        netHostPort.Port(),
		SpaceID:     netHostPort.SpaceID,
	}
}

// networkHostPort is a convenience helper to return the state type
// as network type, here for SpaceHostPort.
func (hp *hostPort) networkHostPort() network.SpaceHostPort {
	return network.SpaceHostPort{
		SpaceAddress: network.SpaceAddress{
			MachineAddress: network.MachineAddress{
				Value: hp.Value,
				Type:  network.AddressType(hp.AddressType),
				Scope: network.Scope(hp.Scope),
			},
			SpaceID: hp.SpaceID,
		},
		NetPort: network.NetPort(hp.Port),
	}
}

// fromNetworkHostsPorts is a helper to create a state type
// out of the network type, here for a nested slice of SpaceHostPort.
func fromNetworkHostsPorts(netHostsPorts []network.SpaceHostPorts) [][]hostPort {
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
// as network type, here for a nested slice of SpaceHostPort.
func networkHostsPorts(hsps [][]hostPort) []network.SpaceHostPorts {
	netHostsPorts := make([]network.SpaceHostPorts, len(hsps))
	for i, hps := range hsps {
		netHostsPorts[i] = make(network.SpaceHostPorts, len(hps))
		for j, hp := range hps {
			netHostsPorts[i][j] = hp.networkHostPort()
		}
	}
	return netHostsPorts
}

// networkHostsPortsForCompare returns a copy of in with only the values
// stored in the database filled in. This allows a meaningful sort to be done.
func networkHostsPortsForCompare(in []network.SpaceHostPorts) []network.SpaceHostPorts {
	netHostsPorts := make([]network.SpaceHostPorts, len(in))
	for i, hps := range in {
		netHostsPorts[i] = make(network.SpaceHostPorts, len(hps))
		for j, hp := range hps {
			netHostsPorts[i][j] = network.SpaceHostPort{
				SpaceAddress: network.SpaceAddress{
					MachineAddress: network.MachineAddress{
						Value: hp.Value,
						Type:  hp.Type,
						Scope: hp.Scope,
					},
					SpaceID: hp.SpaceID,
				},
				NetPort: hp.NetPort,
			}
		}
	}
	return netHostsPorts
}

// addressEqual checks that two slices of network addresses are equal.
func addressesEqual(a, b []network.SpaceAddress) bool {
	return reflect.DeepEqual(a, b)
}

// hostsPortsEqual checks that two arrays of network hostports are equal.
func hostsPortsEqual(a, b []network.SpaceHostPorts) bool {
	// Make a copy of all the values so we don't mutate the args in order
	// to determine if they are the same while we mutate the slice to order them.
	aPrime := network.DupeAndSort(a)
	bPrime := network.DupeAndSort(b)
	return reflect.DeepEqual(aPrime, bPrime)
}

func (st *State) ConvertSpaceHostPorts(sHPs network.SpaceHostPorts) (network.ProviderHostPorts, error) {
	addrs := make(network.ProviderHostPorts, len(sHPs))
	for i, sAddr := range sHPs {
		var err error
		if addrs[i], err = st.ConvertSpaceHostPort(sAddr); err != nil {
			return nil, errors.Trace(err)
		}
	}
	return addrs, nil
}

func (st *State) ConvertSpaceHostPort(sHP network.SpaceHostPort) (network.ProviderHostPort, error) {
	hp := network.ProviderHostPort{
		ProviderAddress: network.ProviderAddress{MachineAddress: sHP.MachineAddress},
		NetPort:         sHP.NetPort,
	}
	if sHP.SpaceID != "" {
		space, err := st.Space(sHP.SpaceID)
		if err != nil {
			return hp, errors.Trace(err)
		}
		hp.SpaceName = network.SpaceName(space.Name())
	}
	return hp, nil
}
