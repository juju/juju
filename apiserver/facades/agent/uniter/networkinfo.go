// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"fmt"
	"strings"
	"time"

	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/retry"
	k8score "k8s.io/api/core/v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/networkingcommon"
	"github.com/juju/juju/apiserver/params"
	k8sprovider "github.com/juju/juju/caas/kubernetes/provider"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
)

// TODO (manadart 2019-10-09):
// This module was pulled together out of the state package and the uniter API.
// It lacks sufficient coverage with direct unit tests, relying instead on the
// integration tests that were present at the time of its conception.
// Over time, the state types should be indirected, mocks generated, and
// appropriate tests added.

// PreferredAddressRetryArgs returns the retry strategy for getting a unit's
// preferred address. Override for testing to use a different clock.
// TODO (manadart 2019-10-09): Pass this as an argument to the NetworkInfo
// constructor instead of exporting this public type for patching in tests.
var PreferredAddressRetryArgs = func() retry.CallArgs {
	return retry.CallArgs{
		Clock:       clock.WallClock,
		Delay:       3 * time.Second,
		MaxDuration: 30 * time.Second,
	}
}

// NetworkInfo is responsible for processing requests for network data for unit
// endpoint bindings and/or relations.
type NetworkInfo struct {
	st            *state.State
	unit          *state.Unit
	app           *state.Application
	defaultEgress []string
	bindings      map[string]string
	spaces        []*state.Space
}

// NewNetworkInfo initialises and returns a new NetworkInfo based on the input
// state and unit tag.
func NewNetworkInfo(st *state.State, tag names.UnitTag) (*NetworkInfo, error) {
	netInfo := &NetworkInfo{st: st}
	err := netInfo.init(tag)
	return netInfo, errors.Trace(err)
}

