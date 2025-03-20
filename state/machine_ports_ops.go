// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"
	"github.com/juju/names/v5"
	jujutxn "github.com/juju/txn/v3"

	"github.com/juju/juju/core/network"
)

var _ ModelOperation = (*openClosePortRangesOperation)(nil)

type openClosePortRangesOperation struct {
	mpr *machinePortRanges

	// unitSelector allows us to specify a unit name and limit the scope
	// of changes to that particular unit only.
	unitSelector string

	// The following fields are populated when the operation steps are being
	// assembled.
	openedPortRangeToUnit map[network.PortRange]string
	endpointsNamesByApp   map[string]set.Strings
	updatedUnitPortRanges map[string]network.GroupedPortRanges
}

// Build implements ModelOperation.
func (op *openClosePortRangesOperation) Build(attempt int) ([]txn.Op, error) {
	if err := checkModelNotDead(op.mpr.st); err != nil {
		return nil, errors.Annotate(err, "cannot open/close ports")
	}

	var createDoc = !op.mpr.docExists
	if attempt > 0 {
		if err := op.mpr.Refresh(); err != nil {
			if !errors.IsNotFound(err) {
				return nil, errors.Annotate(err, "cannot open/close ports")
			}

			// Doc not found; we need to create it.
			createDoc = true
		}
	}

	ops := []txn.Op{
		assertModelNotDeadOp(op.mpr.st.ModelUUID()),
	}
	if op.mpr.machineExists {
		ops = append(ops, assertMachineNotDeadOp(op.mpr.st, op.mpr.doc.MachineID))
	}

	// Start with a clean copy of the existing opened port ranges and set
	// up an auxiliary Portrange->unitName map for detecting port conflicts
	// in a more efficient manner.
	op.cloneExistingUnitPortRanges()
	op.buildPortRangeToUnitMap()

	// Find the endpoints for the applications with existing opened ports
	// and the applications with pending open port requests.
	if err := op.lookupUnitEndpoints(); err != nil {
		return nil, errors.Annotate(err, "cannot open/close ports")
	}

	// Ensure that the pending request list does not contain any bogus endpoints.
	if err := op.validatePendingChanges(); err != nil {
		return nil, errors.Annotate(err, "cannot open/close ports")
	}

	// Append docs for opening each one of the pending port ranges.
	portListModified, err := op.mergePendingOpenPortRanges()
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Scan the port ranges for each unit and prune endpoint-specific
	// entries for which we already have a rule in the wildcard endpoint
	// section.
	portListModified = op.pruneOpenPorts() || portListModified

	// Remove entries that match the pending close port requests.
	modified, err := op.mergePendingClosePortRanges()
	if err != nil {
		return nil, errors.Trace(err)
	}
	portListModified = portListModified || modified

	// Run a final prune pass and remove empty sections
	portListModified = op.pruneEmptySections() || portListModified

	// Bail out if we don't need to mutate the DB document.
	if !portListModified || (createDoc && len(op.updatedUnitPortRanges) == 0) {
		return nil, jujutxn.ErrNoOperations
	}

	// Ensure that none of the units with open port ranges are dead and
	// that all are assigned to this machine.
	if op.mpr.machineExists {
		for unitName := range op.updatedUnitPortRanges {
			ops = append(ops,
				assertUnitNotDeadOp(op.mpr.st, unitName),
				assertUnitAssignedToMachineOp(op.mpr.st, unitName, op.mpr.doc.MachineID),
			)
		}
	}

	if createDoc {
		assert := txn.DocMissing
		ops = append(ops, insertPortsDocOps(&op.mpr.doc, assert, op.updatedUnitPortRanges)...)
	} else if len(op.updatedUnitPortRanges) == 0 {
		// Port list is empty; get rid of ports document.
		ops = append(ops, op.mpr.removeOps()...)
	} else {
		assert := bson.D{{"txn-revno", op.mpr.doc.TxnRevno}}
		ops = append(ops, updatePortsDocOps(&op.mpr.doc, assert, op.updatedUnitPortRanges)...)
	}

	return ops, nil
}

// Done implements ModelOperation.
func (op *openClosePortRangesOperation) Done(err error) error {
	if err != nil {
		return err
	}

	// Document has been persisted to state.
	op.mpr.docExists = true
	op.mpr.doc.UnitRanges = op.updatedUnitPortRanges

	// If we applied all pending changes, clean up the pending maps
	if op.unitSelector == "" {
		op.mpr.pendingOpenRanges = nil
		op.mpr.pendingCloseRanges = nil
	} else {
		// Just remove the map entries for the selected unit.
		if op.mpr.pendingOpenRanges != nil {
			delete(op.mpr.pendingOpenRanges, op.unitSelector)
		}
		if op.mpr.pendingCloseRanges != nil {
			delete(op.mpr.pendingCloseRanges, op.unitSelector)
		}
	}
	return nil
}

