// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strconv"

	"github.com/juju/collections/set"

	"github.com/juju/juju/internal/errors"
)

// HostPort describes methods on an object that
// represents a network connection endpoint.
type HostPort interface {
	Address
	Port() int
}

// HostPorts derives from a slice of HostPort
// and allows bulk operations on its members.
type HostPorts []HostPort

// FilterUnusable returns a copy of the receiver HostPorts after removing
// any addresses unlikely to be usable (ScopeMachineLocal or ScopeLinkLocal).
func (hps HostPorts) FilterUnusable() HostPorts {
	filtered := make(HostPorts, 0, len(hps))
	for _, addr := range hps {
		switch addr.AddressScope() {
		case ScopeMachineLocal, ScopeLinkLocal:
			continue
		}
		filtered = append(filtered, addr)
	}
	return filtered
}

// Strings returns the HostPorts as a slice of
// strings suitable for passing to net.Dial.
func (hps HostPorts) Strings() []string {
	result := make([]string, len(hps))
	for i, addr := range hps {
		result[i] = DialAddress(addr)
	}
	return result
}

// Unique returns a copy of the receiver HostPorts with duplicate endpoints
// removed. Note that this only applies to dial addresses; spaces are ignored.
func (hps HostPorts) Unique() HostPorts {
	results := make([]HostPort, 0, len(hps))
	seen := set.NewStrings()

	for _, addr := range hps {
		da := DialAddress(addr)
		if seen.Contains(da) {
			continue
		}

		seen.Add(da)
		results = append(results, addr)
	}
	return results
}

// PrioritizedForScope orders the HostPorts by best match for the input scope
// matching function and returns them in NetAddr form.
// If there are no suitable addresses then an empty slice is returned.
func (hps HostPorts) PrioritizedForScope(getMatcher ScopeMatchFunc) []string {
	indexes := indexesByScopeMatch(hps, getMatcher)
	out := make([]string, len(indexes))
	for i, index := range indexes {
		out[i] = DialAddress(hps[index])
	}
	return out
}

// DialAddress returns a string value for the input HostPort,
// suitable for passing as an argument to net.Dial.
func DialAddress(a HostPort) string {
	return net.JoinHostPort(a.Host(), strconv.Itoa(a.Port()))
}

// NetPort represents a network port.
// TODO (manadart 2019-08-15): Finish deprecation of `Port` and use that name.
type NetPort int

// Port returns the port number.
func (p NetPort) Port() int {
	return int(p)
}

// MachineHostPort associates a space-unaware address with a port.
type MachineHostPort struct {
	MachineAddress
	NetPort
}

var _ HostPort = MachineHostPort{}

// String implements Stringer.
func (hp MachineHostPort) String() string {
	return DialAddress(hp)
}

// GoString implements fmt.GoStringer.
func (hp MachineHostPort) GoString() string {
	return hp.String()
}

// MachineHostPorts is a slice of MachineHostPort
// allowing use as a receiver for bulk operations.
type MachineHostPorts []MachineHostPort

// HostPorts returns the slice as a new slice of the HostPort indirection.
func (hp MachineHostPorts) HostPorts() HostPorts {
	addrs := make(HostPorts, len(hp))
	for i, hp := range hp {
		addrs[i] = hp
	}
	return addrs
}

// NewMachineHostPorts creates a list of MachineHostPorts
// from each given string address and port.
func NewMachineHostPorts(port int, addresses ...string) MachineHostPorts {
	hps := make(MachineHostPorts, len(addresses))
	for i, addr := range addresses {
		hps[i] = MachineHostPort{
			MachineAddress: NewMachineAddress(addr),
			NetPort:        NetPort(port),
		}
	}
	return hps
}

// ParseMachineHostPort converts a string containing a
// single host and port value to a MachineHostPort.
func ParseMachineHostPort(hp string) (*MachineHostPort, error) {
	host, port, err := net.SplitHostPort(hp)
	if err != nil {
		return nil, errors.Errorf("cannot parse %q as address:port: %w", hp, err)
	}
	numPort, err := strconv.Atoi(port)
	if err != nil {
		return nil, errors.Errorf("cannot parse %q port: %w", hp, err)
	}
	return &MachineHostPort{
		MachineAddress: NewMachineAddress(host),
		NetPort:        NetPort(numPort),
	}, nil
}

