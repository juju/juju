// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"sort"
	"strconv"

	"github.com/juju/utils/set"
)

// RuleSet is a set-like container of Port values.
// TODO(Wallyworld) - this struct is only used by the GCE provider and needs
// to be updated to account for SourceCIDRs along with the GCE provider.
type RuleSet struct {
	values map[string]set.Ints
}

// NewRuleSet creates a map of protocols to sets of stringified port numbers.
func NewRuleSet(rules ...IngressRule) RuleSet {
	var result RuleSet
	result.values = make(map[string]set.Ints)
	result.AddRanges(rules...)
	return result
}

// Size returns the number of rules in the set.
func (rs RuleSet) Size() int {
	size := 0
	for _, rules := range rs.values {
		size += len(rules)
	}
	return size
}

// IsEmpty returns true if the RuleSet is empty.
func (rs RuleSet) IsEmpty() bool {
	return len(rs.values) == 0
}

// Values returns a list of all the ports in the set.
func (rs RuleSet) Values() []Port {
	return rs.Ports()
}

// Protocols returns a list of protocols known to the RuleSet.
func (rs RuleSet) Protocols() []string {
	var result []string
	for key := range rs.values {
		result = append(result, key)
	}
	return result
}

// PortRanges returns a list of all the port ranges in the set for the
// given protocols. If no protocols are provided all known protocols in
// the set are used.
func (rs RuleSet) PortRanges(protocols ...string) []PortRange {
	if len(protocols) == 0 {
		protocols = rs.Protocols()
	}

	var result []PortRange
	for _, protocol := range protocols {
		ranges := collapsePorts(protocol, rs.PortNumbers(protocol)...)
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

// Ports returns a list of all the ports in the set for
// the given protocols. If no protocols are provided then all known
// protocols in the set are used.
func (rs RuleSet) Ports(protocols ...string) []Port {
	if len(protocols) == 0 {
		protocols = rs.Protocols()
	}

	var results []Port
	for _, portRange := range rs.PortRanges(protocols...) {
		for p := portRange.FromPort; p <= portRange.ToPort; p++ {
			results = append(results, Port{portRange.Protocol, p})
		}
	}
	return results
}

// PortNumbers returns a list of all the port numbers in the set for
// the given protocol.
func (rs RuleSet) PortNumbers(protocol string) []int {
	ports, ok := rs.values[protocol]
	if !ok {
		return nil
	}
	return ports.Values()
}

// PortStrings returns a list of stringified ports in the set
// for the given protocol. This is strictly a convenience method
// for situations where another API requires a list of strings.
func (rs RuleSet) PortStrings(protocol string) []string {
	ports, ok := rs.values[protocol]
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

// Add adds a port to the RuleSet.
func (rs *RuleSet) Add(protocol string, port int) {
	if rs.values == nil {
		panic("uninitalised set")
	}
	ports, ok := rs.values[protocol]
	if !ok {
		rs.values[protocol] = set.NewInts(port)
	} else {
		ports.Add(port)
	}
}

// AddRanges adds rules to the RuleSet.
func (rs *RuleSet) AddRanges(rules ...IngressRule) {
	for _, rule := range rules {
		for p := rule.FromPort; p <= rule.ToPort; p++ {
			rs.Add(rule.Protocol, p)
		}
	}
}

// Remove removes the given port from the set.
func (rs *RuleSet) Remove(protocol string, port int) {
	ports, ok := rs.values[protocol]
	if ok {
		ports.Remove(port)
	}
}

// RemoveRanges removes all ports in the given PortRange values
// from the set.
func (rs *RuleSet) RemoveRanges(portRanges ...IngressRule) {
	for _, portRange := range portRanges {
		_, ok := rs.values[portRange.Protocol]
		if ok {
			for p := portRange.FromPort; p <= portRange.ToPort; p++ {
				rs.Remove(portRange.Protocol, p)
			}
		}
	}
}

// Contains returns true if the provided port is in the set.
func (rs *RuleSet) Contains(protocol string, port int) bool {
	ports, ok := rs.values[protocol]
	if !ok {
		return false
	}
	return ports.Contains(port)
}

// ContainsRanges returns true if the provided port ranges are
// in the set.
func (rs *RuleSet) ContainsRanges(portRanges ...IngressRule) bool {
	for _, portRange := range portRanges {
		ports, ok := rs.values[portRange.Protocol]
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

// Union returns a new RuleSet of the shared values
// that are common between both PortSets.
func (rs RuleSet) Union(other RuleSet) RuleSet {
	result := NewRuleSet()
	for protocol, value := range rs.values {
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

// Intersection returns a new RuleSet of the values that are in both
// this set and the other, but not in just one of either.
func (rs RuleSet) Intersection(other RuleSet) RuleSet {
	result := NewRuleSet()
	for protocol, value := range rs.values {
		if ports, ok := other.values[protocol]; ok {
			// For RuleSet, a protocol without any associated ports
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

// Difference returns a new RuleSet of the values
// that are not in the other RuleSet.
func (rs RuleSet) Difference(other RuleSet) RuleSet {
	result := NewRuleSet()
	for protocol, value := range rs.values {
		if ports, ok := other.values[protocol]; ok {
			// For RuleSet, a protocol without any associated ports
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