func (op *openClosePortRangesOperation) cloneExistingUnitPortRanges() {
	op.updatedUnitPortRanges = make(map[string]network.GroupedPortRanges)
	for unitName, existingDoc := range op.mpr.doc.UnitRanges {
		newDoc := make(network.GroupedPortRanges)
		for endpointName, portRanges := range existingDoc {
			newDoc[endpointName] = append([]network.PortRange(nil), portRanges...)
		}
		op.updatedUnitPortRanges[unitName] = newDoc
	}
}

func (op *openClosePortRangesOperation) buildPortRangeToUnitMap() {
	op.openedPortRangeToUnit = make(map[network.PortRange]string)
	for existingUnitName, existingRangeDoc := range op.updatedUnitPortRanges {
		for _, existingRanges := range existingRangeDoc {
			for _, portRange := range existingRanges {
				op.openedPortRangeToUnit[portRange] = existingUnitName
			}
		}
	}
}

// lookupUnitEndpoints loads the bound endpoints for any applications already
// deployed to the target machine as well as any additional applications that
// have pending open port requests.
func (op *openClosePortRangesOperation) lookupUnitEndpoints() error {
	// Find the unique set of applications with opened port ranges on the machine.
	appsWithOpenedPorts := set.NewStrings()
	for unitName := range op.mpr.doc.UnitRanges {
		appName, err := names.UnitApplication(unitName)
		if err != nil {
			return errors.Trace(err)
		}
		appsWithOpenedPorts.Add(appName)
	}

	// Augment list with the applications in the pending open port list.
	for unitName := range op.mpr.pendingOpenRanges {
		appName, err := names.UnitApplication(unitName)
		if err != nil {
			return errors.Trace(err)
		}
		appsWithOpenedPorts.Add(appName)
	}

	// Lookup the endpoint bindings for each application
	op.endpointsNamesByApp = make(map[string]set.Strings)
	for appName := range appsWithOpenedPorts {
		appGlobalID := applicationGlobalKey(appName)
		endpointToSpaceIDMap, _, err := readEndpointBindings(op.mpr.st, appGlobalID)
		if err != nil {
			return errors.Trace(err)
		}

		appEndpoints := set.NewStrings()
		for endpointName := range endpointToSpaceIDMap {
			if endpointName == "" {
				continue
			}
			appEndpoints.Add(endpointName)
		}
		op.endpointsNamesByApp[appName] = appEndpoints
	}

	return nil
}

// validatePendingChanges ensures that the none of the pending open/close
// entries specifies an endpoint that is not defined by the unit's charm
// metadata.
func (op *openClosePortRangesOperation) validatePendingChanges() error {
	for unitName, pendingRangesByEndpoint := range op.mpr.pendingOpenRanges {
		// Already verified; ignore error
		appName, _ := names.UnitApplication(unitName)
		for pendingEndpointName := range pendingRangesByEndpoint {
			if pendingEndpointName != "" && !op.endpointsNamesByApp[appName].Contains(pendingEndpointName) {
				return errors.NotFoundf("open port range: endpoint %q for unit %q", pendingEndpointName, unitName)
			}
		}
	}
	for unitName, pendingRangesByEndpoint := range op.mpr.pendingCloseRanges {
		// Already verified; ignore error
		appName, _ := names.UnitApplication(unitName)
		for pendingEndpointName := range pendingRangesByEndpoint {
			if pendingEndpointName != "" && !op.endpointsNamesByApp[appName].Contains(pendingEndpointName) {
				return errors.NotFoundf("close port range: endpoint %q for unit %q", pendingEndpointName, unitName)
			}
		}
	}

	return nil
}

