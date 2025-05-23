// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"strconv"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	domainapplication "github.com/juju/juju/domain/application"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/deployment"
	"github.com/juju/juju/domain/life"
	domainmachine "github.com/juju/juju/domain/machine"
	"github.com/juju/juju/domain/sequence"
	sequencestate "github.com/juju/juju/domain/sequence/state"
	"github.com/juju/juju/internal/errors"
)

// GetMachineNetNodeUUIDFromName returns the net node UUID for the named machine.
// The following errors may be returned:
// - [applicationerrors.MachineNotFound] if the machine does not exist
func (st *State) GetMachineNetNodeUUIDFromName(ctx context.Context, name machine.Name) (network.NetNodeUUID, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Capture(err)
	}

	var netNodeUUID network.NetNodeUUID
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		netNodeUUID, err = st.getMachineNetNodeUUIDFromName(ctx, tx, name)
		if err != nil {
			return errors.Capture(err)
		}
		return nil
	})
	if err != nil {
		return "", errors.Capture(err)
	}

	return netNodeUUID, nil
}

// placeMachine places the net node and machines if required, depending
// on the placement.
func (st *State) placeMachine(ctx context.Context, tx *sqlair.TX, directive deployment.Placement) (network.NetNodeUUID, []machine.Name, error) {
	switch directive.Type {
	case deployment.PlacementTypeUnset:
		// The placement is unset, so we need to create a machine for the
		// net node to link the unit to.
		machineName, err := st.nextMachineSequence(ctx, tx)
		if err != nil {
			return "", nil, errors.Capture(err)
		}

		_, netNode, err := st.insertMachineAndNetNode(ctx, tx, machineName)
		return netNode, []machine.Name{machineName}, errors.Capture(err)

	case deployment.PlacementTypeMachine:
		// Look up the existing machine by name (example: 0 or 0/lxd/0) and then
		// return the associated net node UUID.
		netNodeUUID, err := st.getMachineNetNodeUUIDFromName(ctx, tx, machine.Name(directive.Directive))
		if err != nil {
			return "", nil, errors.Capture(err)
		}
		return netNodeUUID, nil, nil

	case deployment.PlacementTypeContainer:
		// The placement is container scoped (example: lxd or lxd:0). If there
		// is no directive, we need to create a parent machine (the next in the
		// sequence) with the associated net node UUID. With a directive we need
		// to look up the existing machine and place it there. Then we need to
		// create a child machine for the container and link it to the parent
		// machine.
		machineUUID, machineName, err := st.acquireParentMachineForContainer(ctx, tx, directive.Directive)
		if err != nil {
			return "", nil, errors.Capture(err)
		}

		// Use the container type to determine the scope of the container.
		// For example, lxd.
		scope := directive.Container.String()
		childNetNode, childMachineName, err := st.insertChildMachineForContainerPlacement(ctx, tx, machineUUID, machineName, scope)
		if err != nil {
			return "", nil, errors.Errorf("inserting child machine for container placement: %w", err)
		}
		return childNetNode, []machine.Name{machineName, childMachineName}, nil

	case deployment.PlacementTypeProvider:
		// The placement is handled by the provider, so we need to create a
		// machine for the net node and then insert the provider placement
		// for the machine.
		machineName, err := st.nextMachineSequence(ctx, tx)
		if err != nil {
			return "", nil, errors.Capture(err)
		}

		machineUUID, netNode, err := st.insertMachineAndNetNode(ctx, tx, machineName)
		if err != nil {
			return "", nil, errors.Capture(err)
		}
		if err := st.insertMachineProviderPlacement(ctx, tx, machineUUID, directive.Directive); err != nil {
			return "", nil, errors.Errorf("inserting machine provider placement: %w", err)
		}
		return netNode, []machine.Name{machineName}, nil

	default:
		return "", nil, errors.Errorf("invalid placement type: %v", directive.Type)
	}
}

