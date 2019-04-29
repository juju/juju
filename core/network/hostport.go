// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"net"
	"sort"
	"strconv"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
)

// HostPort associates an address with a port.
type HostPort struct {
	Address
	Port int
}

// NetAddr returns the host-port as an address
// suitable for calling net.Dial.
func (hp HostPort) NetAddr() string {
	return net.JoinHostPort(hp.Value, strconv.Itoa(hp.Port))
}

// String implements Stringer.
func (hp HostPort) String() string {
	return hp.NetAddr()
}

// GoString implements fmt.GoStringer.
func (hp HostPort) GoString() string {
	return hp.String()
}

// Less reports whether hp1 is ordered before hp2
// according to the criteria used by SortHostPorts.
func (hp1 HostPort) Less(hp2 HostPort) bool {
	order1 := hp1.sortOrder()
	order2 := hp2.sortOrder()
	if order1 == order2 {
		if hp1.Address.Value == hp2.Address.Value {
			return hp1.Port < hp2.Port
		}
		return hp1.Address.Value < hp2.Address.Value
	}
	return order1 < order2
}

// AddressesWithPort returns the given addresses all
// associated with the given port.
func AddressesWithPort(addrs []Address, port int) []HostPort {
	hps := make([]HostPort, len(addrs))
	for i, addr := range addrs {
		hps[i] = HostPort{
			Address: addr,
			Port:    port,
		}
	}
	return hps
}

// NewHostPorts creates a list of HostPorts from each given string
// address and port.
func NewHostPorts(port int, addresses ...string) []HostPort {
	hps := make([]HostPort, len(addresses))
	for i, addr := range addresses {
		hps[i] = HostPort{
			Address: NewAddress(addr),
			Port:    port,
		}
	}
	return hps
}

// ParseHostPorts creates a list of HostPorts parsing each given
// string containing address:port. An error is returned if any string
// cannot be parsed as HostPort.
func ParseHostPorts(hostPorts ...string) ([]HostPort, error) {
	hps := make([]HostPort, len(hostPorts))
	for i, hp := range hostPorts {
		hostport, err := ParseHostPort(hp)
		if err != nil {
			return nil, errors.Trace(err)
		}
		hps[i] = *hostport
	}
	return hps, nil
}

// ParseHostPort converts a string containing a single host and port
// value to a HostPort.
func ParseHostPort(hp string) (*HostPort, error) {
	host, port, err := net.SplitHostPort(hp)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot parse %q as address:port", hp)
	}
	numPort, err := strconv.Atoi(port)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot parse %q port", hp)
	}
	return &HostPort{
		Address: NewAddress(host),
		Port:    numPort,
	}, nil
}

// HostsWithoutPort strips the port from each HostPort, returning just
// the addresses.
func HostsWithoutPort(hps []HostPort) []Address {
	addrs := make([]Address, len(hps))
	for i, hp := range hps {
		addrs[i] = hp.Address
	}
	return addrs
}

type hostPortsPreferringIPv4Slice []HostPort

func (hp hostPortsPreferringIPv4Slice) Len() int      { return len(hp) }
func (hp hostPortsPreferringIPv4Slice) Swap(i, j int) { hp[i], hp[j] = hp[j], hp[i] }
func (hp hostPortsPreferringIPv4Slice) Less(i, j int) bool {
	return hp[i].Less(hp[j])
}

// SortHostPorts sorts the given HostPort slice according to the sortOrder of
// each HostPort's embedded Address. See Address.sortOrder() for more info.
func SortHostPorts(hps []HostPort) {
	sort.Sort(hostPortsPreferringIPv4Slice(hps))
}

var netLookupIP = net.LookupIP

