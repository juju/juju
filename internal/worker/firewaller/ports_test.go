// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	"github.com/juju/errors"

	"github.com/juju/juju/core/network"
)

// This is a subset of the port range logic from state.
// It really belongs in core. The entire logic (not just
// the subset here) should be extracted from the persistence
// code and moved to core.

type unitPortRanges struct {
	unitRanges map[string]network.GroupedPortRanges

	pendingOpenRanges  network.GroupedPortRanges
	pendingCloseRanges network.GroupedPortRanges
}

func newUnitPortRanges() *unitPortRanges {
	return &unitPortRanges{
		pendingOpenRanges:  make(network.GroupedPortRanges),
		pendingCloseRanges: make(network.GroupedPortRanges),
	}
}

func (p *unitPortRanges) Open(endpoint string, portRange network.PortRange) {
	p.pendingOpenRanges[endpoint] = append(p.pendingOpenRanges[endpoint], portRange)
}

func (p *unitPortRanges) Close(endpoint string, portRange network.PortRange) {
	p.pendingCloseRanges[endpoint] = append(p.pendingCloseRanges[endpoint], portRange)
}

func (p *unitPortRanges) ByUnitEndpoint() map[string]network.GroupedPortRanges {
	result := map[string]network.GroupedPortRanges{}
	for u, r := range p.unitRanges {
		result[u] = r
	}
	return result
}

type unitPortRangesCommit struct {
	unitName              string
	upr                   *unitPortRanges
	updatedUnitPortRanges map[string]network.GroupedPortRanges
}

func newUnitPortRangesCommit(upr *unitPortRanges, unitName string) *unitPortRangesCommit {
	op := &unitPortRangesCommit{upr: upr, unitName: unitName}
	return op
}

func (op *unitPortRangesCommit) cloneExistingUnitPortRanges() {
	op.updatedUnitPortRanges = make(map[string]network.GroupedPortRanges)
	for unitName, existingDoc := range op.upr.unitRanges {
		rangesCopy := make(network.GroupedPortRanges)
		for endpointName, portRanges := range existingDoc {
			rangesCopy[endpointName] = append([]network.PortRange(nil), portRanges...)
		}
		op.updatedUnitPortRanges[unitName] = rangesCopy
	}
}

func (op *unitPortRangesCommit) Commit() (bool, error) {
	defer func() {
		op.upr.pendingOpenRanges = make(network.GroupedPortRanges)
		op.upr.pendingCloseRanges = make(network.GroupedPortRanges)
	}()

	op.cloneExistingUnitPortRanges()

	portListModified, err := op.mergePendingOpenPortRanges()
	if err != nil {
		return false, errors.Trace(err)
	}
	modified, err := op.mergePendingClosePortRanges()
	if err != nil {
		return false, errors.Trace(err)
	}
	portListModified = portListModified || modified

	if !portListModified {
		return false, nil
	}
	op.upr.unitRanges = op.updatedUnitPortRanges
	return true, nil
}

func (op *unitPortRangesCommit) mergePendingOpenPortRanges() (bool, error) {
	var modified bool
	for endpointName, pendingRanges := range op.upr.pendingOpenRanges {
		for _, pendingRange := range pendingRanges {
			if op.rangeExistsForEndpoint(endpointName, pendingRange) {
				// Exists, no op for opening.
				continue
			}
			op.addPortRanges(endpointName, true, pendingRange)
			modified = true
		}
	}
	return modified, nil
}

func (op *unitPortRangesCommit) mergePendingClosePortRanges() (bool, error) {
	var modified bool
	for endpointName, pendingRanges := range op.upr.pendingCloseRanges {
		for _, pendingRange := range pendingRanges {
			if !op.rangeExistsForEndpoint(endpointName, pendingRange) {
				// Not exists, no op for closing.
				continue
			}
			modified = op.removePortRange(endpointName, pendingRange)
		}
	}
	return modified, nil
}

func (op *unitPortRangesCommit) addPortRanges(endpointName string, merge bool, portRanges ...network.PortRange) {
	if op.updatedUnitPortRanges[op.unitName] == nil {
		op.updatedUnitPortRanges[op.unitName] = make(network.GroupedPortRanges)
	}
	if !merge {
		op.updatedUnitPortRanges[op.unitName][endpointName] = portRanges
		return
	}
	op.updatedUnitPortRanges[op.unitName][endpointName] = append(op.updatedUnitPortRanges[op.unitName][endpointName], portRanges...)
}

func (op *unitPortRangesCommit) removePortRange(endpointName string, portRange network.PortRange) bool {
	if op.updatedUnitPortRanges[op.unitName] == nil || op.updatedUnitPortRanges[op.unitName][endpointName] == nil {
		return false
	}
	var modified bool
	existingRanges := op.updatedUnitPortRanges[op.unitName][endpointName]
	for i, v := range existingRanges {
		if v != portRange {
			continue
		}
		existingRanges = append(existingRanges[:i], existingRanges[i+1:]...)
		if len(existingRanges) == 0 {
			delete(op.updatedUnitPortRanges[op.unitName], endpointName)
		} else {
			op.addPortRanges(endpointName, false, existingRanges...)
		}
		modified = true
	}
	return modified
}

func (op *unitPortRangesCommit) rangeExistsForEndpoint(endpointName string, portRange network.PortRange) bool {
	if len(op.updatedUnitPortRanges[op.unitName][endpointName]) == 0 {
		return false
	}

	for _, existingRange := range op.updatedUnitPortRanges[op.unitName][endpointName] {
		if existingRange == portRange {
			return true
		}
	}
	return false
}