func (st *State) acquireParentMachineForContainer(ctx context.Context, tx *sqlair.TX, directive string) (machine.UUID, machine.Name, error) {
	// If the directive is not empty, we need to look up the existing machine
	// by name (example: 0) and then return the associated machine
	// UUID.
	if directive != "" {
		machineName := machine.Name(directive)
		machineUUID, err := st.getMachineUUIDFromName(ctx, tx, machineName)
		if err != nil {
			return "", "", errors.Capture(err)
		}
		return machineUUID, machineName, nil
	}

	// The directive is empty, so we need to create a new machine for the
	// parent machine. We need to get the next machine sequence and then
	// create the machine and net node.
	machineName, err := st.nextMachineSequence(ctx, tx)
	if err != nil {
		return "", "", errors.Capture(err)
	}

	machineUUID, _, err := st.insertMachineAndNetNode(ctx, tx, machineName)
	if err != nil {
		return "", "", errors.Capture(err)
	}
	return machineUUID, machineName, nil
}

func (st *State) getMachineUUIDFromName(ctx context.Context, tx *sqlair.TX, name machine.Name) (machine.UUID, error) {
	machine := machineNameWithMachineUUID{Name: name}
	query := `
SELECT &machineNameWithMachineUUID.uuid
FROM machine
WHERE name = $machineNameWithMachineUUID.name
`
	stmt, err := st.Prepare(query, machine)
	if err != nil {
		return "", errors.Capture(err)
	}
	err = tx.Query(ctx, stmt, machine).Get(&machine)
	if errors.Is(err, sqlair.ErrNoRows) {
		return "", errors.Errorf("machine %q not found", name).
			Add(applicationerrors.MachineNotFound)
	} else if err != nil {
		return "", errors.Errorf("querying machine %q: %w", name, err)
	}
	return machine.UUID, nil
}

func (st *State) getMachineNetNodeUUIDFromName(ctx context.Context, tx *sqlair.TX, name machine.Name) (network.NetNodeUUID, error) {
	machine := machineNameWithNetNode{Name: name}
	query := `
SELECT &machineNameWithNetNode.net_node_uuid
FROM machine
WHERE name = $machineNameWithNetNode.name
`
	stmt, err := st.Prepare(query, machine)
	if err != nil {
		return "", errors.Capture(err)
	}
	err = tx.Query(ctx, stmt, machine).Get(&machine)
	if errors.Is(err, sqlair.ErrNoRows) {
		return "", errors.Errorf("machine %q not found", name).
			Add(applicationerrors.MachineNotFound)
	} else if err != nil {
		return "", errors.Errorf("querying machine %q: %w", name, err)
	}
	return machine.NetNodeUUID, nil
}

func (st *State) insertNetNode(ctx context.Context, tx *sqlair.TX) (network.NetNodeUUID, error) {
	uuid, err := network.NewNetNodeUUID()
	if err != nil {
		return "", errors.Capture(err)
	}

	netNodeUUID := netNodeUUID{NetNodeUUID: uuid}

	createNode := `INSERT INTO net_node (uuid) VALUES ($netNodeUUID.*)`
	createNodeStmt, err := st.Prepare(createNode, netNodeUUID)
	if err != nil {
		return "", errors.Capture(err)
	}

	if err := tx.Query(ctx, createNodeStmt, netNodeUUID).Run(); err != nil {
		return "", errors.Errorf("creating net node for machine: %w", err)
	}

	return netNodeUUID.NetNodeUUID, nil
}

