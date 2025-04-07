// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"
	"strconv"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/machine"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/placement"
	"github.com/juju/juju/domain/sequence"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// placeNetNodeMachines places the net node and machines if required, depending
// on the placement.
func (st *State) placeNetNodeMachines(ctx context.Context, tx *sqlair.TX, directive placement.Placement) (string, error) {
	switch directive.Type {
	case placement.PlacementTypeUnset:
		// The placement is unset, so we need to create a machine for the
		// net node to link the unit to.
		_, _, netNode, err := st.insertMachineForNetNode(ctx, tx)
		return netNode, errors.Capture(err)

	case placement.PlacementTypeMachine:
		// Look up the existing machine by name (example: 0 or 0/lxd/0) and then
		// return the associated net node UUID.
		return st.getMachineNetNodeUUIDFromName(ctx, tx, machine.Name(directive.Directive))

	case placement.PlacementTypeContainer:
		// The placement is container scoped (example: lxd), so we need to
		// create a parent machine (the next in the sequence) with the
		// associated net node UUID. Then we need to create a child machine
		// for the container and link it to the parent machine.
		machineUUID, machineName, netNode, err := st.insertMachineForNetNode(ctx, tx)
		if err != nil {
			return "", errors.Capture(err)
		}
		if err := st.insertChildMachineForContainerPlacement(ctx, tx, machineUUID, machineName, netNode, directive.Directive); err != nil {
			return "", errors.Errorf("inserting child machine for container placement: %w", err)
		}
		return netNode, nil

	case placement.PlacementTypeProvider:
		// The placement is handled by the provider, so we need to create a
		// machine for the net node and then insert the provider placement
		// for the machine.
		machine, _, netNode, err := st.insertMachineForNetNode(ctx, tx)
		if err != nil {
			return "", errors.Capture(err)
		}
		if err := st.insertMachineProviderPlacement(ctx, tx, machine, directive.Directive); err != nil {
			return "", errors.Errorf("inserting machine provider placement: %w", err)
		}
		return netNode, nil

	default:
		return "", errors.Errorf("invalid placement type %q", directive.Type)
	}
}

func (st *State) getMachineNetNodeUUIDFromName(ctx context.Context, tx *sqlair.TX, name machine.Name) (string, error) {
	machine := machineName{Name: name}
	query := `
SELECT &machineUUID.net_node_uuid
FROM machine
WHERE name = $machineUUID.name
`
	stmt, err := st.Prepare(query, machine, machineUUID{})
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

func (st *State) insertMachineForNetNode(ctx context.Context, tx *sqlair.TX) (machine.UUID, machine.Name, string, error) {
	uuid, err := uuid.NewUUID()
	if err != nil {
		return "", "", "", errors.Capture(err)
	}

	netNodeUUID := netNodeUUID{NetNodeUUID: uuid.String()}

	createNode := `INSERT INTO net_node (uuid) VALUES ($netNodeUUID.*)`
	createNodeStmt, err := st.Prepare(createNode, netNodeUUID)
	if err != nil {
		return "", "", "", errors.Capture(err)
	}

	if err := tx.Query(ctx, createNodeStmt, netNodeUUID).Run(); err != nil {
		return "", "", "", errors.Errorf("creating net node for machine: %w", err)
	}

	machineUUID, err := machine.NewUUID()
	if err != nil {
		return "", "", "", errors.Capture(err)
	}

	seq, err := sequence.NextValue(ctx, st, tx, machineSequenceNamespace)
	if err != nil {
		return "", "", "", errors.Errorf("getting next machine sequence: %w", err)
	}

	machineName := machine.Name(fmt.Sprintf("%d", seq))

	m := createMachine{
		MachineUUID: machineUUID,
		NetNodeUUID: netNodeUUID.NetNodeUUID,
		Name:        machineName,
		LifeID:      life.Alive,
	}

	createMachineQuery := `
INSERT INTO machine (uuid, net_node_uuid, name, life_id)
VALUES ($createMachine.*);
`
	createMachineStmt, err := st.Prepare(createMachineQuery, m)
	if err != nil {
		return "", "", "", errors.Capture(err)
	}
	if err := tx.Query(ctx, createMachineStmt, m).Run(); err != nil {
		return "", "", "", errors.Errorf("creating new machine: %w", err)
	}

	return machineUUID, machineName, netNodeUUID.NetNodeUUID, nil
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
	netNode string,
	scope string,
) error {
	machineUUID, err := machine.NewUUID()
	if err != nil {
		return errors.Capture(err)
	}

	seq, err := sequence.NextValue(ctx, st, tx, fmt.Sprintf(containerSequenceNamespace, parentName))
	if err != nil {
		return errors.Errorf("getting next container machine sequence: %w", err)
	}

	machineName, err := parentName.NamedChild(scope, strconv.FormatUint(seq, 10))
	if err != nil {
		return errors.Errorf("creating container machine name: %w", err)
	}

	m := createMachine{
		MachineUUID: machineUUID,
		NetNodeUUID: netNode,
		Name:        machineName,
		LifeID:      life.Alive,
	}

	createMachineQuery := `
INSERT INTO machine (uuid, net_node_uuid, name, life_id)
VALUES ($createMachine.*);
`
	createMachineStmt, err := st.Prepare(createMachineQuery, m)
	if err != nil {
		return errors.Capture(err)
	}
	if err := tx.Query(ctx, createMachineStmt, m).Run(); err != nil {
		return errors.Errorf("creating new container machine: %w", err)
	}

	parentMachineQuery := `
INSERT INTO machine_parent (parent_uuid, child_uuid)
VALUES ($machineParent.*);
`
	p := machineParent{
		ParentUUID:  parentUUID,
		MachineUUID: machineUUID,
	}
	parentMachineStmt, err := st.Prepare(parentMachineQuery, p)
	if err != nil {
		return errors.Capture(err)
	}

	if err := tx.Query(ctx, parentMachineStmt, p).Run(); err != nil {
		return errors.Errorf("creating new container machine parent: %w", err)
	}
	return nil
}
