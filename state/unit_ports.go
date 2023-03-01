// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/juju/core/network"
)

// UnitPortRanges is implemented by types that can query and/or manipulate
// a set of opened port ranges for a particular unit in an endpoint-aware
// manner.
type UnitPortRanges interface {
	// UnitName returns the name of the unit these ranges apply to.
	UnitName() string

	// ForEndpoint returns a list of open port ranges for a particular
	// application endpoint.
	ForEndpoint(endpointName string) []network.PortRange

	// ByEndpoint returns the list of open port ranges grouped by
	// application endpoint.
	ByEndpoint() network.GroupedPortRanges

	// Open records a request for opening the specified port range for the
	// specified endpoint.
	Open(endpoint string, portRange network.PortRange)

	// Close records a request for closing a particular port range for the
	// specified endpoint.
	Close(endpoint string, portRange network.PortRange)

	// UniquePortRanges returns a slice of unique open PortRanges across
	// all endpoints.
	UniquePortRanges() []network.PortRange

	// Changes returns a ModelOperation for applying any changes that were
	// made to the port ranges for this unit.
	Changes() ModelOperation
}

// MachinePortRanges is implemented by types that can query and/or
// manipulate the set of port ranges opened by one or more units in a machine.
type MachinePortRanges interface {
	// MachineID returns the ID of the machine that this set of port ranges
	// applies to.
	MachineID() string

	// ByUnit returns the set of port ranges opened by each unit in a
	// particular machine subnet grouped by unit name.
	ByUnit() map[string]UnitPortRanges

	// ForUnit returns the set of port ranges opened by the specified unit
	// in a particular machine subnet.
	ForUnit(unitName string) UnitPortRanges

	// Changes returns a ModelOperation for applying any changes that were
	// made to this port range instance for all machine units.
	Changes() ModelOperation

	// UniquePortRanges returns a slice of unique open PortRanges for
	// all units on this machine.
	UniquePortRanges() []network.PortRange
}

// ApplicationPortRanges is implemented by types that can query and/or
// manipulate the set of port ranges opened by one or more units that owned by an application.
type ApplicationPortRanges interface {
	// ApplicationName returns the name of the application.
	ApplicationName() string

	// ByUnit returns the set of port ranges opened by each unit grouped by unit name.
	ByUnit() map[string]UnitPortRanges

	// ForUnit returns the set of port ranges opened by the specified unit.
	ForUnit(unitName string) UnitPortRanges

	// ByEndpoint returns the list of open port ranges grouped by
	// application endpoint.
	ByEndpoint() network.GroupedPortRanges

	// Changes returns a ModelOperation for applying any changes that were
	// made to this port range instance.
	Changes() ModelOperation

	// UniquePortRanges returns a slice of unique open PortRanges all units.
	UniquePortRanges() []network.PortRange
}