func (st *State) insertMachineAndNetNode(ctx context.Context, tx *sqlair.TX, machineName machine.Name) (machine.UUID, network.NetNodeUUID, error) {
	netNodeUUID, err := st.insertNetNode(ctx, tx)
	if err != nil {
		return "", "", errors.Capture(err)
	}

	machineUUID, err := machine.NewUUID()
	if err != nil {
		return "", "", errors.Capture(err)
	}

	m := createMachine{
		MachineUUID: machineUUID,
		NetNodeUUID: netNodeUUID,
		Name:        machineName,
		LifeID:      life.Alive,
	}

	createMachineQuery := `
INSERT INTO machine (uuid, net_node_uuid, name, life_id)
VALUES ($createMachine.*);
`
	createMachineStmt, err := st.Prepare(createMachineQuery, m)
	if err != nil {
		return "", "", errors.Capture(err)
	}
	if err := tx.Query(ctx, createMachineStmt, m).Run(); err != nil {
		return "", "", errors.Errorf("creating new machine: %w", err)
	}

	if err := st.insertMachineInstance(ctx, tx, machineUUID); err != nil {
		return "", "", errors.Errorf("inserting machine instance: %w", err)
	}

	now := st.clock.Now()

	machineStatusID, err := encodeMachineStatus(domainmachine.MachineStatusPending)
	if err != nil {
		return "", "", errors.Capture(err)
	}
	machineInstanceStatusID, err := encodeCloudInstanceStatus(domainmachine.InstanceStatusPending)
	if err != nil {
		return "", "", errors.Capture(err)
	}

	if err := st.insertMachineStatus(ctx, tx, machineUUID, setStatusInfo{
		StatusID: machineStatusID,
		Updated:  ptr(now),
	}); err != nil {
		return "", "", errors.Errorf("inserting machine status: %w", err)
	}
	if err := st.insertMachineInstanceStatus(ctx, tx, machineUUID, setStatusInfo{
		StatusID: machineInstanceStatusID,
		Updated:  ptr(now),
	}); err != nil {
		return "", "", errors.Errorf("inserting machine instance status: %w", err)
	}

	return machineUUID, netNodeUUID, nil
}

func (st *State) insertMachineProviderPlacement(ctx context.Context, tx *sqlair.TX, machineUUID machine.UUID, placement string) error {
	machinePlacement := machinePlacement{
		MachineUUID: machineUUID,
		ScopeID:     0,
		Directive:   placement,
	}
	query := `
INSERT INTO machine_placement (machine_uuid, scope_id, directive)
VALUES ($machinePlacement.*);
`
	stmt, err := st.Prepare(query, machinePlacement)
	if err != nil {
		return errors.Capture(err)
	}
	if err := tx.Query(ctx, stmt, machinePlacement).Run(); err != nil {
		return errors.Errorf("inserting machine placement: %w", err)
	}
	return nil
}

func (st *State) insertChildMachineForContainerPlacement(
	ctx context.Context,
	tx *sqlair.TX,
	parentUUID machine.UUID,
	parentName machine.Name,
	scope string,
) (network.NetNodeUUID, machine.Name, error) {
	machineName, err := st.nextContainerSequence(ctx, tx, scope, parentName)
	if err != nil {
		return "", "", errors.Capture(err)
	}

	machineUUID, netNodeUUID, err := st.insertMachineAndNetNode(ctx, tx, machineName)
	if err != nil {
		return "", "", errors.Capture(err)
	}

	parentMachineQuery := `
INSERT INTO machine_parent (parent_uuid, machine_uuid)
VALUES ($machineParent.*);
`
	p := machineParent{
		ParentUUID:  parentUUID,
		MachineUUID: machineUUID,
	}
	parentMachineStmt, err := st.Prepare(parentMachineQuery, p)
	if err != nil {
		return "", "", errors.Capture(err)
	}

	if err := tx.Query(ctx, parentMachineStmt, p).Run(); err != nil {
		return "", "", errors.Errorf("creating new container machine parent: %w", err)
	}
	return netNodeUUID, machineName, nil
}

func (st *State) nextMachineSequence(ctx context.Context, tx *sqlair.TX) (machine.Name, error) {
	namespace := domainapplication.MachineSequenceNamespace
	seq, err := sequencestate.NextValue(ctx, st, tx, namespace)
	if err != nil {
		return "", errors.Errorf("getting next machine sequence: %w", err)
	}

	return machine.Name(strconv.FormatUint(seq, 10)), nil
}

