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

	"github.com/juju/juju/apiserver/params"
	k8sprovider "github.com/juju/juju/caas/kubernetes/provider"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
)

type NetworkInfo interface {
	ProcessAPIRequest(params.NetworkInfoParams) (params.NetworkInfoResults, error)
	NetworksForRelation(
		binding string, rel *state.Relation, pollPublic bool,
	) (boundSpace string, ingress corenetwork.SpaceAddresses, egress []string, err error)

	init(unit *state.Unit) error
}

// TODO (manadart 2019-10-09):
// This module was pulled together out of the state package and the uniter API.
// It lacks sufficient coverage with direct unit tests, relying instead on the
// integration tests that were present at the time of its conception.
// Over time, the state types should be indirected, mocks generated, and
// appropriate tests added.

// NetworkInfoBase is responsible for processing requests for network data
// for unit endpoint bindings and/or relations.
type NetworkInfoBase struct {
	st *state.State
	// retryFactory returns a retry strategy template used to poll for
	// addresses that may not yet have landed in state,
	// such as for CAAS containers or HA syncing.
	retryFactory func() retry.CallArgs

	unit          *state.Unit
	app           *state.Application
	defaultEgress []string
	bindings      map[string]string
}

// NewNetworkInfo initialises and returns a new NetworkInfoBase
// based on the input state and unit tag.
func NewNetworkInfo(st *state.State, tag names.UnitTag, retryFactory func() retry.CallArgs) (NetworkInfo, error) {
	base := &NetworkInfoBase{
		st:           st,
		retryFactory: retryFactory,
	}

	unit, err := st.Unit(tag.Id())
	if err != nil {
		return nil, errors.Trace(err)
	}

	var netInfo NetworkInfo
	if unit.ShouldBeAssigned() {
		netInfo = &NetworkInfoIAAS{base}
	} else {
		netInfo = &NetworkInfoIAAS{base}
	}

	err = netInfo.init(unit)
	return netInfo, errors.Trace(err)
}

// init uses the member state to initialise NetworkInfoBase entities
// in preparation for the retrieval of network information.
func (n *NetworkInfoBase) init(unit *state.Unit) error {
	var err error

	n.unit = unit

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

	return nil
}