// CollapseToHostPorts returns the input nested slice of MachineHostPort
// as a flat slice of HostPort, preserving the order.
func CollapseToHostPorts(serversHostPorts []MachineHostPorts) HostPorts {
	var collapsed HostPorts
	for _, hps := range serversHostPorts {
		for _, hp := range hps {
			collapsed = append(collapsed, hp)
		}
	}
	return collapsed
}

// ProviderHostPort associates a provider/space aware address with a port.
type ProviderHostPort struct {
	ProviderAddress
	NetPort
}

var _ HostPort = ProviderHostPort{}

// String implements Stringer.
func (hp ProviderHostPort) String() string {
	return DialAddress(hp)
}

// GoString implements fmt.GoStringer.
func (hp ProviderHostPort) GoString() string {
	return hp.String()
}

// ProviderHostPorts is a slice of ProviderHostPort
// allowing use as a receiver for bulk operations.
type ProviderHostPorts []ProviderHostPort

// Addresses extracts the ProviderAddress from each member of the collection,
// then returns them as a new collection, effectively discarding the port.
func (hp ProviderHostPorts) Addresses() ProviderAddresses {
	addrs := make(ProviderAddresses, len(hp))
	for i, hp := range hp {
		addrs[i] = hp.ProviderAddress
	}
	return addrs
}

// HostPorts returns the slice as a new slice of the HostPort indirection.
func (hp ProviderHostPorts) HostPorts() HostPorts {
	addrs := make(HostPorts, len(hp))
	for i, hp := range hp {
		addrs[i] = hp
	}
	return addrs
}

// ParseProviderHostPorts creates a slice of MachineHostPorts parsing
// each given string containing address:port.
// An error is returned if any string cannot be parsed as a MachineHostPort.
func ParseProviderHostPorts(hostPorts ...string) (ProviderHostPorts, error) {
	hps := make(ProviderHostPorts, len(hostPorts))
	for i, hp := range hostPorts {
		mhp, err := ParseMachineHostPort(hp)
		if err != nil {
			return nil, errors.Capture(err)
		}
		hps[i] = ProviderHostPort{
			ProviderAddress: ProviderAddress{MachineAddress: mhp.MachineAddress},
			NetPort:         mhp.NetPort,
		}
	}
	return hps, nil
}

// SpaceHostPort associates a space ID decorated address with a port.
type SpaceHostPort struct {
	SpaceAddress
	NetPort
}

var _ HostPort = SpaceHostPort{}

// String implements Stringer.
func (hp SpaceHostPort) String() string {
	return DialAddress(hp)
}

// GoString implements fmt.GoStringer.
func (hp SpaceHostPort) GoString() string {
	return hp.String()
}

// Less reports whether hp is ordered before hp2
// according to the criteria used by SortHostPorts.
func (hp SpaceHostPort) Less(hp2 SpaceHostPort) bool {
	order1 := SortOrderMostPublic(hp)
	order2 := SortOrderMostPublic(hp2)
	if order1 == order2 {
		if hp.SpaceAddress.Value == hp2.SpaceAddress.Value {
			return hp.Port() < hp2.Port()
		}
		return hp.SpaceAddress.Value < hp2.SpaceAddress.Value
	}
	return order1 < order2
}

// SpaceHostPorts is a slice of SpaceHostPort
// allowing use as a receiver for bulk operations.
type SpaceHostPorts []SpaceHostPort

// NewSpaceHostPorts creates a list of SpaceHostPorts
// from each input string address and port.
func NewSpaceHostPorts(port int, addresses ...string) SpaceHostPorts {
	hps := make(SpaceHostPorts, len(addresses))
	for i, addr := range addresses {
		hps[i] = SpaceHostPort{
			SpaceAddress: NewSpaceAddress(addr),
			NetPort:      NetPort(port),
		}
	}
	return hps
}

