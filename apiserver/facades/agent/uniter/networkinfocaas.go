// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"fmt"

	"github.com/juju/errors"
	k8score "k8s.io/api/core/v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	k8sprovider "github.com/juju/juju/caas/kubernetes/provider"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
)

// NetworkInfoCAAS is used to provide network info for CAAS units.
type NetworkInfoCAAS struct {
	*NetworkInfoBase

	addresses corenetwork.SpaceAddresses
}

// newNetworkInfoCAAS returns a NetworkInfo implementation for a CAAS unit.
// It pre-populates the unit addresses - these are used on every code path.
// If there are no configured model egress subnets, we use the addresses to
// populate defaults for the unit.
func newNetworkInfoCAAS(base *NetworkInfoBase) (*NetworkInfoCAAS, error) {
	addrs, err := base.unit.AllAddresses()
	if err != nil {
		return nil, errors.Trace(err)
	}
	corenetwork.SortAddresses(addrs)

	if len(base.defaultEgress) == 0 {
		base.defaultEgress = subnetsForAddresses(addrs.Values())
	}

	return &NetworkInfoCAAS{
		NetworkInfoBase: base,
		addresses:       addrs,
	}, nil
}

// ProcessAPIRequest handles a request to the uniter API NetworkInfo method.
func (n *NetworkInfoCAAS) ProcessAPIRequest(args params.NetworkInfoParams) (params.NetworkInfoResults, error) {
	bindings := make(map[string]string)
	endpointEgressSubnets := make(map[string][]string)

	result := params.NetworkInfoResults{
		Results: make(map[string]params.NetworkInfoResult),
	}

	// For each of the endpoints in the request, get the bound space and
	// initialise the endpoint egress map with the model's configured
	// egress subnets.
	for _, endpoint := range args.Endpoints {
		binding, ok := n.bindings[endpoint]
		if ok {
			// In practice this is always the alpha space in CAAS.
			// This loop serves as validation of input until this changes.
			bindings[endpoint] = binding
		} else {
			err := errors.NewNotValid(nil, fmt.Sprintf("binding name %q not defined by the unit's charm", endpoint))
			result.Results[endpoint] = params.NetworkInfoResult{Error: common.ServerError(err)}
		}
	}

	endpointIngressAddresses := make(map[string]corenetwork.SpaceAddresses)

	// If we are working in a relation context, get the network information for
	// the relation and set it for the relation's binding.
	if args.RelationId != nil {
		endpoint, _, ingress, egress, err := n.getRelationNetworkInfo(*args.RelationId)
		if err != nil {
			return params.NetworkInfoResults{}, err
		}

		if len(egress) > 0 {
			endpointEgressSubnets[endpoint] = egress
		}
		endpointIngressAddresses[endpoint] = ingress
	}

	// We record the interface addresses as the machine local ones.
	// These are used later as the binding addresses.
	// For CAAS models, we need to default ingress addresses to public ones.
	var interfaceAddr []network.InterfaceAddress
	var defaultIngressAddresses []string
	for _, a := range n.addresses {
		switch a.Scope {
		case corenetwork.ScopeMachineLocal:
			interfaceAddr = append(interfaceAddr, network.InterfaceAddress{Address: a.Value})
		case corenetwork.ScopePublic:
			defaultIngressAddresses = append(defaultIngressAddresses, a.Value)
		}
	}

	networkInfos := map[string]machineNetworkInfoResult{
		corenetwork.AlphaSpaceId: {NetworkInfos: []network.NetworkInfo{{Addresses: interfaceAddr}}},
	}

	for endpoint, space := range bindings {
		// The binding address information based on link layer devices.
		info := machineNetworkInfoResultToNetworkInfoResult(networkInfos[space])

		info.EgressSubnets = endpointEgressSubnets[endpoint]
		info.IngressAddresses = endpointIngressAddresses[endpoint].Values()

		// If there is no ingress address explicitly defined for a given
		// binding, set the ingress addresses to either any defaults set above,
		// or the binding addresses.
		if len(info.IngressAddresses) == 0 {
			info.IngressAddresses = defaultIngressAddresses
		}

		if len(info.IngressAddresses) == 0 {
			ingress := spaceAddressesFromNetworkInfo(networkInfos[space].NetworkInfos)
			corenetwork.SortAddresses(ingress)
			info.IngressAddresses = ingress.Values()
		}

		if len(info.EgressSubnets) == 0 {
			info.EgressSubnets = n.defaultEgress
		}

		result.Results[endpoint] = n.resolveResultHostNames(info)
	}

	return result, nil
}

// getRelationNetworkInfo returns the endpoint name, network space
// and ingress/egress addresses for the input relation ID.
func (n *NetworkInfoCAAS) getRelationNetworkInfo(
	relationId int,
) (string, string, corenetwork.SpaceAddresses, []string, error) {
	rel, endpoint, err := n.getRelationAndEndpointName(relationId)
	if err != nil {
		return "", "", nil, nil, errors.Trace(err)
	}

	cfg, err := n.app.ApplicationConfig()
	if err != nil {
		return "", "", nil, nil, errors.Trace(err)
	}

	var pollAddr bool
	svcType := cfg.GetString(k8sprovider.ServiceTypeConfigKey, "")
	switch k8score.ServiceType(svcType) {
	case k8score.ServiceTypeLoadBalancer, k8score.ServiceTypeExternalName:
		pollAddr = true
	}

	space, ingress, egress, err := n.NetworksForRelation(endpoint, rel, pollAddr)
	return endpoint, space, ingress, egress, errors.Trace(err)
}

// NetworksForRelation returns the ingress and egress addresses for
// a relation and unit.
// The ingress addresses depend on if the relation is cross-model
// and whether the relation endpoint is bound to a space.
func (n *NetworkInfoCAAS) NetworksForRelation(
	_ string, rel *state.Relation, pollAddr bool,
) (string, corenetwork.SpaceAddresses, []string, error) {
	var ingress corenetwork.SpaceAddresses
	var err error

	if err = n.setCrossModelStatus(rel); err != nil {
		return "", nil, nil, errors.Trace(err)
	}

	if pollAddr {
		if ingress, err = n.maybeGetUnitAddress(rel, false); err != nil {
			return "", nil, nil, errors.Trace(err)
		}
	}

	// Ingress addresses can only be public addresses for CAAS.
	// They are always scoped explicitly. See: caas/kubernetes/provider/k8s.go.
	if len(ingress) == 0 {
		for _, addr := range n.addresses {
			if addr.Scope == corenetwork.ScopePublic {
				ingress = append(ingress, addr)
			}
		}
	}

	// If we are working with a cross-model relation, omit any non-public scope
	// addresses from the default egress.
	if n.isCrossModelRelation {
		n.defaultEgress = subnetsForAddresses(ingress.Values())
	}

	// We can pass nil as ingress here, because we have already set
	// n.defaultEgress to either the model configuration or those generated
	// for the unit.
	egress, err := n.getEgressForRelation(rel, nil)
	if err != nil {
		return "", nil, nil, errors.Trace(err)
	}

	return corenetwork.AlphaSpaceId, ingress, egress, nil
}
