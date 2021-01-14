// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"net"
	"time"

	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/retry"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	corenetwork "github.com/juju/juju/core/network"
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

	// Initialise the bindings map with all application endpoints.
	// This will include those for which there is no explicit binding,
	// such as the juju-info endpoint.
	endpoints, err := app.Endpoints()
	if err != nil {
		return nil, errors.Trace(err)
	}
	allBindings := make(map[string]string)
	for _, ep := range endpoints {
		allBindings[ep.Name] = corenetwork.AlphaSpaceId
	}

	// Now fill in those that are bound.
	bindings, err := app.EndpointBindings()
	if err != nil {
		return nil, errors.Trace(err)
	}
	for ep, space := range bindings.Map() {
		allBindings[ep] = space
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
		bindings:      allBindings,
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

	for _, endpoint := range endpoints {
		if err := n.validateEndpoint(endpoint); err != nil {
			result.Results[endpoint] = params.NetworkInfoResult{Error: common.ServerError(err)}
			continue
		}
		valid.Add(endpoint)
	}

	return valid, result
}

func (n *NetworkInfoBase) validateEndpoint(endpoint string) error {
	if _, ok := n.bindings[endpoint]; !ok {
		return errors.NotValidf("undefined for unit charm: endpoint %q", endpoint)
	}
	return nil
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

// resolveResultInfoHostNames returns a new NetworkInfoResult with host names
// in the `Info` member resolved to IP addresses where possible.
func (n *NetworkInfoBase) resolveResultInfoHostNames(netInfo params.NetworkInfoResult) params.NetworkInfoResult {
	for i, info := range netInfo.Info {
		for j, addr := range info.Addresses {
			if ip := net.ParseIP(addr.Address); ip == nil {
				// If the address is not an IP, we assume it is a host name.
				addr.Hostname = addr.Address
				addr.Address = n.resolveHostAddress(addr.Hostname)
				netInfo.Info[i].Addresses[j] = addr
			}
		}
	}
	return netInfo
}

// resolveResultIngressHostNames returns a new NetworkInfoResult with host names
// in the `IngressAddresses` member resolved to IP addresses where possible.
// This is slightly different to the `Info` addresses above in that we do not
// include anything that does not resolve to a usable address.
func (n *NetworkInfoBase) resolveResultIngressHostNames(netInfo params.NetworkInfoResult) params.NetworkInfoResult {
	var newIngress []string
	for _, addr := range netInfo.IngressAddresses {
		if ip := net.ParseIP(addr); ip != nil {
			newIngress = append(newIngress, addr)
			continue
		}
		if ipAddr := n.resolveHostAddress(addr); ipAddr != "" {
			newIngress = append(newIngress, ipAddr)
		}
	}
	netInfo.IngressAddresses = newIngress

	return netInfo
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

var defaultRetryFactory = func() retry.CallArgs {
	return retry.CallArgs{
		Clock:       clock.WallClock,
		Delay:       3 * time.Second,
		MaxDuration: 30 * time.Second,
	}
}
