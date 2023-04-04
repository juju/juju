// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/juju/core/network"
)

var _ UnitPortRanges = (*unitPortRangesForMachine)(nil)

// unitPortRanges is a view on the machinePortRanges type that provides
// unit-level information about the set of opened port ranges for various
// application endpoints.
// This unitPortRanges is denormalised. We should really access the ports by units rather than machine.
type unitPortRangesForMachine struct {
	unitName          string
	machinePortRanges *machinePortRanges
}

// UnitName returns the unit name associated with this set of ports.
func (p *unitPortRangesForMachine) UnitName() string {
	return p.unitName
}

// ForEndpoint returns a list of port ranges that the unit has opened for the
// specified endpoint.
func (p *unitPortRangesForMachine) ForEndpoint(endpointName string) []network.PortRange {
	unitPortRangeDoc := p.machinePortRanges.doc.UnitRanges[p.unitName]
	if len(unitPortRangeDoc) == 0 || len(unitPortRangeDoc[endpointName]) == 0 {
		return nil
	}
	res := append([]network.PortRange(nil), unitPortRangeDoc[endpointName]...)
	network.SortPortRanges(res)
	return res
}

// ByEndpoint returns a map where keys are endpoint names and values are the
// port ranges that the unit has opened for each endpoint.
func (p *unitPortRangesForMachine) ByEndpoint() network.GroupedPortRanges {
	unitPortRangeDoc := p.machinePortRanges.doc.UnitRanges[p.unitName]
	if len(unitPortRangeDoc) == 0 {
		return nil
	}

	res := make(network.GroupedPortRanges)
	for endpointName, portRanges := range unitPortRangeDoc {
		res[endpointName] = append([]network.PortRange(nil), portRanges...)
		network.SortPortRanges(res[endpointName])
	}
	return res
}

// UniquePortRanges returns a slice of unique open PortRanges across all
// endpoints.
func (p *unitPortRangesForMachine) UniquePortRanges() []network.PortRange {
	uniquePortRanges := p.machinePortRanges.doc.UnitRanges[p.unitName].UniquePortRanges()
	network.SortPortRanges(uniquePortRanges)
	return uniquePortRanges
}

// Open records a request for opening a particular port range for the specified
// endpoint.
func (p *unitPortRangesForMachine) Open(endpoint string, portRange network.PortRange) {
	if p.machinePortRanges.pendingOpenRanges == nil {
		p.machinePortRanges.pendingOpenRanges = make(map[string]network.GroupedPortRanges)
	}
	if p.machinePortRanges.pendingOpenRanges[p.unitName] == nil {
		p.machinePortRanges.pendingOpenRanges[p.unitName] = make(network.GroupedPortRanges)
	}

	p.machinePortRanges.pendingOpenRanges[p.unitName][endpoint] = append(
		p.machinePortRanges.pendingOpenRanges[p.unitName][endpoint],
		portRange,
	)
}

// Close records a request for closing a particular port range for the
// specified endpoint.
func (p *unitPortRangesForMachine) Close(endpoint string, portRange network.PortRange) {
	if p.machinePortRanges.pendingCloseRanges == nil {
		p.machinePortRanges.pendingCloseRanges = make(map[string]network.GroupedPortRanges)
	}
	if p.machinePortRanges.pendingCloseRanges[p.unitName] == nil {
		p.machinePortRanges.pendingCloseRanges[p.unitName] = make(network.GroupedPortRanges)
	}

	p.machinePortRanges.pendingCloseRanges[p.unitName][endpoint] = append(
		p.machinePortRanges.pendingCloseRanges[p.unitName][endpoint],
		portRange,
	)
}

// Changes returns a ModelOperation for applying any changes that were made to
// the port ranges for this unit.
func (p *unitPortRangesForMachine) Changes() ModelOperation {
	return &openClosePortRangesOperation{
		mpr:          p.machinePortRanges,
		unitSelector: p.unitName,
	}
}
