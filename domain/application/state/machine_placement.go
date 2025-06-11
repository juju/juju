// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"strconv"

	"github.com/canonical/sqlair"

	coremachine "github.com/juju/juju/core/machine"
	domainapplication "github.com/juju/juju/domain/application"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/deployment"
	"github.com/juju/juju/domain/life"
	domainnetwork "github.com/juju/juju/domain/network"
	"github.com/juju/juju/domain/sequence"
	sequencestate "github.com/juju/juju/domain/sequence/state"
	domainstatus "github.com/juju/juju/domain/status"
	"github.com/juju/juju/internal/errors"
)

// GetMachineNetNodeUUIDFromName returns the net node UUID for the named machine.
// The following errors may be returned:
// - [applicationerrors.MachineNotFound] if the machine does not exist
func (st *State) GetMachineNetNodeUUIDFromName(ctx context.Context, name coremachine.Name) (string, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Capture(err)
	}

	var netNodeUUID string
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
// It returns the net node UUID for the machine and a list of child
// machine names that were created as part of the placement.
func (st *State) placeMachine(
	ctx context.Context,
	tx *sqlair.TX,
	directive deployment.Placement,
	platform deployment.Platform,
) (string, []coremachine.Name, error) {
	st.logger.Debugf(ctx, "placing machine with directive %q, platform %q, isController %t",
		directive.Directive, platform, isController)

	switch directive.Type {
	case deployment.PlacementTypeUnset:
		// The placement is unset, so we need to create a machine for the
		// net node to link the unit to.
		machineName, err := st.nextMachineSequence(ctx, tx)
		if err != nil {
			return "", nil, errors.Capture(err)
		}

		_, netNode, err := st.insertMachineAndNetNode(ctx, tx, machineName, platform)
		return netNode, []coremachine.Name{
			machineName,
		}, errors.Capture(err)

	case deployment.PlacementTypeMachine:
		// Look up the existing machine by name (example: 0 or 0/lxd/0) and then
		// return the associated net node UUID.
		netNodeUUID, err := st.getMachineNetNodeUUIDFromName(ctx, tx, coremachine.Name(directive.Directive))
		return netNodeUUID, nil, errors.Capture(err)

	case deployment.PlacementTypeContainer:
		// The placement is container scoped (example: lxd or lxd:0). If there
		// is no directive, we need to create a parent machine (the next in the
		// sequence) with the associated net node UUID. With a directive we need
		// to look up the existing machine and place it there. Then we need to
		// create a child machine for the container and link it to the parent
		// machine.
		machineUUID, machineName, err := st.acquireParentMachineForContainer(ctx, tx, directive.Directive, platform)
		if err != nil {
			return "", nil, errors.Capture(err)
		}

		// Use the container type to determine the scope of the container.
		// For example, lxd.
		scope := directive.Container.String()
		childNetNode, childMachineName, err := st.insertChildMachineForContainerPlacement(ctx, tx, machineUUID, machineName, scope, platform)
		if err != nil {
			return "", nil, errors.Errorf("inserting child machine for container placement: %w", err)
		}
		return childNetNode, []coremachine.Name{
			machineName,
			childMachineName,
		}, nil

	case deployment.PlacementTypeProvider:
		// The placement is handled by the provider, so we need to create a
		// machine for the net node and then insert the provider placement
		// for the machine.
		machineName, err := st.nextMachineSequence(ctx, tx)
		if err != nil {
			return "", nil, errors.Capture(err)
		}

		machine, netNode, err := st.insertMachineAndNetNode(ctx, tx, machineName, platform)
		if err != nil {
			return "", nil, errors.Capture(err)
		}
		if err := st.insertMachineProviderPlacement(ctx, tx, machine, directive.Directive); err != nil {
			return "", nil, errors.Errorf("inserting machine provider placement: %w", err)
		}
		return netNode, []coremachine.Name{
			machineName,
		}, nil

	default:
		return "", nil, errors.Errorf("invalid placement type: %v", directive.Type)
	}
}

