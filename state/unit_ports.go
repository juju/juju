// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"github.com/juju/juju/core/network"
)

// UnitPorts represents the set of open ports on the machine a unit is deployed to.
type UnitPorts struct {
	unitName      string
	machineID     string
	portsBySubnet map[string][]network.PortRange
}

// UnitName returns the unit name associated with this set of ports.
func (p *UnitPorts) UnitName() string {
	return p.unitName
}

// UnitName returns the machine ID where this unit is deployed to.
func (p *UnitPorts) MachineID() string {
	return p.machineID
}

// String returns p as a user-readable string.
func (p *UnitPorts) String() string {
	return fmt.Sprintf("ports for unit %q on machine %q", p.unitName, p.machineID)
}

// InSubnet returns a list of opened ports for this unit on the specified subnet.
func (p *UnitPorts) InSubnet(subnetID string) []network.PortRange {
	return p.portsBySubnet[subnetID]
}

// BySubnet returns a map where keys are subnet IDs and values are the list of
// open port ranges in each subnet.
func (p *UnitPorts) BySubnet() map[string][]network.PortRange {
	return p.portsBySubnet
}

// UniquePortRanges returns a slice of unique open PortRanges across all
// subnets. This method is provided for backwards compatibility purposes for
// cases where opened port ranges are assumed to apply across all subnets.
//
// Newer applications should always use the subnet-aware methods on this type.
func (p *UnitPorts) UniquePortRanges() []network.PortRange {
	var (
		res  []network.PortRange
		seen = make(map[network.PortRange]struct{})
	)

	for _, portRangesInSubnet := range p.portsBySubnet {
		for _, portRange := range portRangesInSubnet {
			if _, found := seen[portRange]; found {
				continue
			}

			seen[portRange] = struct{}{}
			res = append(res, portRange)
		}
	}

	network.SortPortRanges(res)
	return res
}
