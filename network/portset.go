// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"strconv"

	"github.com/juju/utils/set"
)

// PortSet is a set-like container of Port values.
type PortSet struct {
	values map[string]set.Strings
}

// NewPortSet creates a map of protocols to sets of stringified port numbers.
func NewPortSet(portRanges ...PortRange) PortSet {
	var portMap PortSet
	portMap.values = make(map[string]set.Strings)
	portMap.AddRanges(portRanges...)
	return portMap
}

// Size returns the number of ports in the set.
func (ps PortSet) Size() int {
	return len(ps.Ports())
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

// Ports returns a list of all the ports in the set for the
// given protocols. If no protocols are provided all known
// protocols in the set are used.
func (ps PortSet) Ports(protocols ...string) []Port {
	if len(protocols) == 0 {
		protocols = ps.Protocols()
	}

	var result []Port
	for _, protocol := range protocols {
		ports, ok := ps.values[protocol]
		if !ok {
			return nil
		}
		for _, port := range ports.Values() {
			portNum, _ := strconv.Atoi(port)
			result = append(result, Port{protocol, portNum})

		}
	}
	return result
}

// PortStrings returns a list of stringified ports in the set
// for the given protocol.
func (ps PortSet) PortStrings(protocol string) []string {
	ports, ok := ps.values[protocol]
	if !ok {
		return nil
	}
	return ports.Values()
}

// Add adds a Port to the PortSet.
func (ps *PortSet) Add(port Port) {
	if ps.values == nil {
		panic("uninitalised set")
	}
	portNum := strconv.Itoa(port.Number)
	ports, ok := ps.values[port.Protocol]
	if !ok {
		ps.values[port.Protocol] = set.NewStrings(portNum)
	} else {
		ports.Add(portNum)
	}
}

// AddRanges adds port ranges to the PortSet.
func (ps *PortSet) AddRanges(portRanges ...PortRange) {
	for _, port := range PortRangesToPorts(portRanges) {
		ps.Add(port)
	}
}

// Remove removes the given Port from the set.
func (ps *PortSet) Remove(port Port) {
	ports, ok := ps.values[port.Protocol]
	if ok {
		portNum := strconv.Itoa(port.Number)
		ports.Remove(portNum)
	}
}

// Contains returns true if the provided port is in the set.
func (ps *PortSet) Contains(port Port) bool {
	ports, ok := ps.values[port.Protocol]
	if !ok {
		return false
	}
	portNum := strconv.Itoa(port.Number)
	return ports.Contains(portNum)
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
		ports, ok := other.values[protocol]
		if ok {
			result.values[protocol] = value.Intersection(ports)
		}
	}
	return result
}

// Difference returns a new PortSet of the values
// that are not in the other PortSet.
func (ps PortSet) Difference(other PortSet) PortSet {
	result := NewPortSet()
	for protocol, value := range ps.values {
		ports, ok := other.values[protocol]
		if !ok {
			result.values[protocol] = value
		} else {
			result.values[protocol] = value.Difference(ports)
		}
	}
	return result
}
