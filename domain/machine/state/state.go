// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"
	"github.com/juju/errors"

	coredb "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/domain"
	blockdevice "github.com/juju/juju/domain/blockdevice/state"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/internal/database"
)

// State describes retrieval and persistence methods for storage.
type State struct {
	*domain.StateBase
	logger logger.Logger
}

// NewState returns a new state reference.
func NewState(factory coredb.TxnRunnerFactory, logger logger.Logger) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
		logger:    logger,
	}
}

// CreateMachine creates or updates the specified machine.
// TODO - this just creates a minimal row for now.
func (st *State) CreateMachine(ctx context.Context, machineName machine.Name, nodeUUID, machineUUID string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	machineNameParam := sqlair.M{"name": machineName}
	query := `SELECT &M.uuid FROM machine WHERE name = $M.name`
	queryStmt, err := st.Prepare(query, machineNameParam)
	if err != nil {
		return errors.Trace(err)
	}

	createMachine := `
INSERT INTO machine (uuid, net_node_uuid, name, life_id)
VALUES ($M.machine_uuid, $M.net_node_uuid, $M.name, $M.life_id)
`
	createMachineStmt, err := st.Prepare(createMachine, machineNameParam)
	if err != nil {
		return errors.Trace(err)
	}

	createNode := `INSERT INTO net_node (uuid) VALUES ($M.net_node_uuid)`
	createNodeStmt, err := st.Prepare(createNode, machineNameParam)
	if err != nil {
		return errors.Trace(err)
	}

	createParams := sqlair.M{
		"machine_uuid":  machineUUID,
		"net_node_uuid": nodeUUID,
		"name":          machineName,
		"life_id":       life.Alive,
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		result := sqlair.M{}
		err := tx.Query(ctx, queryStmt, machineNameParam).Get(&result)
		if err != nil {
			if !errors.Is(err, sqlair.ErrNoRows) {
				return errors.Annotatef(err, "querying machine %q", machineName)
			}
		}
		// For now, we just care if the minimal machine row already exists.
		if err == nil {
			return nil
		}

		if err := tx.Query(ctx, createNodeStmt, createParams).Run(); err != nil {
			return errors.Annotatef(err, "creating net node row for machine %q", machineName)
		}
		if err := tx.Query(ctx, createMachineStmt, createParams).Run(); err != nil {
			return errors.Annotatef(err, "creating machine row for machine %q", machineName)
		}
		return nil
	})
	return errors.Annotatef(err, "inserting machine %q", machineName)
}

// DeleteMachine deletes the specified machine and any dependent child records.
// TODO - this just deals with child block devices for now.
func (st *State) DeleteMachine(ctx context.Context, machineName machine.Name) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	machineNameParam := sqlair.M{"name": machineName}

	queryMachine := `SELECT uuid AS &machineUUID.* FROM machine WHERE name = $M.name`
	queryMachineStmt, err := st.Prepare(queryMachine, machineNameParam, machineUUID{})
	if err != nil {
		return errors.Trace(err)
	}

	deleteMachine := `DELETE FROM machine WHERE name = $M.name`
	deleteMachineStmt, err := st.Prepare(deleteMachine, machineNameParam)
	if err != nil {
		return errors.Trace(err)
	}

	deleteNode := `
DELETE FROM net_node WHERE uuid IN
(SELECT net_node_uuid FROM machine WHERE name = $M.name) 
`
	deleteNodeStmt, err := st.Prepare(deleteNode, machineNameParam)
	if err != nil {
		return errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		result := machineUUID{}
		err = tx.Query(ctx, queryMachineStmt, machineNameParam).Get(&result)
		// Machine already deleted is a no op.
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Annotatef(err, "looking up UUID for machine %q", machineName)
		}

		machineUUID := result.UUID

		if err := blockdevice.RemoveMachineBlockDevices(ctx, tx, machineUUID); err != nil {
			return errors.Annotatef(err, "deleting block devices for machine %q", machineName)
		}

		if err := tx.Query(ctx, deleteMachineStmt, machineNameParam).Run(); err != nil {
			return errors.Annotatef(err, "deleting machine %q", machineName)
		}
		if err := tx.Query(ctx, deleteNodeStmt, machineNameParam).Run(); err != nil {
			return errors.Annotatef(err, "deleting net node for machine  %q", machineName)
		}

		return nil
	})
	return errors.Annotatef(err, "deleting machine %q", machineName)
}