// init uses the member state to initialise NetworkInfo entities in preparation
// for the retrieval of network information.
func (n *NetworkInfo) init(tag names.UnitTag) error {
	var err error

	if n.unit, err = n.st.Unit(tag.Id()); err != nil {
		return errors.Trace(err)
	}

	if n.app, err = n.unit.Application(); err != nil {
		return errors.Trace(err)
	}

	bindings, err := n.app.EndpointBindings()
	if err != nil {
		return errors.Trace(err)
	}
	n.bindings = bindings.Map()

	if n.defaultEgress, err = n.getModelEgressSubnets(); err != nil {
		return errors.Trace(err)
	}

	if n.spaces, err = n.st.AllSpaces(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

// getModelEgressSubnets returns model configuration for egress subnets.
func (n *NetworkInfo) getModelEgressSubnets() ([]string, error) {
	model, err := n.st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	cfg, err := model.ModelConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return cfg.EgressSubnets(), nil
}

// ProcessAPIRequest handles a request to the uniter API NetworkInfo method.
// TODO (manadart 2019-10-09): This method verges on impossible to reason about
// and should be rewritten.
func (n *NetworkInfo) ProcessAPIRequest(args params.NetworkInfoParams) (params.NetworkInfoResults, error) {
	spaces := set.NewStrings()
	bindings := make(map[string]string)
	endpointEgressSubnets := make(map[string][]string)

	result := params.NetworkInfoResults{
		Results: make(map[string]params.NetworkInfoResult),
	}
	// For each of the endpoints in the request, get the bound space and
	// initialise the endpoint egress map with the model's configured
	// egress subnets. Keep track of the spaces that we observe.
	for _, endpoint := range args.Endpoints {
		binding, ok := n.bindings[endpoint]
		if ok {
			spaces.Add(binding)
			bindings[endpoint] = binding
		} else {
			// If default binding is not explicitly defined, use the default space.
			// This should no longer be the case....
			if endpoint == "" {
				bindings[endpoint] = corenetwork.AlphaSpaceId
			} else {
				err := errors.NewNotValid(nil, fmt.Sprintf("binding name %q not defined by the unit's charm", endpoint))
				result.Results[endpoint] = params.NetworkInfoResult{Error: common.ServerError(err)}
			}
		}
		endpointEgressSubnets[endpoint] = n.defaultEgress
	}

	endpointIngressAddresses := make(map[string]corenetwork.SpaceAddresses)

	// If we are working in a relation context, get the network information for
	// the relation and set it for the relation's binding.
	if args.RelationId != nil {
		endpoint, space, ingress, egress, err := n.getRelationNetworkInfo(*args.RelationId)
		if err != nil {
			return params.NetworkInfoResults{}, err
		}

		spaces.Add(space)
		if len(egress) > 0 {
			endpointEgressSubnets[endpoint] = egress
		}
		endpointIngressAddresses[endpoint] = ingress
	}

	var (
		networkInfos            map[string]state.MachineNetworkInfoResult
		defaultIngressAddresses []string
	)

	if n.unit.ShouldBeAssigned() {
		var err error
		// TODO (manadart 2019-09-10): This looks like it might be called
		// twice in some cases - getRelationNetworkInfo (called above)
		// calls NetworksForRelation, which also calls this method.
		if networkInfos, err = n.machineNetworkInfos(spaces.Values()...); err != nil {
			return params.NetworkInfoResults{}, err
		}
	} else {
		// For CAAS units, we build up a minimal result struct
		// based on the default space and unit public/private addresses,
		// ie the addresses of the CAAS service.
		addrs, err := n.unit.AllAddresses()
		if err != nil {
			return params.NetworkInfoResults{}, err
		}
		corenetwork.SortAddresses(addrs)

		// We record the interface addresses as the machine local ones - these
		// are used later as the binding addresses.
		// For CAAS models, we need to default ingress addresses to all available
		// addresses so record those in the default ingress address slice.
		var interfaceAddr []network.InterfaceAddress
		for _, a := range addrs {
			if a.Scope == corenetwork.ScopeMachineLocal {
				interfaceAddr = append(interfaceAddr, network.InterfaceAddress{Address: a.Value})
			} else {
				defaultIngressAddresses = append(defaultIngressAddresses, a.Value)
			}
		}
		networkInfos = make(map[string]state.MachineNetworkInfoResult)
		networkInfos[corenetwork.AlphaSpaceId] = state.MachineNetworkInfoResult{
			NetworkInfos: []network.NetworkInfo{{Addresses: interfaceAddr}},
		}
	}

	for endpoint, space := range bindings {
		// The binding address information based on link layer devices.
		info := networkingcommon.MachineNetworkInfoResultToNetworkInfoResult(networkInfos[space])

		// Set egress and ingress address information.
		info.EgressSubnets = endpointEgressSubnets[endpoint]

		ingressAddrs := make([]string, len(endpointIngressAddresses[endpoint]))
		for i, addr := range endpointIngressAddresses[endpoint] {
			ingressAddrs[i] = addr.Value
		}
		info.IngressAddresses = ingressAddrs

		// If there is no ingress address explicitly defined for a given
		// binding, set the ingress addresses to either any defaults set above,
		// or the binding addresses.
		if len(info.IngressAddresses) == 0 {
			info.IngressAddresses = defaultIngressAddresses
		}

		if len(info.IngressAddresses) == 0 {
			ingress := spaceAddressesFromNetworkInfo(networkInfos[space].NetworkInfos)
			corenetwork.SortAddresses(ingress)
			info.IngressAddresses = make([]string, len(ingress))
			for i, addr := range ingress {
				info.IngressAddresses[i] = addr.Value
			}
		}

		// If there is no egress subnet explicitly defined for a given binding,
		// default to the first ingress address. This matches the behaviour when
		// there's a relation in place.
		if len(info.EgressSubnets) == 0 && len(info.IngressAddresses) > 0 {
			var err error
			info.EgressSubnets, err = network.FormatAsCIDR([]string{info.IngressAddresses[0]})
			if err != nil {
				return result, errors.Trace(err)
			}
		}

		result.Results[endpoint] = info
	}

	return dedupNetworkInfoResults(result), nil
}

func dedupNetworkInfoResults(info params.NetworkInfoResults) params.NetworkInfoResults {
	for epName, res := range info.Results {
		if res.Error != nil {
			continue
		}
		res.IngressAddresses = dedupStringListPreservingOrder(res.IngressAddresses)
		res.EgressSubnets = dedupStringListPreservingOrder(res.EgressSubnets)
		for infoIdx, info := range res.Info {
			res.Info[infoIdx].Addresses = dedupAddrList(info.Addresses)
		}
		info.Results[epName] = res
	}

	return info
}

func dedupStringListPreservingOrder(values []string) []string {
	// Ideally, we would use a set.Strings(values).Values() here but since
	// it does not preserve the insertion order we need to do this manually.
	seen := set.NewStrings()
	out := make([]string, 0, len(values))
	for _, v := range values {
		if seen.Contains(v) {
			continue
		}
		seen.Add(v)
		out = append(out, v)
	}

	return out
}

func dedupAddrList(addrList []params.InterfaceAddress) []params.InterfaceAddress {
	if len(addrList) <= 1 {
		return addrList
	}

	uniqueAddrList := make([]params.InterfaceAddress, 0, len(addrList))
	seenAddrSet := set.NewStrings()
	for _, addr := range addrList {
		if seenAddrSet.Contains(addr.Address) {
			continue
		}

		seenAddrSet.Add(addr.Address)
		uniqueAddrList = append(uniqueAddrList, addr)
	}

	return uniqueAddrList
}

// getRelationNetworkInfo returns the endpoint name, network space
// and ingress/egress addresses for the input relation ID.
func (n *NetworkInfo) getRelationNetworkInfo(
	relationId int,
) (string, string, corenetwork.SpaceAddresses, []string, error) {
	rel, err := n.st.Relation(relationId)
	if err != nil {
		return "", "", nil, nil, errors.Trace(err)
	}
	endpoint, err := rel.Endpoint(n.unit.ApplicationName())
	if err != nil {
		return "", "", nil, nil, errors.Trace(err)
	}

	pollPublic := n.unit.ShouldBeAssigned()
	// For k8s services which may have a public
	// address, we want to poll in case it's not ready yet.
	if !pollPublic {
		cfg, err := n.app.ApplicationConfig()
		if err != nil {
			return "", "", nil, nil, errors.Trace(err)
		}
		svcType := cfg.GetString(k8sprovider.ServiceTypeConfigKey, "")
		switch k8score.ServiceType(svcType) {
		case k8score.ServiceTypeLoadBalancer, k8score.ServiceTypeExternalName:
			pollPublic = true
		}
	}

	space, ingress, egress, err := n.NetworksForRelation(endpoint.Name, rel, pollPublic)
	return endpoint.Name, space, ingress, egress, errors.Trace(err)
}

// NetworksForRelation returns the ingress and egress addresses for
// a relation and unit.
// The ingress addresses depend on if the relation is cross-model
// and whether the relation endpoint is bound to a space.
func (n *NetworkInfo) NetworksForRelation(
	binding string, rel *state.Relation, pollPublic bool,
) (boundSpace string, ingress corenetwork.SpaceAddresses, egress []string, _ error) {
	relEgress := state.NewRelationEgressNetworks(n.st)
	egressSubnets, err := relEgress.Networks(rel.Tag().Id())
	if err != nil && !errors.IsNotFound(err) {
		return "", nil, nil, errors.Trace(err)
	} else if err == nil {
		egress = egressSubnets.CIDRS()
	} else {
		egress = n.defaultEgress
	}

	boundSpace, err = n.spaceForBinding(binding)
	if err != nil && !errors.IsNotValid(err) {
		return "", nil, nil, errors.Trace(err)
	}

	fallbackIngressToPrivateAddr := func() error {
		// TODO(ycliuhw): lp-1830252 retry here once this is fixed.
		address, err := n.unit.PrivateAddress()
		if err != nil {
			logger.Warningf("no private address for unit %q in relation %q", n.unit.Name(), rel)
		} else if address.Value != "" {
			ingress = append(ingress, address)
		}
		return nil
	}

	// If the endpoint for this relation is not bound to a space, or
	// is bound to the default space, we need to look up the ingress
	// address info which is aware of cross model relations.
	if boundSpace == corenetwork.AlphaSpaceId || err != nil {
		_, crossModel, err := rel.RemoteApplication()
		if err != nil {
			return "", nil, nil, errors.Trace(err)
		}
		if crossModel && (n.unit.ShouldBeAssigned() || pollPublic) {
			address, err := pollForAddress(n.unit.PublicAddress)
			if err != nil {
				logger.Warningf(
					"no public address for unit %q in cross model relation %q, will use private address",
					n.unit.Name(), rel,
				)
			} else if address.Value != "" {
				ingress = append(ingress, address)
			}
			if len(ingress) == 0 {
				if err := fallbackIngressToPrivateAddr(); err != nil {
					return "", nil, nil, errors.Trace(err)
				}
			}
		}
	}

	if len(ingress) == 0 {
		if n.unit.ShouldBeAssigned() {
			// We don't yet have an ingress address, so pick one from the space to
			// which the endpoint is bound.
			networkInfos, err := n.machineNetworkInfos(boundSpace)
			if err != nil {
				return "", nil, nil, errors.Trace(err)
			}
			ingress = spaceAddressesFromNetworkInfo(networkInfos[boundSpace].NetworkInfos)
		} else {
			// Be be consistent with IAAS behaviour above, we'll return all addresses.
			addrs, err := n.unit.AllAddresses()
			if err != nil {
				logger.Warningf("no service address for unit %q in relation %q", n.unit.Name(), rel)
			} else {
				for _, addr := range addrs {
					if addr.Scope != corenetwork.ScopeMachineLocal {
						ingress = append(ingress, addr)
					}
				}
			}
		}
	}

	corenetwork.SortAddresses(ingress)

	// If no egress subnets defined, We default to the ingress address.
	if len(egress) == 0 && len(ingress) > 0 {
		egress, err = network.FormatAsCIDR([]string{ingress[0].Value})
		if err != nil {
			return "", nil, nil, errors.Trace(err)
		}
	}
	return boundSpace, ingress, egress, nil
}

// machineNetworkInfos returns network info for the unit's machine based on
// devices with addresses in the input spaces.
// TODO (manadart 2019-10-10): `GetNetworkInfoForSpaces` is only used here and
// could be relocated from the state package, reducing cross-cutting concerns
// there.
func (n *NetworkInfo) machineNetworkInfos(spaces ...string) (map[string]state.MachineNetworkInfoResult, error) {
	machineID, err := n.unit.AssignedMachineId()
	if err != nil {
		return nil, err
	}
	machine, err := n.st.Machine(machineID)
	if err != nil {
		return nil, err
	}

	return machine.GetNetworkInfoForSpaces(set.NewStrings(spaces...)), nil
}

// spaceForBinding returns the space id
// associated with the specified endpoint.
func (n *NetworkInfo) spaceForBinding(endpoint string) (string, error) {
	boundSpace, known := n.bindings[endpoint]
	if !known {
		// If default binding is not explicitly defined, use the default space.
		// This should no longer be the case....
		if endpoint == "" {
			return corenetwork.AlphaSpaceId, nil
		}
		return "", errors.NewNotValid(nil, fmt.Sprintf("binding id %q not defined by the unit's charm", endpoint))
	}
	return boundSpace, nil
}

// spaceAddressesFromNetworkInfo returns a SpaceAddresses collection
// from a slice of NetworkInfo.
// We need to construct sortable addresses from link-layer devices,
// which unlike addresses from the machines collection, do not have the scope
// information that we need.
// The best we can do here is identify fan addresses so that they are sorted
// after other addresses.
func spaceAddressesFromNetworkInfo(netInfos []network.NetworkInfo) corenetwork.SpaceAddresses {
	var addrs corenetwork.SpaceAddresses
	for _, nwInfo := range netInfos {
		scope := corenetwork.ScopeCloudLocal
		if strings.HasPrefix(nwInfo.InterfaceName, "fan-") {
			scope = corenetwork.ScopeFanLocal
		}

		for _, addr := range nwInfo.Addresses {
			addrs = append(addrs, corenetwork.NewScopedSpaceAddress(addr.Address, scope))
		}
	}
	return addrs
}

func pollForAddress(fetcher func() (corenetwork.SpaceAddress, error)) (corenetwork.SpaceAddress, error) {
	var address corenetwork.SpaceAddress
	retryArg := PreferredAddressRetryArgs()
	retryArg.Func = func() error {
		var err error
		address, err = fetcher()
		return err
	}
	retryArg.IsFatalError = func(err error) bool {
		return !network.IsNoAddressError(err)
	}
	return address, retry.Call(retryArg)
}
