// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package port

import (
	"sort"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

type UUID string

// NewUUID generates a new UUID.
func NewUUID() (UUID, error) {
	id, err := uuid.NewUUID()
	if err != nil {
		return UUID(""), err
	}
	return UUID(id.String()), nil
}

// String returns the UUID as a string.
func (u UUID) String() string {
	return string(u)
}

// Validate ensures the consistency of the UUID.
func (u UUID) Validate() error {
	if u == "" {
		return errors.Errorf("uuid cannot be empty")
	}
	if !uuid.IsValidUUIDString(string(u)) {
		return errors.Errorf("uuid %q is not valid", u)
	}
	return nil
}

// Endpoint represents a unit's network endpoint.
type Endpoint struct {
	UUID     UUID
	Endpoint string
}

type PortRangeWithUUID struct {
	UUID      UUID
	PortRange network.PortRange
}

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
	for _, grp := range byUnitByEndpoint {
		for _, portRanges := range grp {
			network.SortPortRanges(portRanges)
		}
	}
	return byUnitByEndpoint
}