// mergePendingOpenPortRanges compares the set of new port ranges to open to
// the set of currently opened port ranges and appends a new entry for each
// port range that is not present in the current list and does not conflict
// with any pre-existing entries. The method returns a boolean value to
// indicate whether new documents were generated.
func (op *openClosePortRangesOperation) mergePendingOpenPortRanges() (bool, error) {
	var portListModified bool
	for pendingUnitName, pendingRangesByEndpoint := range op.mpr.pendingOpenRanges {
		// If we are only interested in the changes for a particular
		// unit only, exclude any pending changes for other units.
		if op.unitSelector != "" && op.unitSelector != pendingUnitName {
			continue
		}
		for pendingEndpointName, pendingRanges := range pendingRangesByEndpoint {
			for _, pendingRange := range pendingRanges {
				// If this port range has already been opened by the same unit this is a no-op
				// when the range is opened for all endpoints. Otherwise, we still need to add
				// an entry for the appropriate endpoint.
				if op.openedPortRangeToUnit[pendingRange] == pendingUnitName {
					if op.rangeExistsForEndpoint(pendingUnitName, "", pendingRange) || op.rangeExistsForEndpoint(pendingUnitName, pendingEndpointName, pendingRange) {
						continue
					}

					// Still need to add an entry for the specified endpoint.
				} else if err := op.checkForPortRangeConflict(pendingUnitName, pendingRange); err != nil {
					return false, errors.Annotatef(err, "cannot open ports %v", pendingRange)
				}

				// We can safely add the new port range to the updated port list.
				if op.updatedUnitPortRanges[pendingUnitName] == nil {
					op.updatedUnitPortRanges[pendingUnitName] = make(network.GroupedPortRanges)
				}
				op.updatedUnitPortRanges[pendingUnitName][pendingEndpointName] = append(
					op.updatedUnitPortRanges[pendingUnitName][pendingEndpointName],
					pendingRange,
				)
				op.openedPortRangeToUnit[pendingRange] = pendingUnitName
				portListModified = true
			}
		}
	}

	return portListModified, nil
}

// pruneOpenPorts examines the open ports for each unit and removes any
// endpoint-specific ranges that are also present in the wildcard (all
// endpoints) section for the unit. The method returns a boolean value to
// indicate whether any ranges where pruned.
func (op *openClosePortRangesOperation) pruneOpenPorts() bool {
	var portListModified bool
	for unitName, unitRangeDoc := range op.updatedUnitPortRanges {
		for endpointName, portRanges := range unitRangeDoc {
			if endpointName == "" {
				continue
			}

			for i := 0; i < len(portRanges); i++ {
				for _, wildcardPortRange := range unitRangeDoc[""] {
					if portRanges[i] != wildcardPortRange {
						continue
					}

					// This port is redundant as it already
					// exists in the wildcard section.
					// Remove it from the port range list.
					portRanges[i] = portRanges[len(portRanges)-1]
					portRanges = portRanges[:len(portRanges)-1]
					portListModified = true
					i--
					break
				}
			}
			unitRangeDoc[endpointName] = portRanges
		}
		op.updatedUnitPortRanges[unitName] = unitRangeDoc
	}
	return portListModified
}

// mergePendingClosePortRanges compares the set of port ranges to close to the
// set of currently opened port range documents and removes the entries that
// correspond to the port ranges that should be closed.
//
// The implementation contains additional logic to detect cases where a port
// range is currently opened for all endpoints and we attempt to close it
// for a specific endpoint. In this case, the port range will be removed from
// the wildcard slot of the unitPortRanges document and new entries will be
// added for all bound endpoints except the one where the port range is closed.
//
// The method returns a boolean value to indicate whether any changes were made.
func (op *openClosePortRangesOperation) mergePendingClosePortRanges() (bool, error) {
	var portListModified bool
	for pendingUnitName, pendingRangesByEndpoint := range op.mpr.pendingCloseRanges {
		// If we are only interested in the changes for a particular
		// unit only, exclude any pending changes for other units.
		if op.unitSelector != "" && op.unitSelector != pendingUnitName {
			continue
		}
		for pendingEndpointName, pendingRanges := range pendingRangesByEndpoint {
			for _, pendingRange := range pendingRanges {
				// If the port range has not been opened by
				// this unit we only need to ensure that it
				// doesn't cause a conflict with port ranges
				// opened by other units.
				if op.openedPortRangeToUnit[pendingRange] != pendingUnitName {
					if err := op.checkForPortRangeConflict(pendingUnitName, pendingRange); err != nil {
						return false, errors.Annotatef(err, "cannot close ports %v", pendingRange)
					}

					// This port range is not open so this is a no-op.
					continue
				}

				portListModified = op.removePortRange(pendingUnitName, pendingEndpointName, pendingRange) || portListModified
			}
		}
	}

	return portListModified, nil
}

