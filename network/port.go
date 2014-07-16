// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"fmt"
	"net"
	"sort"
	"strconv"
)

// Port identifies a network port number for a particular protocol.
type Port struct {
	Protocol string
	Number   int
}

// String implements Stringer.
func (p Port) String() string {
	return fmt.Sprintf("%d/%s", p.Number, p.Protocol)
}

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

type portSlice []Port

func (p portSlice) Len() int      { return len(p) }
func (p portSlice) Swap(i, j int) { p[i], p[j] = p[j], p[i] }
func (p portSlice) Less(i, j int) bool {
	p1 := p[i]
	p2 := p[j]
	if p1.Protocol != p2.Protocol {
		return p1.Protocol < p2.Protocol
	}
	return p1.Number < p2.Number
}

// SortPorts sorts the given ports, first by protocol, then by number.
func SortPorts(ports []Port) {
	sort.Sort(portSlice(ports))
}

type hostPortPreferringIPv4Slice []HostPort

func (h hostPortPreferringIPv4Slice) Len() int      { return len(h) }
func (h hostPortPreferringIPv4Slice) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h hostPortPreferringIPv4Slice) Less(i, j int) bool {
	addr1 := h[i].Address
	addr2 := h[j].Address
	if addr1.Type == HostName {
		// Prefer hostnames on top, if possible.
		return addr2.Type != HostName
	}
	if addr1.Type == IPv4Address || addr2.Type == IPv4Address {
		// Prefer IPv4 addresses to IPv6 ones.
		return addr1.Type == IPv4Address
	}
	return addr1.Value == addr2.Value
}

type hostPortPreferringIPv6Slice struct {
	hostPortPreferringIPv4Slice
}

func (h hostPortPreferringIPv6Slice) Less(i, j int) bool {
	addr1 := h.hostPortPreferringIPv4Slice[i].Address
	addr2 := h.hostPortPreferringIPv4Slice[j].Address
	if addr1.Type == HostName {
		// Prefer hostnames on top, if possible.
		return addr2.Type != HostName
	}
	if addr1.Type == IPv6Address || addr2.Type == IPv6Address {
		// Prefer IPv6 addresses to IPv4 ones.
		return addr1.Type == IPv6Address
	}
	return addr1.Value == addr2.Value
}

// SortHostPorts sorts the given HostPort slice, putting hostnames on
// top and depending on the preferIPv6 flag either IPv6 or IPv4
// addresses after that.
func SortHostPorts(hps []HostPort, preferIPv6 bool) {
	if preferIPv6 {
		sort.Sort(hostPortPreferringIPv6Slice{hostPortPreferringIPv4Slice(hps)})
	} else {
		sort.Sort(hostPortPreferringIPv4Slice(hps))
	}
}