func (st *State) acquireParentMachineForContainer(
	ctx context.Context,
	tx *sqlair.TX,
	directive string,
	platform deployment.Platform,
) (coremachine.UUID, coremachine.Name, error) {
	// If the directive is not empty, we need to look up the existing machine
	// by name (example: 0) and then return the associated machine
	// UUID.
	if directive != "" {
		machineName := coremachine.Name(directive)
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

	machineUUID, _, err := st.insertMachineAndNetNode(ctx, tx, machineName, platform)
	if err != nil {
		return "", "", errors.Capture(err)
	}
	return machineUUID, machineName, nil
}

func (st *State) getMachineUUIDFromName(ctx context.Context, tx *sqlair.TX, name coremachine.Name) (coremachine.UUID, error) {
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

func (st *State) getMachineNetNodeUUIDFromName(ctx context.Context, tx *sqlair.TX, name coremachine.Name) (string, error) {
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

func (st *State) insertNetNode(ctx context.Context, tx *sqlair.TX) (string, error) {
	uuid, err := domainnetwork.NewNetNodeUUID()
	if err != nil {
		return "", errors.Capture(err)
	}

	netNodeUUID := netNodeUUID{NetNodeUUID: uuid.String()}

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

func (st *State) insertMachineAndNetNode(
	ctx context.Context,
	tx *sqlair.TX,
	machineName coremachine.Name,
	platform deployment.Platform,
) (coremachine.UUID, string, error) {
	netNodeUUID, err := st.insertNetNode(ctx, tx)
	if err != nil {
		return "", "", errors.Capture(err)
	}

	machineUUID, err := coremachine.NewUUID()
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

	if err := st.insertMachinePlatform(ctx, tx, machineUUID, platform); err != nil {
		return "", "", errors.Errorf("inserting machine platform: %w", err)
	}

	if err := st.insertMachineInstance(ctx, tx, machineUUID); err != nil {
		return "", "", errors.Errorf("inserting machine instance: %w", err)
	}

	now := st.clock.Now()

	machineStatusID, err := domainstatus.EncodeMachineStatus(domainstatus.MachineStatusPending)
	if err != nil {
		return "", "", errors.Capture(err)
	}
	machineInstanceStatusID, err := domainstatus.EncodeCloudInstanceStatus(domainstatus.InstanceStatusPending)
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

func (st *State) insertMachineProviderPlacement(ctx context.Context, tx *sqlair.TX, machineUUID coremachine.UUID, placement string) error {
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
	parentUUID coremachine.UUID,
	parentName coremachine.Name,
	scope string,
	platform deployment.Platform,
) (string, coremachine.Name, error) {
	machineName, err := st.nextContainerSequence(ctx, tx, scope, parentName)
	if err != nil {
		return "", "", errors.Capture(err)
	}

	machineUUID, netNodeUUID, err := st.insertMachineAndNetNode(ctx, tx, machineName, platform)
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

func (st *State) nextMachineSequence(ctx context.Context, tx *sqlair.TX) (coremachine.Name, error) {
	namespace := domainapplication.MachineSequenceNamespace
	seq, err := sequencestate.NextValue(ctx, st, tx, namespace)
	if err != nil {
		return "", errors.Errorf("getting next machine sequence: %w", err)
	}

	return coremachine.Name(strconv.FormatUint(seq, 10)), nil
}

func (st *State) nextContainerSequence(ctx context.Context, tx *sqlair.TX, scope string, parentName coremachine.Name) (coremachine.Name, error) {
	namespace := sequence.MakePrefixNamespace(domainapplication.ContainerSequenceNamespace, parentName.String())
	seq, err := sequencestate.NextValue(ctx, st, tx, namespace)
	if err != nil {
		return "", errors.Errorf("getting next container machine sequence: %w", err)
	}

	return parentName.NamedChild(scope, strconv.FormatUint(seq, 10))
}

func (st *State) insertMachinePlatform(
	ctx context.Context,
	tx *sqlair.TX,
	mUUID coremachine.UUID,
	platform deployment.Platform,
) error {
	// Prepare query for setting the machine cloud instance.
	query := `
INSERT INTO machine_platform (*)
VALUES ($machinePlatformUUID.*);
`
	stmt, err := st.Prepare(query, machinePlatformUUID{})
	if err != nil {
		return errors.Capture(err)
	}

	arch, err := encodeArchitecture(platform.Architecture)
	if err != nil {
		return errors.Errorf("encoding architecture %q: %w", platform.Architecture, err)
	}

	var channel sql.Null[string]
	if platform.Channel != "" {
		channel = sql.Null[string]{V: platform.Channel, Valid: true}
	}

	osType, err := encodeOSType(platform.OSType)
	if err != nil {
		return errors.Errorf("encoding OS type %q: %w", platform.OSType, err)
	}

	return tx.Query(ctx, stmt, machinePlatformUUID{
		MachineUUID:    mUUID,
		OSID:           osType,
		Channel:        channel,
		ArchitectureID: arch,
	}).Run()
}

func (st *State) insertMachineInstance(
	ctx context.Context,
	tx *sqlair.TX,
	mUUID coremachine.UUID,
) error {
	// Prepare query for setting the machine cloud instance.
	setInstanceData := `
INSERT INTO machine_cloud_instance (*)
VALUES ($machineUUID.*);
`
	setInstanceDataStmt, err := st.Prepare(setInstanceData, machineUUID{})
	if err != nil {
		return errors.Capture(err)
	}

	return tx.Query(ctx, setInstanceDataStmt, machineUUID{
		MachineUUID: mUUID,
	}).Run()
}

func (st *State) insertMachineStatus(ctx context.Context, tx *sqlair.TX, mUUID coremachine.UUID, status setStatusInfo) error {
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
	mUUID coremachine.UUID,
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