// ResolveOrDropHostnames tries to resolve each address of type
// HostName (except for "localhost" - it's kept unchanged) using the
// local resolver. If successful, each IP address corresponding to the
// hostname is inserted in the same order. If not successful, a debug
// log is added and the hostname is removed from the list. Duplicated
// addresses after the resolving is done are removed.
func ResolveOrDropHostnames(hps []HostPort) []HostPort {
	uniqueAddrs := set.NewStrings()
	result := make([]HostPort, 0, len(hps))
	for _, hp := range hps {
		val := hp.Value
		if uniqueAddrs.Contains(val) {
			continue
		}
		// localhost is special - do not resolve it, because it can be
		// used both as an IPv4 or IPv6 endpoint (e.g. in IPv6-only
		// networks).
		if hp.Type != HostName || hp.Value == "localhost" {
			result = append(result, hp)
			uniqueAddrs.Add(val)
			continue
		}
		ips, err := netLookupIP(val)
		if err != nil {
			logger.Debugf("removing unresolvable address %q: %v", val, err)
			continue
		}
		for _, ip := range ips {
			if ip == nil {
				continue
			}
			addr := NewAddress(ip.String())
			if !uniqueAddrs.Contains(addr.Value) {
				result = append(result, HostPort{Address: addr, Port: hp.Port})
				uniqueAddrs.Add(addr.Value)
			}
		}
	}
	return result
}

// FilterUnusableHostPorts returns a copy of the given HostPorts after
// removing any addresses unlikely to be usable (ScopeMachineLocal or
// ScopeLinkLocal).
func FilterUnusableHostPorts(hps []HostPort) []HostPort {
	filtered := make([]HostPort, 0, len(hps))
	for _, hp := range hps {
		switch hp.Scope {
		case ScopeMachineLocal, ScopeLinkLocal:
			continue
		}
		filtered = append(filtered, hp)
	}
	return filtered
}

// HostPortsToStrings converts each HostPort to string calling its
// NetAddr() method.
func HostPortsToStrings(hps []HostPort) []string {
	result := make([]string, len(hps))
	for i, hp := range hps {
		result[i] = hp.NetAddr()
	}
	return result
}

// APIHostPortsToNoProxyString converts list of lists of NetAddrs() to
// a NoProxy-like comma separated string, ignoring local addresses
func APIHostPortsToNoProxyString(ahp [][]HostPort) string {
	noProxySet := set.NewStrings()
	for _, host := range ahp {
		for _, hp := range host {
			if hp.Address.Scope == ScopeMachineLocal ||
				hp.Address.Scope == ScopeLinkLocal {
				continue
			}
			noProxySet.Add(hp.Address.Value)
		}
	}
	return strings.Join(noProxySet.SortedValues(), ",")
}

// CollapseHostPorts returns a flattened list of HostPorts keeping the
// same order they appear in serversHostPorts.
func CollapseHostPorts(serversHostPorts [][]HostPort) []HostPort {
	var collapsed []HostPort
	for _, hps := range serversHostPorts {
		collapsed = append(collapsed, hps...)
	}
	return collapsed
}

// EnsureFirstHostPort scans the given list of HostPorts and if
// "first" is found, it moved to index 0. Otherwise, if "first" is not
// in the list, it's inserted at index 0.
func EnsureFirstHostPort(first HostPort, hps []HostPort) []HostPort {
	var result []HostPort
	found := false
	for _, hp := range hps {
		if hp.NetAddr() == first.NetAddr() && !found {
			// Found, so skip it.
			found = true
			continue
		}
		result = append(result, hp)
	}
	// Insert it at the top.
	result = append([]HostPort{first}, result...)
	return result
}

// UniqueHostPorts returns the given hostPorts after filtering out any
// duplicates, preserving the input order.
func UniqueHostPorts(hostPorts []HostPort) []HostPort {
	results := make([]HostPort, 0, len(hostPorts))

	seen := make(map[HostPort]bool, len(hostPorts))
	for _, hostPort := range hostPorts {
		if seen[hostPort] {
			continue
		}

		seen[hostPort] = true
		results = append(results, hostPort)
	}

	return results
}
