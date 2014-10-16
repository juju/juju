// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"

	"github.com/juju/errors"
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

// PortRange represents a single range of ports.
type PortRange struct {
	FromPort int
	ToPort   int
	Protocol string
}

// IsValid determines if the port range is valid.
func (p PortRange) Validate() error {
	proto := strings.ToLower(p.Protocol)
	if proto != "tcp" && proto != "udp" {
		return errors.Errorf(`invalid protocol %q, expected "tcp" or "udp"`, proto)
	}
	err := errors.Errorf(
		"invalid port range %d-%d/%s",
		p.FromPort,
		p.ToPort,
		p.Protocol,
	)
	switch {
	case p.FromPort > p.ToPort:
		return err
	case p.FromPort < 1 || p.FromPort > 65535:
		return err
	case p.ToPort < 1 || p.ToPort > 65535:
		return err
	}
	return nil
}

// ConflictsWith determines if the two port ranges conflict.
func (a PortRange) ConflictsWith(b PortRange) bool {
	if a.Protocol != b.Protocol {
		return false
	}
	return a.ToPort >= b.FromPort && b.ToPort >= a.FromPort
}

func (p PortRange) String() string {
	if p.FromPort == p.ToPort {
		return fmt.Sprintf("%d/%s", p.FromPort, strings.ToLower(p.Protocol))
	}
	return fmt.Sprintf("%d-%d/%s", p.FromPort, p.ToPort, strings.ToLower(p.Protocol))
}

type portRangeSlice []PortRange

func (p portRangeSlice) Len() int      { return len(p) }
func (p portRangeSlice) Swap(i, j int) { p[i], p[j] = p[j], p[i] }
func (p portRangeSlice) Less(i, j int) bool {
	p1 := p[i]
	p2 := p[j]
	if p1.Protocol != p2.Protocol {
		return p1.Protocol < p2.Protocol
	}
	if p1.FromPort != p2.FromPort {
		return p1.FromPort < p2.FromPort
	}
	return p1.ToPort < p2.ToPort
}

// SortPortRanges sorts the given ports, first by protocol, then by number.
func SortPortRanges(portRanges []PortRange) {
	sort.Sort(portRangeSlice(portRanges))
}

// PortRangesToPorts is a temporary function converting a slice of port ranges
// to a slice of ports. It is here to fill the gap caused
// by environments dealing with port ranges and the firewaller still
// dealing with individual ports
// TODO (domas) 2014-07-31: remove this once firewaller is capable of
// handling port ranges
func PortRangesToPorts(portRanges []PortRange) (result []Port) {
	for _, portRange := range portRanges {
		for p := portRange.FromPort; p <= portRange.ToPort; p++ {
			result = append(result, Port{portRange.Protocol, p})
		}
	}
	return
}

// PortsToPortRanges is a temporary function converting a slice of ports to
// a slice of port ranges. It is here to fill the gap caused by environments
// handling port ranges and firewaller still dealing with individual ports.
// TODO (domas) 2014-07-31: remove this once firewaller is capable of
// handling port ranges
func PortsToPortRanges(ports []Port) (result []PortRange) {
	for _, p := range ports {
		result = append(result, PortRange{p.Number, p.Number, p.Protocol})
	}
	return
}

// CollapsePorts collapses a slice of ports into port ranges.
func CollapsePorts(ports []Port) (result []PortRange) {
	SortPorts(ports)
	fromPort := 0
	toPort := 0
	protocol := ""
	for _, p := range ports {
		if fromPort == 0 {
			// new port range
			fromPort = p.Number
			toPort = p.Number
			protocol = p.Protocol
		} else if p.Number == toPort+1 && protocol == p.Protocol {
			// continuing port range
			toPort = p.Number
		} else {
			// break in port range
			result = append(result,
				PortRange{
					Protocol: protocol,
					FromPort: fromPort,
					ToPort:   toPort,
				})
			fromPort = p.Number
			toPort = p.Number
			protocol = p.Protocol
		}
	}
	if fromPort != 0 {
		result = append(result, PortRange{
			Protocol: protocol,
			FromPort: fromPort,
			ToPort:   toPort,
		})

	}
	return
}
