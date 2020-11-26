// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"net"
	"strings"
	"time"

	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/retry"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/params"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
)

type NetworkInfo interface {
	ProcessAPIRequest(params.NetworkInfoParams) (params.NetworkInfoResults, error)
	NetworksForRelation(
		binding string, rel *state.Relation, pollPublic bool,
	) (boundSpace string, ingress corenetwork.SpaceAddresses, egress []string, err error)
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
	// and resolve addresses that may not yet have landed in state,
	// such as for CAAS services.
	retryFactory func() retry.CallArgs

	// lookupHost is a function for returning a list of IP addresses in
	// string form that correspond to an input host/address.
	lookupHost func(string) ([]string, error)

	unit          *state.Unit
	app           *state.Application
	defaultEgress []string
	bindings      map[string]string
}

// NewNetworkInfo initialises and returns a new NetworkInfo
// based on the input state and unit tag.
func NewNetworkInfo(st *state.State, tag names.UnitTag) (NetworkInfo, error) {
	n, err := NewNetworkInfoForStrategy(st, tag, defaultRetryFactory, net.LookupHost)
	return n, errors.Trace(err)
}

// NewNetworkInfoWithBehaviour initialises and returns a new NetworkInfo
// based on the input state and unit tag, allowing further specification of
// behaviour via the input retry factory and host resolver.
func NewNetworkInfoForStrategy(
	st *state.State, tag names.UnitTag, retryFactory func() retry.CallArgs, lookupHost func(string) ([]string, error),
) (NetworkInfo, error) {
	unit, err := st.Unit(tag.Id())
	if err != nil {
		return nil, errors.Trace(err)
	}

	app, err := unit.Application()
	if err != nil {
		return nil, errors.Trace(err)
	}

	bindings, err := app.EndpointBindings()
	if err != nil {
		return nil, errors.Trace(err)
	}

	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	cfg, err := model.ModelConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}

	base := &NetworkInfoBase{
		st:            st,
		unit:          unit,
		app:           app,
		bindings:      bindings.Map(),
		defaultEgress: cfg.EgressSubnets(),
		retryFactory:  retryFactory,
		lookupHost:    lookupHost,
	}

	var netInfo NetworkInfo
	if unit.ShouldBeAssigned() {
		netInfo, err = newNetworkInfoIAAS(base)
	} else {
		netInfo, err = newNetworkInfoCAAS(base)
	}
	return netInfo, errors.Trace(err)
}

// validateEndpoints returns the endpoints from the input slice that are
// valid for the unit.
// Any invalid endpoints are indicated as errors in the returned results.
func (n *NetworkInfoBase) validateEndpoints(endpoints []string) (set.Strings, params.NetworkInfoResults) {
	valid := set.NewStrings()
	result := params.NetworkInfoResults{Results: make(map[string]params.NetworkInfoResult)}

	// For each of the endpoints in the request, get the bound space and
	// initialise the endpoint egress map with the model's configured
	// egress subnets. Keep track of the spaces that we observe.
	for _, endpoint := range endpoints {
		if _, ok := n.bindings[endpoint]; ok {
			valid.Add(endpoint)
		} else {
			err := errors.NotValidf("undefined for unit charm: endpoint %q", endpoint)
			result.Results[endpoint] = params.NetworkInfoResult{Error: apiservererrors.ServerError(err)}
		}
	}

	return valid, result
}

// getRelationAndEndpointName returns the relation for the input ID
// and the name of the endpoint used by the relation.
func (n *NetworkInfoBase) getRelationAndEndpointName(relationID int) (*state.Relation, string, error) {
	rel, err := n.st.Relation(relationID)
	if err != nil {
		return nil, "", errors.Trace(err)
	}

	endpoint, err := rel.Endpoint(n.unit.ApplicationName())
	if err != nil {
		return nil, "", errors.Trace(err)
	}

	return rel, endpoint.Name, nil
}

// maybeGetUnitAddress returns an address for the member unit if either the
// input relation is cross-model and pollAddr is passed as true.
// The unit public address is preferred, but we will fall back to the private
// address if it does not become available in the polling window.
func (n *NetworkInfoBase) maybeGetUnitAddress(rel *state.Relation) (corenetwork.SpaceAddresses, error) {
	_, crossModel, err := rel.RemoteApplication()
	if err != nil {
		return nil, errors.Trace(err)
	}
	if !crossModel {
		return nil, nil
	}

	address, err := n.pollForAddress(n.unit.PublicAddress)
	if err != nil {
		logger.Warningf(
			"no public address for unit %q in cross model relation %q, will use private address", n.unit.Name(), rel)
	} else if address.Value != "" {
		return corenetwork.SpaceAddresses{address}, nil
	}

	address, err = n.pollForAddress(n.unit.PrivateAddress)
	if err != nil {
		logger.Warningf("no private address for unit %q in relation %q", n.unit.Name(), rel)
	} else if address.Value != "" {
		return corenetwork.SpaceAddresses{address}, nil
	}

	return nil, nil
}

