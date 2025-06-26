// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controllernode

import (
	"context"
	"net/netip"
	"strings"

	"github.com/juju/collections/set"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/logger"
)

// APIAddress represents one of the API addresses, accessible for clients
// and/or agents.
type APIAddress struct {
	// Address is the address of the API represented as "host:port" string.
	Address string
	// IsAgent indicates whether the address is available for agents.
	IsAgent bool
	// Scope is the address scope.
	Scope network.Scope
}

type APIAddresses []APIAddress

// PrioritizedForScope orders the APIAddresses by best match for the input scope
// matching function and returns them in string form, e.g. "192.168.0.54:17070".
// If there are no suitable addresses then an empty slice is returned.
func (addrs APIAddresses) PrioritizedForScope(getMatcher ScopeMatchFunc) []string {
	indexes := indexesByScopeMatch(addrs, getMatcher)
	out := make([]string, len(indexes))
	for i, index := range indexes {
		out[i] = addrs[index].Address
	}
	return out
}

// indexesByScopeMatch filters address indexes by matching scope,
// then returns them in descending order of best match.
func indexesByScopeMatch(addrs APIAddresses, matchFunc ScopeMatchFunc) []int {
	matches := filterAndCollateAddressIndexes(addrs, matchFunc)

	var prioritized []int
	for _, matchType := range scopeMatchHierarchy() {
		indexes, ok := matches[matchType]
		if ok && len(indexes) > 0 {
			prioritized = append(prioritized, indexes...)
		}
	}
	return prioritized
}

// filterAndCollateAddressIndexes filters address indexes using the input scope
// matching function, then returns the results grouped by scope match quality.
// Invalid results are omitted.
func filterAndCollateAddressIndexes(addrs APIAddresses, matchFunc ScopeMatchFunc) map[ScopeMatch][]int {
	matches := make(map[ScopeMatch][]int)
	for i, addr := range addrs {
		matchType := matchFunc(addr)
		if matchType != invalidScope {
			matches[matchType] = append(matches[matchType], i)
		}
	}
	return matches
}

// ScopeMatchCloudLocal is an address scope matching function for determining
// the extent to which the input address' scope satisfies a requirement for
// accessibility from within the local cloud.
// Machine-only addresses do not satisfy this matcher.
func ScopeMatchCloudLocal(addr APIAddress) ScopeMatch {
	addrPort, err := netip.ParseAddrPort(addr.Address)
	if err != nil {
		// TODO - log error
		return invalidScope
	}

	switch addr.Scope {
	case network.ScopeCloudLocal:
		if addrPort.Addr().Is4() {
			return exactScopeIPv4
		}
		return exactScope
	case network.ScopeFanLocal:
		if addrPort.Addr().Is4() {
			return firstFallbackScopeIPv4
		}
		return firstFallbackScope
	case network.ScopePublic, network.ScopeUnknown:
		if addrPort.Addr().Is4() {
			return secondFallbackScopeIPv4
		}
		return secondFallbackScope
	}
	return invalidScope
}

// ScopeMatchPublic is an address scope matching function for determining the
// extent to which the input address' scope satisfies a requirement for public
// accessibility.
func ScopeMatchPublic(addr APIAddress) ScopeMatch {
	scope := addr.Scope

	// If this call fails, we have a hostname, thus safe
	// to ignore the error.
	addrPort, _ := netip.ParseAddrPort(addr.Address)

	switch scope {
	case network.ScopePublic:
		if addrPort.Addr().Is4() {
			return exactScopeIPv4
		}
		return exactScope
	case network.ScopeCloudLocal:
		if addrPort.Addr().Is4() {
			return firstFallbackScopeIPv4
		}
		return firstFallbackScope
	case network.ScopeFanLocal, network.ScopeUnknown:
		if addrPort.Addr().Is4() {
			return secondFallbackScopeIPv4
		}
		return secondFallbackScope
	}
	return invalidScope
}

// ScopeMatchFunc is an alias for a function that accepts an Address,
// and returns what kind of scope match is determined by the body.
type ScopeMatchFunc = func(addr APIAddress) ScopeMatch

// ScopeMatch is a numeric designation of how well the requirement
// for satisfying a scope is met.
type ScopeMatch int

const (
	invalidScope ScopeMatch = iota
	exactScopeIPv4
	exactScope
	firstFallbackScopeIPv4
	firstFallbackScope
	secondFallbackScopeIPv4
	secondFallbackScope
)

func scopeMatchHierarchy() []ScopeMatch {
	return []ScopeMatch{
		exactScopeIPv4, exactScope,
		firstFallbackScopeIPv4, firstFallbackScope,
		secondFallbackScopeIPv4, secondFallbackScope,
	}
}

// ToNoProxyString converts list of lists of APIAddress to
// a NoProxy-like comma separated string, ignoring local addresses.
func (addrs APIAddresses) ToNoProxyString() string {
	noProxySet := set.NewStrings()
	for _, addr := range addrs {
		if addr.Scope == network.ScopeMachineLocal || addr.Scope == network.ScopeLinkLocal {
			continue
		}
		addrPort, err := netip.ParseAddrPort(addr.Address)
		if err != nil {
			// This shouldn't happen, but log it just in case.
			logger.GetLogger("juju.services.controllernode").Errorf(
				context.Background(),
				"parsing address and port %q for proxy string: %w", addr.Address, err)
		}
		noProxySet.Add(addrPort.Addr().String())
	}
	return strings.Join(noProxySet.SortedValues(), ",")
}
