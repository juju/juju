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

type hostPortsByPreference struct {
	hostPorts  []HostPort
	order      []int
	preference AddressType
}

func (h hostPortsByPreference) Len() int {
	return len(h.hostPorts)
}

func (h hostPortsByPreference) Swap(i, j int) {
	h.hostPorts[i], h.hostPorts[j] = h.hostPorts[j], h.hostPorts[i]
	h.order[i], h.order[j] = h.order[j], h.order[i]
}

func alternateIPAddressType(t AddressType) AddressType {
	if t == IPv4Address {
		return IPv6Address
	}
	return IPv4Address
}

func (h hostPortsByPreference) Less(i, j int) bool {
	addr1 := h.hostPorts[i].Address
	addr2 := h.hostPorts[j].Address
	switch addr1.Type {
	case addr2.Type:
		// Same-type addresses are kept in original order.
		return h.order[i] < h.order[j]
	case HostName:
		// Hostnames come before non-hostnames.
		return true
	case h.preference:
		return addr2.Type == alternateIPAddressType(h.preference)
	}
	return false
}

// SortHostPorts sorts the given HostPort slice, putting hostnames on
// top and depending on the preferIPv6 flag either IPv6 or IPv4
// addresses after that. Order of same-type addresses are kept stable.
func SortHostPorts(hps []HostPort, preferIPv6 bool) {
	h := hostPortsByPreference{
		hostPorts: hps,
		order:     make([]int, len(hps)),
	}
	for i := range h.order {
		h.order[i] = i
	}
	if preferIPv6 {
		h.preference = IPv6Address
	} else {
		h.preference = IPv4Address
	}
	sort.Sort(h)
}
