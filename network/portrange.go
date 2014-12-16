// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"fmt"
	"sort"
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
