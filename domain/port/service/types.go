// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/port"
)

// portRangeUUIDIndex is a map targeting the uuids of existing port ranges for a unit.
// These UUIDS are indexed, first by the endpoint, then by the port range itself.
type portRangeUUIDIndex map[string]map[network.PortRange]string

// indexPortRanges indexes the given port ranges by endpoint and port range.
func indexPortRanges(groupedPortRanges map[string][]port.PortRangeUUID) portRangeUUIDIndex {
	index := make(portRangeUUIDIndex)
	for endpoint, portRanges := range groupedPortRanges {
		for _, portRange := range portRanges {
			if _, ok := index[endpoint]; !ok {
				index[endpoint] = make(map[network.PortRange]string)
			}
			index[endpoint][portRange.PortRange] = portRange.UUID
		}
	}
	return index
}
