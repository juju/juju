// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
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
		machineName, err := st.nextMachineSequence(ctx, tx)
		if err != nil {
			return "", errors.Capture(err)
		}

		_, netNode, err := st.insertMachineForNetNode(ctx, tx, machineName)
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
		machineName, err := st.nextMachineSequence(ctx, tx)
		if err != nil {
			return "", errors.Capture(err)
		}

		machineUUID, _, err := st.insertMachineForNetNode(ctx, tx, machineName)
		if err != nil {
			return "", errors.Capture(err)
		}

		// Use the container type to determine the scope of the container.
		// For example, lxd.
		scope := directive.Container.String()
		childNetNode, err := st.insertChildMachineForContainerPlacement(ctx, tx, machineUUID, machineName, scope)
		if err != nil {
			return "", errors.Errorf("inserting child machine for container placement: %w", err)
		}
		return childNetNode, nil

	case placement.PlacementTypeProvider:
		// The placement is handled by the provider, so we need to create a
		// machine for the net node and then insert the provider placement
		// for the machine.
		machineName, err := st.nextMachineSequence(ctx, tx)
		if err != nil {
			return "", errors.Capture(err)
		}

		machine, netNode, err := st.insertMachineForNetNode(ctx, tx, machineName)
		if err != nil {
			return "", errors.Capture(err)
		}
		if err := st.insertMachineProviderPlacement(ctx, tx, machine, directive.Directive); err != nil {
			return "", errors.Errorf("inserting machine provider placement: %w", err)
		}
		return netNode, nil

	default:
		return "", errors.Errorf("invalid placement type: %v", directive.Type)
	}
}

func (st *State) getMachineNetNodeUUIDFromName(ctx context.Context, tx *sqlair.TX, name machine.Name) (string, error) {
	machine := machineName{Name: name}
	query := `
SELECT &machineName.net_node_uuid
FROM machine
WHERE name = $machineName.name
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
	uuid, err := uuid.NewUUID()
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

func (st *State) insertMachineForNetNode(ctx context.Context, tx *sqlair.TX, machineName machine.Name) (machine.UUID, string, error) {
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
) (string, error) {
	machineName, err := st.nextContainerSequence(ctx, tx, scope, parentName)
	if err != nil {
		return "", errors.Capture(err)
	}

	machineUUID, netNodeUUID, err := st.insertMachineForNetNode(ctx, tx, machineName)
	if err != nil {
		return "", errors.Capture(err)
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
		return "", errors.Capture(err)
	}

	if err := tx.Query(ctx, parentMachineStmt, p).Run(); err != nil {
		return "", errors.Errorf("creating new container machine parent: %w", err)
	}
	return netNodeUUID, nil
}

func (st *State) nextMachineSequence(ctx context.Context, tx *sqlair.TX) (machine.Name, error) {
	seq, err := sequence.NextValue(ctx, st, tx, machineSequenceNamespace)
	if err != nil {
		return "", errors.Errorf("getting next machine sequence: %w", err)
	}

	return machine.Name(strconv.FormatUint(seq, 10)), nil
}

func (st *State) nextContainerSequence(ctx context.Context, tx *sqlair.TX, scope string, parentName machine.Name) (machine.Name, error) {
	seq, err := sequence.NextValue(ctx, st, tx, sequence.MakePrefixNamespace(containerSequenceNamespace, parentName.String()))
	if err != nil {
		return "", errors.Errorf("getting next container machine sequence: %w", err)
	}

	return parentName.NamedChild(scope, strconv.FormatUint(seq, 10))
}