// getEgressForRelation returns any explicitly defined egress subnets
// for the relation, falling back to configured model egress.
// If there are none, it attempts to resolve a subnet from the input
// ingress addresses.
func (n *NetworkInfoBase) getEgressForRelation(
	rel *state.Relation, ingress corenetwork.SpaceAddresses,
) ([]string, error) {
	egressSubnets, err := state.NewRelationEgressNetworks(n.st).Networks(rel.Tag().Id())
	if err != nil {
		if !errors.IsNotFound(err) {
			return nil, errors.Trace(err)
		}
	}

	if egressSubnets != nil {
		egress := egressSubnets.CIDRS()
		if len(egress) > 0 {
			return egress, nil
		}
	}

	if len(n.defaultEgress) > 0 {
		return n.defaultEgress, nil
	}

	return subnetsForAddresses(ingress.Values()), nil
}

func (n *NetworkInfoBase) resolveResultHostNames(netInfoResult params.NetworkInfoResult) params.NetworkInfoResult {
	// Maintain a cache of host-name -> address resolutions.
	resolved := make(map[string]string)
	addressForHost := func(hostName string) string {
		resolvedAddr, ok := resolved[hostName]
		if !ok {
			resolvedAddr = n.resolveHostAddress(hostName)
			resolved[hostName] = resolvedAddr
		}
		return resolvedAddr
	}

	// Resolve addresses in Info.
	for i, info := range netInfoResult.Info {
		for j, addr := range info.Addresses {
			if ip := net.ParseIP(addr.Address); ip == nil {
				// If the address is not an IP, we assume it is a host name.
				addr.Hostname = addr.Address
				addr.Address = addressForHost(addr.Hostname)
				netInfoResult.Info[i].Addresses[j] = addr
			}
		}
	}

	// Resolve addresses in IngressAddresses.
	// This is slightly different to the addresses above in that we do not
	// include anything that does not resolve to a usable address.
	var newIngress []string
	for _, addr := range netInfoResult.IngressAddresses {
		if ip := net.ParseIP(addr); ip != nil {
			newIngress = append(newIngress, addr)
			continue
		}
		if ipAddr := addressForHost(addr); ipAddr != "" {
			newIngress = append(newIngress, ipAddr)
		}
	}
	netInfoResult.IngressAddresses = newIngress

	return netInfoResult
}

func (n *NetworkInfoBase) resolveHostAddress(hostName string) string {
	resolved, err := n.lookupHost(hostName)
	if err != nil {
		logger.Errorf("resolving %q: %v", hostName, err)
		return ""
	}

	// This preserves prior behaviour from when resolution was done client-side
	// within the network-get tool.
	// This check is probably no longer necessary, but is preserved here
	// conservatively.
	for _, addr := range resolved {
		if ip := net.ParseIP(addr); ip != nil && !ip.IsLoopback() {
			return addr
		}
	}

	if len(resolved) == 0 {
		logger.Warningf("no addresses resolved for host %q", hostName)
	} else {
		// If we got results, but they were all filtered out, then we need to
		// help out operators with some advice.
		logger.Warningf(
			"no usable addresses resolved for host %q\n\t"+
				"resolved: %v\n\t"+
				"consider editing the hosts file, or changing host resolution order via /etc/nsswitch.conf",
			hostName,
			resolved,
		)
	}

	return ""
}

// subnetsForAddresses wraps the core/network method of the same name,
// limiting the return to container at most one result.
// TODO (manadart 2020-11-19): This preserves prior behaviour,
// but should we just return them all?
func subnetsForAddresses(addrs []string) []string {
	if egress := corenetwork.SubnetsForAddresses(addrs); len(egress) > 0 {
		return egress[:1]
	}
	return nil
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
		return !corenetwork.IsNoAddressError(err)
	}
	return address, retry.Call(retryArg)
}

func uniqueNetworkInfoResults(info params.NetworkInfoResults) params.NetworkInfoResults {
	for epName, res := range info.Results {
		if res.Error != nil {
			continue
		}
		res.IngressAddresses = uniqueStringsPreservingOrder(res.IngressAddresses)
		res.EgressSubnets = uniqueStringsPreservingOrder(res.EgressSubnets)
		for infoIdx, info := range res.Info {
			res.Info[infoIdx].Addresses = uniqueInterfaceAddresses(info.Addresses)
		}
		info.Results[epName] = res
	}

	return info
}

func uniqueStringsPreservingOrder(values []string) []string {
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

func uniqueInterfaceAddresses(addrList []params.InterfaceAddress) []params.InterfaceAddress {
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
