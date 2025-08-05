// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"

	coremachine "github.com/juju/juju/core/machine"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	domainnetwork "github.com/juju/juju/domain/network"
	"github.com/juju/juju/internal/errors"
)

// GetMachineNetNodeUUIDFromName returns the net node UUID for the named machine.
// The following errors may be returned:
// - [applicationerrors.MachineNotFound] if the machine does not exist
func (st *State) GetMachineNetNodeUUIDFromName(ctx context.Context, name coremachine.Name) (string, error) {
	db, err := st.DB(ctx)
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

// IsMachineController returns whether the machine is a controller machine.
// It returns a NotFound if the given machine doesn't exist.
func (s *State) IsMachineController(ctx context.Context, mName string) (bool, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return false, errors.Capture(err)
	}

	var result count
	query := `
SELECT COUNT(*) AS &count.count
FROM   v_machine_is_controller
WHERE  machine_uuid = $entityUUID.uuid
`
	queryStmt, err := s.Prepare(query, entityUUID{}, result)
	if err != nil {
		return false, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		mUUID, err := s.getMachineUUIDFromName(ctx, tx, mName)
		if err != nil {
			return err
		}

		if err := tx.Query(ctx, queryStmt, mUUID).Get(&result); errors.Is(err, sqlair.ErrNoRows) {
			// If no rows are returned, the machine is not a controller.
			return nil
		} else if err != nil {
			return errors.Errorf("querying if machine %q is a controller: %w", mName, err)
		}
		return nil
	})
	if err != nil {
		return false, errors.Errorf("checking if machine %q is a controller: %w", mName, err)
	}

	return result.Count == 1, nil
}

func (st *State) getMachineUUIDFromName(ctx context.Context, tx *sqlair.TX, mName string) (entityUUID, error) {
	machineNameParam := entityName{Name: mName}
	machineUUIDoutput := entityUUID{}
	query := `SELECT uuid AS &entityUUID.uuid FROM machine WHERE name = $entityName.name`
	queryStmt, err := st.Prepare(query, machineNameParam, machineUUIDoutput)
	if err != nil {
		return entityUUID{}, errors.Capture(err)
	}

	if err := tx.Query(ctx, queryStmt, machineNameParam).Get(&machineUUIDoutput); errors.Is(err, sqlair.ErrNoRows) {
		return entityUUID{}, errors.Errorf("machine %q: %w", mName, machineerrors.MachineNotFound)
	} else if err != nil {
		return entityUUID{}, errors.Errorf("querying UUID for machine %q: %w", mName, err)
	}
	return machineUUIDoutput, nil
}

// GetMachinesForApplication returns the names of the machines which have a unit.
// of the specified application deployed to it.
// The following errors may be returned:
// - [applicationerrors.ApplicationNotFound] if the application does not exist.
func (st *State) GetMachinesForApplication(ctx context.Context, appUUID string) ([]string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Errorf("cannot get database find names for machines %q: %w", appUUID, err)
	}

	ident := entityUUID{UUID: appUUID}

	checkAppExistsStmt, err := st.Prepare(`
SELECT COUNT(*) AS &count.count
FROM application
WHERE uuid = $entityUUID.uuid
`, count{}, ident)
	if err != nil {
		return nil, errors.Capture(err)
	}

	selectMachinesStmt, err := st.Prepare(`
SELECT machine.name AS  &entityName.name
FROM machine
JOIN unit ON machine.net_node_uuid = unit.net_node_uuid
WHERE unit.application_uuid = $entityUUID.uuid
`, entityName{}, ident)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var machineNames []entityName
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var count count
		err := tx.Query(ctx, checkAppExistsStmt, ident).Get(&count)
		if err != nil {
			return errors.Errorf("checking if application %q exists: %w", appUUID, err)
		}
		if count.Count == 0 {
			return errors.Errorf("application %q does not exist", appUUID).Add(applicationerrors.ApplicationNotFound)
		}

		err = tx.Query(ctx, selectMachinesStmt, ident).GetAll(&machineNames)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("querying machine names for application %q: %w", appUUID, err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Errorf("getting machine names for application %q: %w", appUUID, err)
	}
	return transform.Slice(machineNames, func(v entityName) string {
		return v.Name
	}), nil
}
