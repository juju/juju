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

	machineNameParam := sqlair.M{"machine_name": machineName}
	query := `SELECT &M.uuid FROM machine WHERE machine_name = $M.machine_name`
	queryStmt, err := st.Prepare(query, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}

	createMachine := `
INSERT INTO machine (uuid, net_node_uuid, machine_name, life_id)
VALUES ($M.machine_uuid, $M.net_node_uuid, $M.machine_name, $M.life_id)
`
	createMachineStmt, err := st.Prepare(createMachine, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}

	createNode := `INSERT INTO net_node (uuid) VALUES ($M.net_node_uuid)`
	createNodeStmt, err := st.Prepare(createNode, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
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

		createParams := sqlair.M{
			"machine_uuid":  machineUUID,
			"net_node_uuid": nodeUUID,
			"machine_name":  machineName,
			"life_id":       life.Alive,
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

	machineNameParam := sqlair.M{"machine_name": machineName}

	queryMachine := `SELECT &M.uuid FROM machine WHERE machine_name = $M.machine_name`
	queryMachineStmt, err := st.Prepare(queryMachine, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}

	deleteMachine := `DELETE FROM machine WHERE machine_name = $M.machine_name`
	deleteMachineStmt, err := st.Prepare(deleteMachine, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}

	deleteNode := `
DELETE FROM net_node WHERE uuid IN
(SELECT net_node_uuid FROM machine WHERE machine_name = $M.machine_name) 
`
	deleteNodeStmt, err := st.Prepare(deleteNode, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		result := sqlair.M{}
		err = tx.Query(ctx, queryMachineStmt, machineNameParam).Get(result)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Annotatef(err, "looking up UUID for machine %q", machineName)
		}
		// Machine already deleted is a no op.
		if len(result) == 0 {
			return nil
		}
		machineUUID := result["uuid"].(string)

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
	return "machine", "SELECT machine_name FROM machine"
}

// GetMachineLife returns the life status of the specified machine.
func (st *State) GetMachineLife(ctx context.Context, machineName machine.Name) (*life.Life, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	queryForLife := `SELECT life_id as &machineLife.life_id FROM machine WHERE machine_name = $M.machine_name`
	lifeStmt, err := st.Prepare(queryForLife, sqlair.M{}, machineLife{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	var lifeResult life.Life
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		result := machineLife{}
		err := tx.Query(ctx, lifeStmt, sqlair.M{"machine_name": machineName}).Get(&result)
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

	query := `SELECT machine_name AS &machineName.* FROM machine`
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
