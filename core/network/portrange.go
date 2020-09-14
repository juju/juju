// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/juju/errors"
)

// GroupedPortRanges represents a list of PortRange instances grouped by a
// particular feature.
type GroupedPortRanges map[string][]PortRange

// UniquePortRanges returns the unique set of PortRanges in this group.
func (grp GroupedPortRanges) UniquePortRanges() []PortRange {
	var allPorts []PortRange
	for _, portRanges := range grp {
		allPorts = append(allPorts, portRanges...)
	}
	uniquePortRanges := UniquePortRanges(allPorts)
	SortPortRanges(uniquePortRanges)
	return uniquePortRanges
}

// Clone returns a copy of this port range grouping.
func (grp GroupedPortRanges) Clone() GroupedPortRanges {
	if len(grp) == 0 {
		return nil
	}

	grpCopy := make(GroupedPortRanges, len(grp))
	for k, v := range grp {
		grpCopy[k] = append([]PortRange(nil), v...)
	}
	return grpCopy
}

// PortRange represents a single range of ports on a particular subnet.
type PortRange struct {
	FromPort int
	ToPort   int
	Protocol string
	Name     string
}

// IsValid determines if the port range is valid.
func (p PortRange) Validate() error {
	proto := strings.ToLower(p.Protocol)
	if proto != "tcp" && proto != "udp" && proto != "icmp" {
		return errors.Errorf(`invalid protocol %q, expected "tcp", "udp", or "icmp"`, proto)
	}
	if proto == "icmp" {
		if p.FromPort == p.ToPort && p.FromPort == -1 {
			return nil
		}
		return errors.Errorf(`protocol "icmp" doesn't support any ports; got "%v"`, p.FromPort)
	}
	if p.FromPort > p.ToPort {
		return errors.Errorf("invalid port range %s", p)
	} else if p.FromPort <= 0 || p.FromPort > 65535 || p.ToPort <= 0 || p.ToPort > 65535 {
		return errors.Errorf("port range bounds must be between 1 and 65535, got %d-%d", p.FromPort, p.ToPort)
	}
	return nil
}

// Length returns the number of ports in the range.  If the range is not valid,
// it returns 0. If this range uses ICMP as the protocol then a -1 is returned
// instead.
func (p PortRange) Length() int {
	if err := p.Validate(); err != nil {
		return 0
	}
	return (p.ToPort - p.FromPort) + 1
}

// ConflictsWith determines if the two port ranges conflict.
func (p PortRange) ConflictsWith(other PortRange) bool {
	if p.Protocol != other.Protocol {
		return false
	}
	return p.ToPort >= other.FromPort && other.ToPort >= p.FromPort
}

// SanitizeBounds returns a copy of the port range, which is guaranteed to have
// FromPort >= ToPort and both FromPort and ToPort fit into the valid range
// from 1 to 65535, inclusive.
func (p PortRange) SanitizeBounds() PortRange {
	res := p
	if res.Protocol == "icmp" {
		return res
	}
	if res.FromPort > res.ToPort {
		res.FromPort, res.ToPort = res.ToPort, res.FromPort
	}
	for _, bound := range []*int{&res.FromPort, &res.ToPort} {
		switch {
		case *bound <= 0:
			*bound = 1
		case *bound > 65535:
			*bound = 65535
		}
	}
	return res
}

// String returns a formatted representation of this port range.
func (p PortRange) String() string {
	protocol := strings.ToLower(p.Protocol)
	if protocol == "icmp" {
		return protocol
	}
	if p.FromPort == p.ToPort {
		return fmt.Sprintf("%d/%s", p.FromPort, protocol)
	}
	return fmt.Sprintf("%d-%d/%s", p.FromPort, p.ToPort, protocol)
}

func (p PortRange) GoString() string {
	return p.String()
}

// LessThan returns true if other should appear after p when sorting a port
// range list.
func (p PortRange) LessThan(other PortRange) bool {
	if p.Protocol != other.Protocol {
		return p.Protocol < other.Protocol
	}
	if p.FromPort != other.FromPort {
		return p.FromPort < other.FromPort
	}
	return p.ToPort < other.ToPort
}

// SortPortRanges sorts the given ports, first by protocol, then by number.
func SortPortRanges(portRanges []PortRange) {
	sort.Slice(portRanges, func(i, j int) bool {
		return portRanges[i].LessThan(portRanges[j])
	})
}

// UniquePortRanges removes any duplicate port ranges from the input and
// returns de-dupped list back.
func UniquePortRanges(portRanges []PortRange) []PortRange {
	var (
		res       []PortRange
		processed = make(map[PortRange]struct{})
	)

	for _, pr := range portRanges {
		if _, seen := processed[pr]; seen {
			continue
		}

		res = append(res, pr)
		processed[pr] = struct{}{}
	}
	return res
}

// ParsePortRange builds a PortRange from the provided string. If the
// string does not include a protocol then "tcp" is used. Validate()
// gets called on the result before returning. If validation fails the
// invalid PortRange is still returned.
// Example strings: "80/tcp", "443", "12345-12349/udp", "icmp".
func ParsePortRange(inPortRange string) (PortRange, error) {
	// Extract the protocol.
	protocol := "tcp"
	parts := strings.SplitN(inPortRange, "/", 2)
	if len(parts) == 2 {
		inPortRange = parts[0]
		protocol = parts[1]
	}

	// Parse the ports.
	portRange, err := parsePortRange(inPortRange)
	if err != nil {
		return portRange, errors.Trace(err)
	}
	if portRange.FromPort == -1 {
		protocol = "icmp"
	}
	portRange.Protocol = protocol

	return portRange, portRange.Validate()
}

// MustParsePortRange converts a raw port-range string into a PortRange.
// If the string is invalid, the function panics.
func MustParsePortRange(portRange string) PortRange {
	portrange, err := ParsePortRange(portRange)
	if err != nil {
		panic(err)
	}
	return portrange
}

func parsePortRange(portRange string) (PortRange, error) {
	var result PortRange
	var start, end int
	parts := strings.Split(portRange, "-")
	if len(parts) > 2 {
		return result, errors.Errorf("invalid port range %q", portRange)
	}

	if len(parts) == 1 {
		if parts[0] == "icmp" {
			start, end = -1, -1
		} else {
			port, err := strconv.Atoi(parts[0])
			if err != nil {
				return result, errors.Annotatef(err, "invalid port %q", portRange)
			}
			start, end = port, port
		}
	} else {
		var err error
		if start, err = strconv.Atoi(parts[0]); err != nil {
			return result, errors.Annotatef(err, "invalid port %q", parts[0])
		}
		if end, err = strconv.Atoi(parts[1]); err != nil {
			return result, errors.Annotatef(err, "invalid port %q", parts[1])
		}
	}

	result = PortRange{
		FromPort: start,
		ToPort:   end,
	}
	return result, nil
}

// CombinePortRanges groups together all port ranges according to
// protocol, and then combines then into contiguous port ranges.
// NOTE: Juju only allows its model to contain non-overlapping port ranges.
// This method operates on that assumption.
func CombinePortRanges(ranges ...PortRange) []PortRange {
	SortPortRanges(ranges)
	var result []PortRange
	var current *PortRange
	for _, pr := range ranges {
		thispr := pr
		if current == nil {
			current = &thispr
			continue
		}
		if pr.Protocol == current.Protocol && pr.FromPort == current.ToPort+1 {
			current.ToPort = thispr.ToPort
			continue
		}
		result = append(result, *current)
		current = &thispr
	}
	if current != nil {
		result = append(result, *current)
	}
	return result
}