func (st *State) nextContainerSequence(ctx context.Context, tx *sqlair.TX, scope string, parentName machine.Name) (machine.Name, error) {
	namespace := sequence.MakePrefixNamespace(domainapplication.ContainerSequenceNamespace, parentName.String())
	seq, err := sequencestate.NextValue(ctx, st, tx, namespace)
	if err != nil {
		return "", errors.Errorf("getting next container machine sequence: %w", err)
	}

	return parentName.NamedChild(scope, strconv.FormatUint(seq, 10))
}

func (st *State) insertMachineInstance(
	ctx context.Context,
	tx *sqlair.TX,
	mUUID machine.UUID,
) error {
	// Prepare query for setting the machine cloud instance.
	setInstanceData := `
INSERT INTO machine_cloud_instance (*)
VALUES ($machineInstanceUUID.*);
`
	setInstanceDataStmt, err := st.Prepare(setInstanceData, machineInstanceUUID{})
	if err != nil {
		return errors.Capture(err)
	}

	return tx.Query(ctx, setInstanceDataStmt, machineInstanceUUID{
		MachineUUID: mUUID,
	}).Run()
}

func (st *State) insertMachineStatus(ctx context.Context, tx *sqlair.TX, mUUID machine.UUID, status setStatusInfo) error {
	// Prepare query for setting machine status
	statusQuery := `
INSERT INTO machine_status (*)
VALUES ($setMachineStatus.*)
  ON CONFLICT (machine_uuid)
  DO UPDATE SET
  	status_id = excluded.status_id,
	message = excluded.message,
	updated_at = excluded.updated_at,
	data = excluded.data;
`
	statusQueryStmt, err := st.Prepare(statusQuery, setMachineStatus{})
	if err != nil {
		return errors.Capture(err)
	}

	// Query for setting the machine status.
	err = tx.Query(ctx, statusQueryStmt, setMachineStatus{
		MachineUUID: mUUID,
		StatusID:    status.StatusID,
		Message:     status.Message,
		Data:        status.Data,
		Updated:     status.Updated,
	}).Run()
	if err != nil {
		return errors.Errorf("setting machine status for machine %q: %w", mUUID, err)
	}

	return nil
}

func (st *State) insertMachineInstanceStatus(
	ctx context.Context,
	tx *sqlair.TX,
	mUUID machine.UUID,
	status setStatusInfo,
) error {
	// Prepare query for setting the machine cloud instance status
	statusQuery := `
INSERT INTO machine_cloud_instance_status (*)
VALUES ($setMachineStatus.*)
ON CONFLICT (machine_uuid)
DO UPDATE SET 
  status_id = excluded.status_id, 
message = excluded.message, 
updated_at = excluded.updated_at,
data = excluded.data;
`
	statusQueryStmt, err := st.Prepare(statusQuery, setMachineStatus{})
	if err != nil {
		return errors.Capture(err)
	}

	// Query for setting the machine cloud instance status
	err = tx.Query(ctx, statusQueryStmt, setMachineStatus{
		MachineUUID: mUUID,
		StatusID:    status.StatusID,
		Message:     status.Message,
		Data:        status.Data,
		Updated:     status.Updated,
	}).Run()
	if err != nil {
		return errors.Errorf("setting machine status for machine %q: %w", mUUID, err)
	}
	return nil
}

func encodeMachineStatus(s domainmachine.MachineStatusType) (int, error) {
	// This is taken from the machine domain. We only support the pending
	// when creating a machine, so we don't need to worry about the other
	// statuses.
	var result int
	switch s {
	case domainmachine.MachineStatusPending:
		result = 3
	default:
		return -1, errors.Errorf("unknown status %q", s)
	}
	return result, nil
}

func encodeCloudInstanceStatus(s domainmachine.InstanceStatusType) (int, error) {
	// This is taken from the machine domain. We only support the pending
	// when creating a machine, so we don't need to worry about the other
	// statuses.
	var result int
	switch s {
	case domainmachine.InstanceStatusPending:
		result = 1
	default:
		return -1, errors.Errorf("unknown status %q", s)
	}
	return result, nil
}
