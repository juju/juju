// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"sort"
	"strconv"

	"github.com/juju/utils/set"
)

// PortSet is a set-like container of Port values.
type PortSet struct {
	values map[string]set.Ints
}

// NewPortSet creates a map of protocols to sets of stringified port numbers.
func NewPortSet(portRanges ...PortRange) PortSet {
	var result PortSet
	result.values = make(map[string]set.Ints)
	result.AddRanges(portRanges...)
	return result
}

// Size returns the number of ports in the set.
func (ps PortSet) Size() int {
	size := 0
	for _, ports := range ps.values {
		size += len(ports)
	}
	return size
}

// IsEmpty returns true if the PortSet is empty.
func (ps PortSet) IsEmpty() bool {
	return len(ps.values) == 0
}

// Values returns a list of all the ports in the set.
func (ps PortSet) Values() []Port {
	return ps.Ports()
}

// Protocols returns a list of protocols known to the PortSet.
func (ps PortSet) Protocols() []string {
	var result []string
	for key := range ps.values {
		result = append(result, key)
	}
	return result
}

// PortRanges returns a list of all the port ranges in the set for the
// given protocols. If no protocols are provided all known protocols in
// the set are used.
func (ps PortSet) PortRanges(protocols ...string) []PortRange {
	if len(protocols) == 0 {
		protocols = ps.Protocols()
	}

	var result []PortRange
	for _, protocol := range protocols {
		ranges := collapsePorts(protocol, ps.PortNumbers(protocol)...)
		result = append(result, ranges...)
	}
	return result
}

func collapsePorts(protocol string, ports ...int) (result []PortRange) {
	if len(ports) == 0 {
		return nil
	}

	sort.Ints(ports)

	fromPort := 0
	toPort := 0
	for _, port := range ports {
		if fromPort == 0 {
			// new port range
			fromPort = port
			toPort = port
		} else if port == toPort+1 {
			// continuing port range
			toPort = port
		} else {
			// break in port range
			result = append(result, PortRange{
				Protocol: protocol,
				FromPort: fromPort,
				ToPort:   toPort,
			})
			fromPort = port
			toPort = port
		}
	}
	result = append(result, PortRange{
		Protocol: protocol,
		FromPort: fromPort,
		ToPort:   toPort,
	})
	return
}

// PortNumbers returns a list of all the port numbers in the set for
// the given protocols. If no protocols are provided then all known
// protocols in the set are used.
func (ps PortSet) Ports(protocols ...string) []Port {
	if len(protocols) == 0 {
		protocols = ps.Protocols()
	}

	var results []Port
	for _, portRange := range ps.PortRanges(protocols...) {
		for p := portRange.FromPort; p <= portRange.ToPort; p++ {
			results = append(results, Port{portRange.Protocol, p})
		}
	}
	return results
}

// PortNumbers returns a list of all the port numbers in the set for
// the given protocol.
func (ps PortSet) PortNumbers(protocol string) []int {
	ports, ok := ps.values[protocol]
	if !ok {
		return nil
	}
	return ports.Values()
}

// PortStrings returns a list of stringified ports in the set
// for the given protocol. This is strictly a convenience method
// for situations where another API requires a list of strings.
func (ps PortSet) PortStrings(protocol string) []string {
	ports, ok := ps.values[protocol]
	if !ok {
		return nil
	}
	var result []string
	for _, port := range ports.Values() {
		portStr := strconv.Itoa(port)
		result = append(result, portStr)
	}
	return result
}

// Add adds a port to the PortSet.
func (ps *PortSet) Add(protocol string, port int) {
	if ps.values == nil {
		panic("uninitalised set")
	}
	ports, ok := ps.values[protocol]
	if !ok {
		ps.values[protocol] = set.NewInts(port)
	} else {
		ports.Add(port)
	}
}

// AddRanges adds port ranges to the PortSet.
func (ps *PortSet) AddRanges(portRanges ...PortRange) {
	for _, portRange := range portRanges {
		for p := portRange.FromPort; p <= portRange.ToPort; p++ {
			ps.Add(portRange.Protocol, p)
		}
	}
}

// Remove removes the given port from the set.
func (ps *PortSet) Remove(protocol string, port int) {
	ports, ok := ps.values[protocol]
	if ok {
		ports.Remove(port)
	}
}

// RemoveRanges removes all ports in the given PortRange values
// from the set.
func (ps *PortSet) RemoveRanges(portRanges ...PortRange) {
	for _, portRange := range portRanges {
		_, ok := ps.values[portRange.Protocol]
		if ok {
			for p := portRange.FromPort; p <= portRange.ToPort; p++ {
				ps.Remove(portRange.Protocol, p)
			}
		}
	}
}

// Contains returns true if the provided port is in the set.
func (ps *PortSet) Contains(protocol string, port int) bool {
	ports, ok := ps.values[protocol]
	if !ok {
		return false
	}
	return ports.Contains(port)
}

// ContainsRanges returns true if the provided port ranges are
// in the set.
func (ps *PortSet) ContainsRanges(portRanges ...PortRange) bool {
	for _, portRange := range portRanges {
		ports, ok := ps.values[portRange.Protocol]
		if !ok {
			return false
		}
		for p := portRange.FromPort; p <= portRange.ToPort; p++ {
			if !ports.Contains(p) {
				return false
			}
		}
	}
	return true
}

// Union returns a new PortSet of the shared values
// that are common between both PortSets.
func (ps PortSet) Union(other PortSet) PortSet {
	result := NewPortSet()
	for protocol, value := range ps.values {
		result.values[protocol] = value.Union(nil)
	}
	for protocol, value := range other.values {
		ports, ok := result.values[protocol]
		if !ok {
			value = nil
		}
		result.values[protocol] = ports.Union(value)
	}
	return result
}

// Intersection returns a new PortSet of the values that are in both
// this set and the other, but not in just one of either.
func (ps PortSet) Intersection(other PortSet) PortSet {
	result := NewPortSet()
	for protocol, value := range ps.values {
		if ports, ok := other.values[protocol]; ok {
			// For PortSet, a protocol without any associated ports
			// doesn't make a lot of sense. It's also a waste of space.
			// Consequently, if the intersection for a protocol is empty
			// then we simply skip it.
			if newValue := value.Intersection(ports); !newValue.IsEmpty() {
				result.values[protocol] = newValue
			}
		}
	}
	return result
}

// Difference returns a new PortSet of the values
// that are not in the other PortSet.
func (ps PortSet) Difference(other PortSet) PortSet {
	result := NewPortSet()
	for protocol, value := range ps.values {
		if ports, ok := other.values[protocol]; ok {
			// For PortSet, a protocol without any associated ports
			// doesn't make a lot of sense. It's also a waste of space.
			// Consequently, if the difference for a protocol is empty
			// then we simply skip it.
			if newValue := value.Difference(ports); !newValue.IsEmpty() {
				result.values[protocol] = newValue
			}
		} else {
			result.values[protocol] = value
		}
	}
	return result
}
