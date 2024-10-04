// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package port

import (
	"sort"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/unit"
)

// UnitPortRange represents a range of ports for a given protocol for a
// given unit.
type UnitEndpointPortRange struct {
	UnitUUID  unit.UUID
	Endpoint  string
	PortRange network.PortRange
}

func (u UnitEndpointPortRange) LessThan(other UnitEndpointPortRange) bool {
	if u.UnitUUID != other.UnitUUID {
		return u.UnitUUID < other.UnitUUID
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

func (prs UnitEndpointPortRanges) ByUnitByEndpoint() map[unit.UUID]network.GroupedPortRanges {
	byUnitByEndpoint := make(map[unit.UUID]network.GroupedPortRanges)
	for _, unitEnpointPortRange := range prs {
		unitUUID := unitEnpointPortRange.UnitUUID
		endpoint := unitEnpointPortRange.Endpoint
		if _, ok := byUnitByEndpoint[unitUUID]; !ok {
			byUnitByEndpoint[unitUUID] = network.GroupedPortRanges{}
		}
		byUnitByEndpoint[unitUUID][endpoint] = append(byUnitByEndpoint[unitUUID][endpoint], unitEnpointPortRange.PortRange)
	}
	return byUnitByEndpoint
}
