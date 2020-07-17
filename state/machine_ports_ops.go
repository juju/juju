// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/juju/core/network"
	jujutxn "github.com/juju/txn"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

var (
	_ ModelOperation = (*openClosePortsOperation)(nil)
)

type openClosePortsOperation struct {
	p               *machineSubnetPorts
	unitName        string
	openPortRanges  []network.PortRange
	closePortRanges []network.PortRange

	updatedPortList []portRangeDoc
}

// Build implements ModelOperation.
func (op *openClosePortsOperation) Build(attempt int) ([]txn.Op, error) {
	var createPortsDoc = op.p.areNew
	if attempt > 0 {
		if err := checkModelNotDead(op.p.st); err != nil {
			return nil, errors.Annotate(err, "cannot open/close ports")
		}
		if err := op.p.Refresh(); err != nil {
			if !errors.IsNotFound(err) {
				return nil, errors.Annotate(err, "cannot open/close ports")
			}

			// Ports doc not found; we need to add a new one.
			createPortsDoc = true
		}
	}

	if err := verifySubnetAlive(op.p.st, op.p.doc.SubnetID); err != nil {
		return nil, errors.Annotate(err, "cannot open/close ports")
	}

	ops := []txn.Op{
		assertModelNotDeadOp(op.p.st.ModelUUID()),
		assertUnitNotDeadOp(op.p.st, op.unitName),
	}

	// Start with a clean copy of the ranges from the ports document
	var portListModified bool
	op.updatedPortList = append(op.updatedPortList[0:0], op.p.doc.Ports...)

	// Check for conflicts opening each port range and update the final port list
	for _, openPortRange := range op.openPortRanges {
		var alreadyExists bool
		for _, existing := range op.updatedPortList {
			identical, err := checkForPortRangeConflict(existing.asPortRange(), existing.UnitName, openPortRange, op.unitName)
			if err != nil {
				return nil, errors.Annotatef(err, "cannot open ports %v", openPortRange)
			} else if identical {
				alreadyExists = true
				break // nothing to do
			}
		}

		if !alreadyExists {
			op.updatedPortList = append(op.updatedPortList, portRangeDoc{
				FromPort: openPortRange.FromPort,
				ToPort:   openPortRange.ToPort,
				Protocol: openPortRange.Protocol,
				UnitName: op.unitName,
			})
			portListModified = true
		}
	}

	// Check for conflicts closing each port range and update the final port list
	for _, closePortRange := range op.closePortRanges {
		var foundAtIndex = -1
		for i, existing := range op.updatedPortList {
			identical, err := checkForPortRangeConflict(existing.asPortRange(), existing.UnitName, closePortRange, op.unitName)
			if err != nil && existing.UnitName == op.unitName {
				return nil, errors.Annotatef(err, "cannot close ports %v", closePortRange)
			} else if identical {
				foundAtIndex = i
				continue
			}
		}

		if foundAtIndex != -1 {
			// Delete portrange at foundAtIndex by copying the last
			// element of the list over and trimming the slice len.
			op.updatedPortList[foundAtIndex] = op.updatedPortList[len(op.updatedPortList)-1]
			op.updatedPortList = op.updatedPortList[:len(op.updatedPortList)-1]
			portListModified = true
		}
	}

	// Nothing to do
	if !portListModified || (createPortsDoc && len(op.updatedPortList) == 0) {
		return nil, jujutxn.ErrNoOperations
	}

	if createPortsDoc {
		assert := txn.DocMissing
		ops = append(ops, addPortsDocOps(op.p.st, &op.p.doc, assert, op.updatedPortList...)...)
	} else if len(op.updatedPortList) == 0 {
		// Port list is empty; get rid of ports document.
		ops = append(ops, op.p.removeOps()...)
	} else {
		assert := bson.D{{"txn-revno", op.p.doc.TxnRevno}}
		ops = append(ops, setPortsDocOps(op.p.st, op.p.doc, assert, op.updatedPortList...)...)
	}

	return ops, nil
}

// Done implements ModelOperation.
func (op *openClosePortsOperation) Done(err error) error {
	if err != nil {
		return err
	}

	// Document has been persisted to state.
	op.p.areNew = false
	op.p.doc.Ports = op.updatedPortList
	return nil
}

func verifySubnetAlive(st *State, subnetID string) error {
	if subnetID == "" {
		return nil
	}

	subnet, err := st.Subnet(subnetID)
	if err != nil {
		return errors.Trace(err)
	} else if subnet.Life() != Alive {
		return errors.Errorf("subnet %q not alive", subnet.ID())
	}
	return nil
}

// checkForPortRangeConflict inspects two ranges that may or may not belong to
// the same unit for conflicts. If a port conflict is detected, an error is
// returned.
func checkForPortRangeConflict(rangeA network.PortRange, unitA string, rangeB network.PortRange, unitB string) (identical bool, err error) {
	if err := rangeA.Validate(); err != nil {
		return false, errors.Trace(err)
	} else if err := rangeB.Validate(); err != nil {
		return false, errors.Trace(err)
	}

	// Same unit and range; no conflict.
	if rangeA == rangeB && unitA == unitB {
		return true, nil
	}

	if rangeA.ConflictsWith(rangeB) {
		return false, errors.Errorf("port ranges %v (%q) and %v (%q) conflict", rangeA, unitA, rangeB, unitB)
	}

	return false, nil
}