// InitialWatchStatement returns the table and the initial watch statement
// for the machines.
func (s *State) InitialWatchStatement() (string, string) {
	return "machine", "SELECT name FROM machine"
}

// GetMachineLife returns the life status of the specified machine.
func (st *State) GetMachineLife(ctx context.Context, machineName machine.Name) (*life.Life, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	machineNameParam := sqlair.M{"name": machineName}
	queryForLife := `SELECT life_id as &machineLife.life_id FROM machine WHERE name = $M.name`
	lifeStmt, err := st.Prepare(queryForLife, machineNameParam, machineLife{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	var lifeResult life.Life
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		result := machineLife{}
		err := tx.Query(ctx, lifeStmt, machineNameParam).Get(&result)
		if err != nil {
			return errors.Annotatef(err, "looking up life for machine %q", machineName)
		}

		lifeResult = result.ID

		return nil
	})

	if err != nil && errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.NotFoundf("machine %q", machineName)
	}

	return &lifeResult, errors.Annotatef(err, "getting life status for machines %q", machineName)
}

// AllMachineNames retrieves the names of all machines in the model.
func (st *State) AllMachineNames(ctx context.Context) ([]machine.Name, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	query := `SELECT name AS &machineName.* FROM machine`
	queryStmt, err := st.Prepare(query, machineName{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	var results []machineName
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, queryStmt).GetAll(&results)
	})
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Annotate(err, "querying all machines")
	}

	// Transform the results ([]machineName) into a slice of machine.Name.
	machineNames := transform.Slice[machineName, machine.Name](results, func(r machineName) machine.Name { return machine.Name(r.Name) })

	return machineNames, nil
}

type machineReboot struct {
	UUID string `db:"uuid"`
}

// RequireMachineReboot sets the machine referenced by its UUID as requiring a reboot.
//
// Reboot requests are handled through the "machine_requires_reboot" table which contains only
// machine UUID for which a reboot has been requested.
// This function is idempotent.
func (st *State) RequireMachineReboot(ctx context.Context, uuid string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	setRebootFlag := `INSERT INTO machine_requires_reboot (machine_uuid) VALUES ($machineReboot.uuid)`
	setRebootFlagStmt, err := sqlair.Prepare(setRebootFlag, machineReboot{})
	if err != nil {
		return errors.Trace(err)
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, setRebootFlagStmt, machineReboot{uuid}).Run()
	})
	if database.IsErrConstraintPrimaryKey(err) {
		// if the same uuid is added twice, do nothing (idempotency)
		return nil
	}
	return errors.Annotatef(err, "requiring reboot of machine %q", uuid)
}

// CancelMachineReboot cancels the reboot of the machine referenced by its UUID if it has
// previously been required.
//
// It basically removes the uuid from the "machine_requires_reboot" table if present.
// This function is idempotent.
func (st *State) CancelMachineReboot(ctx context.Context, uuid string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}
	unsetRebootFlag := `DELETE FROM machine_requires_reboot WHERE machine_uuid = $machineReboot.uuid`
	unsetRebootFlagStmt, err := sqlair.Prepare(unsetRebootFlag, machineReboot{})
	if err != nil {
		return errors.Trace(err)
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, unsetRebootFlagStmt, machineReboot{uuid}).Run()
	})
	return errors.Annotatef(err, "cancelling reboot of machine %q", uuid)
}

// IsMachineRebootRequired checks if the specified machine requires a reboot.
//
// It queries the "machine_requires_reboot" table for the machine UUID to determine if a reboot is required.
// Returns a boolean value indicating if a reboot is required, and an error if any occur during the process.
func (st *State) IsMachineRebootRequired(ctx context.Context, uuid string) (bool, error) {
	db, err := st.DB()
	if err != nil {
		return false, errors.Trace(err)
	}

	var isRebootRequired bool
	isRebootFlag := `SELECT machine_uuid as &machineReboot.uuid  FROM machine_requires_reboot WHERE machine_uuid = $machineReboot.uuid`
	isRebootFlagStmt, err := sqlair.Prepare(isRebootFlag, machineReboot{})
	if err != nil {
		return false, errors.Trace(err)
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var results machineReboot
		err := tx.Query(ctx, isRebootFlagStmt, machineReboot{uuid}).Get(&results)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Trace(err)
		}
		isRebootRequired = !errors.Is(err, sqlair.ErrNoRows)
		return nil
	})

	return isRebootRequired, errors.Annotatef(err, "requiring reboot of machine %q", uuid)
}
