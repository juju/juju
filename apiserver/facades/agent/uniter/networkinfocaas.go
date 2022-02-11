// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"sort"

	"github.com/juju/errors"

	"github.com/juju/juju/caas"
	k8sprovider "github.com/juju/juju/caas/kubernetes/provider"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// NetworkInfoCAAS is used to provide network info for CAAS units.
type NetworkInfoCAAS struct {
	*NetworkInfoBase

	// addresses contains all services and container addresses for the unit.
	addresses network.SpaceAddresses
}

// newNetworkInfoCAAS returns a NetworkInfo implementation for a CAAS unit.
// It pre-populates the unit addresses - these are used on every code path.
func newNetworkInfoCAAS(base *NetworkInfoBase) (*NetworkInfoCAAS, error) {
	addrs, err := base.unit.AllAddresses()
	if err != nil {
		return nil, errors.Trace(err)
	}
	sort.Sort(addrs)

	return &NetworkInfoCAAS{
		NetworkInfoBase: base,
		addresses:       addrs,
	}, nil
}

// ProcessAPIRequest handles a request to the uniter API NetworkInfo method.
func (n *NetworkInfoCAAS) ProcessAPIRequest(args params.NetworkInfoParams) (params.NetworkInfoResults, error) {
	validEndpoints, result := n.validateEndpoints(args.Endpoints)

	// We record the interface addresses as the machine local ones.
	// These are used later as the binding addresses.
	// For CAAS models, we need to default ingress addresses to all other
	// address scopes so record those in the default ingress address slice.
	var interfaceAddr []params.InterfaceAddress
	var defaultIngressAddresses []string
	for _, a := range n.addresses {
		if a.Scope == network.ScopeMachineLocal {
			interfaceAddr = append(interfaceAddr, params.InterfaceAddress{Address: a.Value})
		} else {
			defaultIngressAddresses = append(defaultIngressAddresses, a.Value)
		}
	}

	defaultEgress := n.defaultEgress
	if len(defaultEgress) == 0 {
		defaultEgress = subnetsForAddresses(defaultIngressAddresses)
	}

	// If we are working in a relation context,
	// get the network information for the relation
	// and set it for the relation's binding.
	if args.RelationId != nil {
		endpoint, _, ingress, egress, err := n.getRelationNetworkInfo(*args.RelationId)
		if err != nil {
			return params.NetworkInfoResults{}, err
		}

		result.Results[endpoint] = params.NetworkInfoResult{
			Info:             []params.NetworkInfo{{Addresses: interfaceAddr}},
			EgressSubnets:    egress,
			IngressAddresses: ingress.Values(),
		}
	}

	// For each of the requested endpoints, set any empty results to the
	// defaults determined above.
	for _, endpoint := range validEndpoints.Values() {
		info, ok := result.Results[endpoint]
		if !ok {
			info = params.NetworkInfoResult{
				Info:             []params.NetworkInfo{{Addresses: interfaceAddr}},
				IngressAddresses: defaultIngressAddresses,
				EgressSubnets:    defaultEgress,
			}
		}

		// We only resolve the `Info` member addresses for CAAS.
		// Host names in `IngressAddresses` are preserved,
		// which diverges from the IAAS implementation.
		result.Results[endpoint] = n.resolveResultInfoHostNames(info)
	}

	return result, nil
}

// getRelationNetworkInfo returns the endpoint name, network space
// and ingress/egress addresses for the input relation ID.
func (n *NetworkInfoCAAS) getRelationNetworkInfo(
	relationId int,
) (string, string, network.SpaceAddresses, []string, error) {
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
	switch caas.ServiceType(svcType) {
	case caas.ServiceLoadBalancer, caas.ServiceExternal:
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
	endpoint string, rel *state.Relation, pollAddr bool,
) (string, network.SpaceAddresses, []string, error) {
	var ingress network.SpaceAddresses
	var err error

	// If NetworksForRelation is called during ProcessAPIRequest,
	// this is a second validation, but we need to do it for the cases
	// where NetworksForRelation is called directly by EnterScope.
	if err = n.validateEndpoint(endpoint); err != nil {
		return "", nil, nil, errors.Trace(err)
	}

	if pollAddr {
		if ingress, err = n.maybeGetUnitAddress(rel, false); err != nil {
			return "", nil, nil, errors.Trace(err)
		}
	}

	if len(ingress) == 0 {
		for _, addr := range n.addresses {
			if addr.Scope != network.ScopeMachineLocal {
				ingress = append(ingress, addr)
			}
		}
	}

	sort.Sort(ingress)

	egress, err := n.getEgressForRelation(rel, ingress)
	if err != nil {
		return "", nil, nil, errors.Trace(err)
	}

	return network.AlphaSpaceId, ingress, egress, nil
}
