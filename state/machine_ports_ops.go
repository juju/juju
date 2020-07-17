// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

var (
	_ ModelOperation = (*openClosePortsOperation)(nil)
)

type openClosePortsOperation struct {
	p               *MachineSubnetPorts
	openPortRanges  []PortRange
	closePortRanges []PortRange

	updatedPortList []PortRange
}

// Build implements ModelOperation.
func (op *openClosePortsOperation) Build(attempt int) ([]txn.Op, error) {
	var createPortsDoc = op.p.areNew
	if attempt > 0 {
		if err := checkModelNotDead(op.p.st); err != nil {
			return nil, errors.Annotate(err, "cannot open/close ports")
		}
		if err := op.verifySubnetAliveWhenSet(); err != nil {
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

	ops := []txn.Op{
		assertModelNotDeadOp(op.p.st.ModelUUID()),
	}

	// Start with a clean copy of the ranges from the ports document
	var portListModified bool
	op.updatedPortList = append(op.updatedPortList[0:0], op.p.doc.Ports...)

	// Check for conflicts opening each port range and update the final port list
	for _, openPortRange := range op.openPortRanges {
		var alreadyExists bool
		for _, existingPorts := range op.updatedPortList {
			if existingPorts == openPortRange {
				alreadyExists = true
				break // nothing to do
			}

			if err := existingPorts.CheckConflicts(openPortRange); err != nil {
				return nil, errors.Annotatef(err, "cannot open ports %v", openPortRange)
			}
		}

		if !alreadyExists {
			op.updatedPortList = append(op.updatedPortList, openPortRange)
			ops = append(ops, assertUnitNotDeadOp(op.p.st, openPortRange.UnitName))
			portListModified = true
		}
	}

	// Check for conflicts closing each port range and update the final port list
	for _, closePortRange := range op.closePortRanges {
		var foundAtIndex = -1
		for i, existingPorts := range op.updatedPortList {
			if existingPorts == closePortRange {
				foundAtIndex = i
				continue
			}

			if err := existingPorts.CheckConflicts(closePortRange); err != nil && existingPorts.UnitName == closePortRange.UnitName {
				return nil, errors.Annotatef(err, "cannot close ports %v", closePortRange)
			}
		}

		if foundAtIndex != -1 {
			// Delete portrange at foundAtIndex by copying the last
			// element of the list over and trimming the slice len.
			op.updatedPortList[foundAtIndex] = op.updatedPortList[len(op.updatedPortList)-1]
			op.updatedPortList = op.updatedPortList[:len(op.updatedPortList)-1]
			ops = append(ops, assertUnitNotDeadOp(op.p.st, closePortRange.UnitName))
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

func (op *openClosePortsOperation) verifySubnetAliveWhenSet() error {
	if op.p.doc.SubnetID == "" {
		return nil
	}

	subnet, err := op.p.st.Subnet(op.p.doc.SubnetID)
	if err != nil {
		return errors.Trace(err)
	} else if subnet.Life() != Alive {
		return errors.Errorf("subnet %q not alive", subnet.CIDR())
	}
	return nil
}