func (op *openClosePortRangesOperation) removePortRange(unitName, endpointName string, portRange network.PortRange) bool {
	var portListModified bool

	// Sanity check
	if len(op.updatedUnitPortRanges[unitName]) == 0 {
		return false
	}

	// If we target all endpoints, remove the range from the wildcard entry
	// as well as any other endpoint-specific entries (if present)
	if endpointName == "" {
		delete(op.openedPortRangeToUnit, portRange)
		for existingEndpointName, existingRanges := range op.updatedUnitPortRanges[unitName] {
			for i := 0; i < len(existingRanges); i++ {
				if existingRanges[i] != portRange {
					continue
				}

				// Remove entry from list
				existingRanges[i] = existingRanges[len(existingRanges)-1]
				op.updatedUnitPortRanges[unitName][existingEndpointName] = existingRanges[:len(existingRanges)-1]
				portListModified = true
				break
			}
		}

		return portListModified
	}

	// If we target a specific endpoint, start by removing the port from
	// the specified endpoint (if the range is present).
	if existingRanges := op.updatedUnitPortRanges[unitName][endpointName]; len(existingRanges) != 0 {
		for i := 0; i < len(existingRanges); i++ {
			if existingRanges[i] != portRange {
				continue
			}

			// Remove entry from list
			existingRanges[i] = existingRanges[len(existingRanges)-1]
			op.updatedUnitPortRanges[unitName][endpointName] = existingRanges[:len(existingRanges)-1]
			portListModified = true
			break
		}
	}

	// If the port range is instead present in the wildcard slot, we
	// need to remove it and replace it with entries for each bound endpoint
	// except the one we just closed the port to.
	if existingRanges := op.updatedUnitPortRanges[unitName][""]; len(existingRanges) != 0 {
		for i := 0; i < len(existingRanges); i++ {
			if existingRanges[i] != portRange {
				continue
			}

			// Remove entry from list
			existingRanges[i] = existingRanges[len(existingRanges)-1]
			op.updatedUnitPortRanges[unitName][""] = existingRanges[:len(existingRanges)-1]
			portListModified = true

			// This has already been checked during endpoint lookup.
			// The error can be safely ignored here.
			appName, _ := names.UnitApplication(unitName)

			// Iterate the set of application endpoints
			for appEndpoint := range op.endpointsNamesByApp[appName] {
				if appEndpoint == endpointName {
					continue // the port is closed for this endpoint
				}

				// The port should remain open for the remaining endpoints.
				op.updatedUnitPortRanges[unitName][appEndpoint] = append(
					op.updatedUnitPortRanges[unitName][appEndpoint],
					portRange,
				)
			}

			break
		}
	}

	// Finally, check if the port range is still open for any other endpoint.
	// If not, remove it from the openedPortRangeToUnit map.
	for endpointName := range op.updatedUnitPortRanges[unitName] {
		if op.rangeExistsForEndpoint(unitName, endpointName, portRange) {
			return portListModified
		}
	}

	delete(op.openedPortRangeToUnit, portRange)
	return portListModified
}

func (op *openClosePortRangesOperation) rangeExistsForEndpoint(unitName, endpointName string, portRange network.PortRange) bool {
	if len(op.updatedUnitPortRanges[unitName]) == 0 || len(op.updatedUnitPortRanges[unitName][endpointName]) == 0 {
		return false
	}

	for _, existingPortRange := range op.updatedUnitPortRanges[unitName][endpointName] {
		if existingPortRange == portRange {
			return true
		}
	}

	return false
}

// pruneEmptySections removes empty port range sections from the updated unit
// port range documents and removes the docs themselves if they end up empty.
// The method returns a boolean value to indicate whether any changes where
// made.
func (op *openClosePortRangesOperation) pruneEmptySections() bool {
	var portListModified bool
	for unitName, unitRangeDoc := range op.updatedUnitPortRanges {
		for endpointName, portRanges := range unitRangeDoc {
			if len(portRanges) == 0 {
				delete(unitRangeDoc, endpointName)
				portListModified = true
			}
		}
		if len(unitRangeDoc) == 0 {
			delete(op.updatedUnitPortRanges, unitName)
			portListModified = true
			continue
		}
		op.updatedUnitPortRanges[unitName] = unitRangeDoc
	}
	return portListModified
}

// checkForPortRangeConflict returns an error if a pending port range conflicts
// with any already opeend port range.
func (op *openClosePortRangesOperation) checkForPortRangeConflict(pendingUnitName string, pendingRange network.PortRange) error {
	if err := pendingRange.Validate(); err != nil {
		return errors.Trace(err)
	}

	for existingRange, existingUnitName := range op.openedPortRangeToUnit {
		if pendingRange.ConflictsWith(existingRange) {
			return errors.Errorf("port ranges %v (%q) and %v (%q) conflict", existingRange, existingUnitName, pendingRange, pendingUnitName)
		}
	}

	return nil
}