// getModelEgressSubnets returns model configuration for egress subnets.
func (n *NetworkInfoBase) getModelEgressSubnets() ([]string, error) {
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
func (n *NetworkInfoBase) getRelationNetworkInfo(
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
func (n *NetworkInfoBase) NetworksForRelation(
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
		address, err := n.pollForAddress(n.unit.PrivateAddress)
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
			address, err := n.pollForAddress(n.unit.PublicAddress)
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
func (n *NetworkInfoBase) machineNetworkInfos(spaceIDs ...string) (map[string]machineNetworkInfoResult, error) {
	machineID, err := n.unit.AssignedMachineId()
	if err != nil {
		return nil, errors.Trace(err)
	}
	machine, err := n.st.Machine(machineID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	spaceSet := set.NewStrings(spaceIDs...)

	results := make(map[string]machineNetworkInfoResult)

	var privateIPAddress string

	if spaceSet.Contains(corenetwork.AlphaSpaceId) {
		var err error
		privateMachineAddress, err := n.pollForAddress(machine.PrivateAddress)
		if err != nil {
			results[corenetwork.AlphaSpaceId] = machineNetworkInfoResult{Error: errors.Annotatef(
				err, "getting machine %q preferred private address", machine.MachineTag())}

			// Remove this ID to prevent further processing.
			spaceSet.Remove(corenetwork.AlphaSpaceId)
		} else {
			privateIPAddress = privateMachineAddress.Value
		}
	}

	// Link-layer devices are set in a single transaction for all devices
	// observed on the machine, so the first result will include them all.
	var addresses []*state.Address
	retryArg := n.retryFactory()
	retryArg.Func = func() error {
		var err error
		addresses, err = machine.AllAddresses()
		return err
	}
	retryArg.IsFatalError = func(err error) bool {
		return err != nil
	}
	if err := retry.Call(retryArg); err != nil {
		result := machineNetworkInfoResult{Error: errors.Annotate(err, "getting devices addresses")}
		for _, id := range spaceSet.Values() {
			if _, ok := results[id]; !ok {
				results[id] = result
			}
		}
		return results, nil
	}

	logger.Debugf("Looking for address from %v in spaces %v", addresses, spaceIDs)

	var privateLinkLayerAddress *state.Address
	for _, addr := range addresses {
		subnet, err := addr.Subnet()
		switch {
		case errors.IsNotFound(err):
			logger.Debugf("skipping %s: not linked to a known subnet (%v)", addr, err)

			// For a space-less model, we will not have subnets populated,
			// and will therefore not find a subnet for the address.
			// Capture the link-layer information for machine private address
			// so that we can return as much information as possible.
			// TODO (manadart 2020-02-21): This will not be required once
			// discovery (or population of subnets by other means) is
			// introduced for the non-space IAAS providers (LXD, manual, etc).
			if addr.Value() == privateIPAddress {
				privateLinkLayerAddress = addr
			}
		case err != nil:
			logger.Errorf("cannot get subnet for address %q - %q", addr, err)
		default:
			if spaceSet.Contains(subnet.SpaceID()) {
				r := results[subnet.SpaceID()]
				r.NetworkInfos, err = addAddressToResult(r.NetworkInfos, addr)
				if err != nil {
					r.Error = err
				} else {
					results[subnet.SpaceID()] = r
				}
			}

			// TODO (manadart 2020-02-21): This reflects the behaviour prior
			// to the introduction of the alpha space.
			// It mimics the old behaviour for the empty space ("").
			// If that was passed in, we included the machine's preferred
			// local-cloud address no matter what space it was in,
			// treating the request as space-agnostic.
			// To preserve this behaviour, we return the address as a result
			// in the alpha space no matter its *real* space if addresses in
			// the alpha space were requested.
			// This should be removed with the institution of universal mutable
			// spaces.
			if spaceSet.Contains(corenetwork.AlphaSpaceId) && addr.Value() == privateIPAddress {
				r := results[corenetwork.AlphaSpaceId]
				r.NetworkInfos, err = addAddressToResult(r.NetworkInfos, addr)
				if err != nil {
					r.Error = err
				} else {
					results[corenetwork.AlphaSpaceId] = r
				}
			}
		}
	}

	// If addresses in the alpha space were requested and we populated none,
	// then we are working with a space-less provider.
	// If we found a link-layer device for the machine's private address,
	// use that information, otherwise return the minimal result based on
	// the IP.
	// TODO (manadart 2020-02-21): As mentioned above, this is not required
	// when we have subnets populated for all providers.
	if r, ok := results[corenetwork.AlphaSpaceId]; !ok && spaceSet.Contains(corenetwork.AlphaSpaceId) {
		if privateLinkLayerAddress != nil {
			r.NetworkInfos, _ = addAddressToResult(r.NetworkInfos, privateLinkLayerAddress)
		} else {
			r.NetworkInfos = []network.NetworkInfo{{
				Addresses: []network.InterfaceAddress{{
					Address: privateIPAddress,
				}},
			}}
		}

		results[corenetwork.AlphaSpaceId] = r
	}

	for _, id := range spaceSet.Values() {
		if _, ok := results[id]; !ok {
			results[id] = machineNetworkInfoResult{
				Error: errors.Errorf("machine %q has no devices in space %q", machineID, id),
			}
		}
	}
	return results, nil
}

// spaceForBinding returns the space id
// associated with the specified endpoint.
func (n *NetworkInfoBase) spaceForBinding(endpoint string) (string, error) {
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

func (n *NetworkInfoBase) pollForAddress(
	fetcher func() (corenetwork.SpaceAddress, error),
) (corenetwork.SpaceAddress, error) {
	var address corenetwork.SpaceAddress
	retryArg := n.retryFactory()
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
		scope := corenetwork.ScopeUnknown
		if strings.HasPrefix(nwInfo.InterfaceName, "fan-") {
			scope = corenetwork.ScopeFanLocal
		}

		for _, addr := range nwInfo.Addresses {
			addrs = append(addrs, corenetwork.NewScopedSpaceAddress(addr.Address, scope))
		}
	}
	return addrs
}

var defaultRetryFactory = func() retry.CallArgs {
	return retry.CallArgs{
		Clock:       clock.WallClock,
		Delay:       3 * time.Second,
		MaxDuration: 30 * time.Second,
	}
}
