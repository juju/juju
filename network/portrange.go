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

func (p PortRange) GoString() string {
	return p.String()
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

// CollapsePorts collapses a slice of ports into port ranges.
//
// NOTE(dimitern): This is deprecated and should be removed when
// possible. It still exists, because in a few places slices of Ports
// are converted to PortRanges internally.
func CollapsePorts(ports []Port) (result []PortRange) {
	// First, convert ports to ranges, then sort them.
	var portRanges []PortRange
	for _, p := range ports {
		portRanges = append(portRanges, PortRange{p.Number, p.Number, p.Protocol})
	}
	SortPortRanges(portRanges)
	fromPort := 0
	toPort := 0
	protocol := ""
	// Now merge single port ranges while preserving the order.
	for _, pr := range portRanges {
		if fromPort == 0 {
			// new port range
			fromPort = pr.FromPort
			toPort = pr.ToPort
			protocol = pr.Protocol
		} else if pr.FromPort == toPort+1 && protocol == pr.Protocol {
			// continuing port range
			toPort = pr.FromPort
		} else {
			// break in port range
			result = append(result,
				PortRange{
					Protocol: protocol,
					FromPort: fromPort,
					ToPort:   toPort,
				})
			fromPort = pr.FromPort
			toPort = pr.ToPort
			protocol = pr.Protocol
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

// ParsePortRange builds a PortRange from the provided string. If the
// string does not include a protocol then "tcp" is used. Validate()
// gets called on the result before returning. If validation fails the
// invalid PortRange is still returned.
// Example strings: "80/tcp", "443", "12345-12349/udp".
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
		port, err := strconv.Atoi(parts[0])
		if err != nil {
			return result, errors.Annotatef(err, "invalid port %q", portRange)
		}
		start = port
		end = port
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

// ParsePortRanges splits the provided string on commas and extracts a
// PortRange from each part of the split string. Whitespace is ignored.
// Example strings: "80/tcp", "80,443,1234/udp", "123-456, 25/tcp".
func ParsePortRanges(inPortRanges string) ([]PortRange, error) {
	var portRanges []PortRange
	for _, portRange := range strings.Split(inPortRanges, ",") {
		portRange, err := ParsePortRange(strings.TrimSpace(portRange))
		if err != nil {
			return portRanges, errors.Trace(err)
		}
		portRanges = append(portRanges, portRange)
	}
	return portRanges, nil
}
