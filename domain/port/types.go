// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package port

import (
	"sort"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/unit"
)

type UnitGroupedPortRanges map[unit.Name][]network.PortRange

// UnitPortRange represents a range of ports for a given protocol for a
// given unit.
type UnitEndpointPortRange struct {
	UnitName  unit.Name
	Endpoint  string
	PortRange network.PortRange
}

func (u UnitEndpointPortRange) LessThan(other UnitEndpointPortRange) bool {
	if u.UnitName != other.UnitName {
		return u.UnitName < other.UnitName
	}
	if u.Endpoint != other.Endpoint {
		return u.Endpoint < other.Endpoint
	}
	return u.PortRange.LessThan(other.PortRange)
}

func SortUnitEndpointPortRanges(portRanges UnitEndpointPortRanges) {
	sort.Slice(portRanges, func(i, j int) bool {
		return portRanges[i].LessThan(portRanges[j])
	})
}

type UnitEndpointPortRanges []UnitEndpointPortRange

func (prs UnitEndpointPortRanges) ByUnitByEndpoint() map[unit.Name]network.GroupedPortRanges {
	byUnitByEndpoint := make(map[unit.Name]network.GroupedPortRanges)
	for _, unitEnpointPortRange := range prs {
		unitName := unitEnpointPortRange.UnitName
		endpoint := unitEnpointPortRange.Endpoint
		if _, ok := byUnitByEndpoint[unitName]; !ok {
			byUnitByEndpoint[unitName] = network.GroupedPortRanges{}
		}
		byUnitByEndpoint[unitName][endpoint] = append(byUnitByEndpoint[unitName][endpoint], unitEnpointPortRange.PortRange)
	}
	return byUnitByEndpoint
}

// PortRangesOnSubnet represents a collections of ports, organised into port ranges,
// coupled with a subnet, organised into CIDRs.
type PortRangesOnSubnet struct {
	PortRanges  []network.PortRange
	SubnetCIDRs []string
}

// GroupedPortRangesOnSubnets represents a collections ports coupled with subnets,
// grouped by a particular feature. (e.g. unit name)
type GroupedPortRangesOnSubnets map[string]PortRangesOnSubnet