// HostPorts returns the slice as a new slice of the HostPort indirection.
func (hps SpaceHostPorts) HostPorts() HostPorts {
	addrs := make(HostPorts, len(hps))
	for i, hp := range hps {
		addrs[i] = hp
	}
	return addrs
}

// InSpaces returns the SpaceHostPorts that are in the input spaces.
func (hps SpaceHostPorts) InSpaces(spaces ...SpaceInfo) (SpaceHostPorts, bool) {
	if len(spaces) == 0 {
		logger.Errorf(context.TODO(), "host ports not filtered - no spaces given.")
		return hps, false
	}

	spaceInfos := SpaceInfos(spaces)
	var selectedHostPorts SpaceHostPorts
	for _, hp := range hps {
		if space := spaceInfos.GetByID(hp.SpaceID); space != nil {
			logger.Debugf(context.TODO(), "selected %q as a hostPort in space %q", hp.Value, space.Name)
			selectedHostPorts = append(selectedHostPorts, hp)
		}
	}

	if len(selectedHostPorts) > 0 {
		return selectedHostPorts, true
	}

	logger.Errorf(context.TODO(), "no hostPorts found in spaces %s", spaceInfos)
	return hps, false
}

// AllMatchingScope returns the HostPorts that best satisfy the input scope
// matching function, as strings usable as arguments to net.Dial.
func (hps SpaceHostPorts) AllMatchingScope(getMatcher ScopeMatchFunc) []string {
	indexes := indexesForScope(hps, getMatcher)
	out := make([]string, 0, len(indexes))
	for _, index := range indexes {
		out = append(out, DialAddress(hps[index]))
	}
	return out
}

func (hps SpaceHostPorts) Len() int      { return len(hps) }
func (hps SpaceHostPorts) Swap(i, j int) { hps[i], hps[j] = hps[j], hps[i] }
func (hps SpaceHostPorts) Less(i, j int) bool {
	return hps[i].Less(hps[j])
}

// SpaceAddressesWithPort returns the input SpaceAddresses
// all associated with the given port.
func SpaceAddressesWithPort(addrs SpaceAddresses, port int) SpaceHostPorts {
	hps := make(SpaceHostPorts, len(addrs))
	for i, addr := range addrs {
		hps[i] = SpaceHostPort{
			SpaceAddress: addr,
			NetPort:      NetPort(port),
		}
	}
	return hps
}

// EnsureFirstHostPort scans the given list of SpaceHostPorts and if
// "first" is found, it moved to index 0. Otherwise, if "first" is not
// in the list, it's inserted at index 0.
func EnsureFirstHostPort(first SpaceHostPort, hps SpaceHostPorts) SpaceHostPorts {
	var result []SpaceHostPort
	found := false
	for _, hp := range hps {
		if hp.String() == first.String() && !found {
			// Found, so skip it.
			found = true
			continue
		}
		result = append(result, hp)
	}
	// Insert it at the top.
	result = append(SpaceHostPorts{first}, result...)
	return result
}

// HostsPortsSlice is used to sort a slice of [SpaceHostPorts].
type HostsPortsSlice []SpaceHostPorts

// DupeAndSort returns a sorted copy of in.
func DupeAndSort(in []SpaceHostPorts) []SpaceHostPorts {
	result := make([]SpaceHostPorts, len(in))

	for i, val := range in {
		var inner SpaceHostPorts
		inner = append(inner, val...)
		sort.Sort(inner)
		result[i] = inner
	}
	sort.Sort(HostsPortsSlice(result))
	return result
}

func (hp HostsPortsSlice) Len() int      { return len(hp) }
func (hp HostsPortsSlice) Swap(i, j int) { hp[i], hp[j] = hp[j], hp[i] }
func (hp HostsPortsSlice) Less(i, j int) bool {
	lhs := (hostPortsSlice)(hp[i]).String()
	rhs := (hostPortsSlice)(hp[j]).String()
	return lhs < rhs
}

func (hp hostPortsSlice) String() string {
	var result string
	for _, val := range hp {
		result += fmt.Sprintf("%s-%d ", val.SpaceAddress, val.Port())
	}
	return result
}

type hostPortsSlice []SpaceHostPort
