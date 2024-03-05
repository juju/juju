// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"

	coredb "github.com/juju/juju/core/database"
	"github.com/juju/juju/domain"
	blockdevice "github.com/juju/juju/domain/blockdevice/state"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/internal/uuid"
)

// Logger is the interface used by the state to log messages.
type Logger interface {
	Debugf(string, ...interface{})
}

// State describes retrieval and persistence methods for storage.
type State struct {
	*domain.StateBase
	logger Logger
}

// NewState returns a new state reference.
func NewState(factory coredb.TxnRunnerFactory, logger Logger) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
		logger:    logger,
	}
}

// UpsertMachine creates or updates the specified machine.
// TODO - this just creates a minimal row for now.
func (s *State) UpsertMachine(ctx context.Context, machineId string) error {
	db, err := s.DB()
	if err != nil {
		return errors.Trace(err)
	}

	machineIDParam := sqlair.M{"machine_id": machineId}
	query := `SELECT &M.uuid FROM machine WHERE machine_id = $M.machine_id`
	queryStmt, err := sqlair.Prepare(query, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}

	createMachine := `
INSERT INTO machine (uuid, net_node_uuid, machine_id, life_id)
VALUES ($M.machine_uuid, $M.net_node_uuid, $M.machine_id, $M.life_id)
`
	createMachineStmt, err := sqlair.Prepare(createMachine, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}

	createNode := `INSERT INTO net_node (uuid) VALUES ($M.net_node_uuid)`
	createNodeStmt, err := sqlair.Prepare(createNode, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		result := sqlair.M{}
		err := tx.Query(ctx, queryStmt, machineIDParam).Get(&result)
		if err != nil {
			if !errors.Is(err, sqlair.ErrNoRows) {
				return errors.Annotatef(err, "querying machine %q", machineId)
			}
		}
		// For now, we just care if the minimal machine row already exists.
		if err == nil {
			return nil
		}
		nodeUUID, err := uuid.NewUUID()
		if err != nil {
			return errors.Trace(err)
		}
		machineUUID, err := uuid.NewUUID()
		if err != nil {
			return errors.Trace(err)
		}
		createParams := sqlair.M{
			"machine_uuid":  machineUUID.String(),
			"net_node_uuid": nodeUUID.String(),
			"machine_id":    machineId,
			"life_id":       life.Alive,
		}
		if err := tx.Query(ctx, createNodeStmt, createParams).Run(); err != nil {
			return errors.Annotatef(err, "creating net node row for machine %q", machineId)
		}
		if err := tx.Query(ctx, createMachineStmt, createParams).Run(); err != nil {
			return errors.Annotatef(err, "creating machine row for machine %q", machineId)
		}
		return nil
	})
	return errors.Annotatef(err, "upserting machine %q", machineId)
}

// DeleteMachine deletes the specified machine and any dependent child records.
// TODO - this just deals with child block devices for now.
func (s *State) DeleteMachine(ctx context.Context, machineId string) error {
	db, err := s.DB()
	if err != nil {
		return errors.Trace(err)
	}

	machineIDParam := sqlair.M{"machine_id": machineId}

	queryMachine := `SELECT &M.uuid FROM machine WHERE machine_id = $M.machine_id`
	queryMachineStmt, err := sqlair.Prepare(queryMachine, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}

	deleteMachine := `DELETE FROM machine WHERE machine_id = $M.machine_id`
	deleteMachineStmt, err := sqlair.Prepare(deleteMachine, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}

	deleteNode := `
DELETE FROM net_node WHERE uuid IN
(SELECT net_node_uuid FROM machine WHERE machine_id = $M.machine_id) 
`
	deleteNodeStmt, err := sqlair.Prepare(deleteNode, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		result := sqlair.M{}
		err = tx.Query(ctx, queryMachineStmt, machineIDParam).Get(result)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Annotatef(err, "looking up UUID for machine %q", machineId)
		}
		// Machine already deleted is a no op.
		if len(result) == 0 {
			return nil
		}
		machineUUID := result["uuid"].(string)

		if err := blockdevice.RemoveMachineBlockDevices(ctx, tx, machineUUID); err != nil {
			return errors.Annotatef(err, "deleting block devices for machine %q", machineId)
		}

		if err := tx.Query(ctx, deleteMachineStmt, machineIDParam).Run(); err != nil {
			return errors.Annotatef(err, "deleting machine %q", machineId)
		}
		if err := tx.Query(ctx, deleteNodeStmt, machineIDParam).Run(); err != nil {
			return errors.Annotatef(err, "deleting net node for machine  %q", machineId)
		}

		return nil
	})
	return errors.Annotatef(err, "deleting machine %q", machineId)
}
