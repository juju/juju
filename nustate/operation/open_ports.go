// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

import (
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/nustate/model"
	"github.com/juju/juju/nustate/operation/precondition"
	"github.com/juju/juju/nustate/persistence"
	"github.com/juju/juju/nustate/persistence/transaction"
)

// Changes is implemented by types that can provide a set of transaction
// elements for applying a complex, multi-model mutation operation.
type Changes interface {
	transaction.Element
	Changes(transaction.Context) (transaction.ModelTxn, error)
}

// We could use the concrete type here but perhaps an interface would make
// mocking easier at the facade level?
type MutateMachinePortRangesOperation interface {
	Changes

	ForUnit(string) MutateUnitPortRangesOperation
}

type mutateMachinePortRangesOp struct {
	transaction.BuildingBlock

	store     persistence.Store
	portModel model.MachinePortRanges

	// Recorded set of changes to apply.
	pendingOpen  map[string]map[string][]network.PortRange
	pendingClose map[string]map[string][]network.PortRange
}

func MutateMachinePortRanges(modelStore persistence.Store, machineID string) (MutateMachinePortRangesOperation, error) {
	portModel, err := modelStore.FindMachinePortRanges(machineID)
	if err != nil {
		return nil, err
	}

	return &mutateMachinePortRangesOp{
		store:        modelStore,
		portModel:    portModel,
		pendingOpen:  make(map[string]map[string][]network.PortRange),
		pendingClose: make(map[string]map[string][]network.PortRange),
	}, nil
}

func (op *mutateMachinePortRangesOp) ForUnit(unitName string) MutateUnitPortRangesOperation {
	return &mutateUnitPortRangesOp{
		mpr:      op,
		unitName: unitName,
	}
}

func (op *mutateMachinePortRangesOp) Changes(ctx transaction.Context) (transaction.ModelTxn, error) {
	if ctx.Attempt > 0 {
		// TODO: Refresh our models; this means we can mutate the model
		// in-place without worrying about its state in case an error
		// occurs.
	}

	// Calculate effective port ranges
	effectivePortRanges, err := op.mergePendingPortRanges()
	if err != nil {
		return nil, err
	}

	// Merge to latched model and return it back
	op.portModel.SetPortRanges(effectivePortRanges)
	return transaction.ModelTxn{
		precondition.MachineAlive(op.portModel.MachineID()),
		// TODO: a unit-assigned-to-machine assertion for each unit
		// in the port model
		op.portModel,
	}, nil

}

func (op *mutateMachinePortRangesOp) mergePendingPortRanges() (map[string]*model.UnitPortRanges, error) {
	// see machine_port_ops.go
	panic("not implemented")
}

type MutateUnitPortRangesOperation interface {
	// Just use a fluid interface for fun
	Open(endpoint string, portRange network.PortRange) MutateUnitPortRangesOperation
	Close(endpoint string, portRange network.PortRange) MutateUnitPortRangesOperation
}

type mutateUnitPortRangesOp struct {
	mpr      *mutateMachinePortRangesOp
	unitName string
}

func (op *mutateUnitPortRangesOp) Open(endpoint string, portRange network.PortRange) MutateUnitPortRangesOperation {
	if op.mpr.pendingOpen[op.unitName] == nil {
		op.mpr.pendingOpen[op.unitName] = make(map[string][]network.PortRange)
	}
	op.mpr.pendingOpen[op.unitName][endpoint] = append(op.mpr.pendingOpen[op.unitName][endpoint], portRange)
	return op
}

func (op *mutateUnitPortRangesOp) Close(endpoint string, portRange network.PortRange) MutateUnitPortRangesOperation {
	if op.mpr.pendingClose[op.unitName] == nil {
		op.mpr.pendingClose[op.unitName] = make(map[string][]network.PortRange)
	}
	op.mpr.pendingClose[op.unitName][endpoint] = append(op.mpr.pendingClose[op.unitName][endpoint], portRange)
	return op
}
