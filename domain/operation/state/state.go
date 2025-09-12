// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	coremachine "github.com/juju/juju/core/machine"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/domain/operation/internal"
	internalerrors "github.com/juju/juju/internal/errors"
	internaluuid "github.com/juju/juju/internal/uuid"
)

// State is used to access the database.
type State struct {
	*domain.StateBase
	logger logger.Logger
}

// NewState creates a state to access the database.
func NewState(factory coredatabase.TxnRunnerFactory, logger logger.Logger) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
		logger:    logger,
	}
}

// GetIDsForAbortingTaskOfReceiver returns a slice of task IDs for any
// task with the given receiver UUID and having a status of Aborting.
func (st *State) GetIDsForAbortingTaskOfReceiver(
	ctx context.Context,
	receiverUUID internaluuid.UUID,
) ([]string, error) {
	return nil, errors.NotImplemented
}

// GetPaginatedTaskLogsByUUID returns a paginated slice of log messages and
// the page number.
func (st *State) GetPaginatedTaskLogsByUUID(
	ctx context.Context,
	taskUUID string,
	page int,
) ([]internal.TaskLogMessage, int, error) {
	// TODO: return log messages in order from oldest to newest.
	return nil, 0, errors.NotImplemented
}

// GetTaskIDsByUUIDsFilteredByReceiverUUID returns task IDs of the tasks
// provided having the given receiverUUID.
func (st *State) GetTaskIDsByUUIDsFilteredByReceiverUUID(
	ctx context.Context,
	receiverUUID internaluuid.UUID,
	taskUUIDs []string,
) ([]string, error) {
	return nil, errors.NotImplemented
}

// GetTaskUUIDByID returns the task UUID for the given task ID.
func (st *State) GetTaskUUIDByID(ctx context.Context, taskID string) (string, error) {
	return "", errors.NotImplemented
}

// NamespaceForTaskAbortingWatcher returns the namespace for
// the TaskAbortingWatcher. This custom namespace only notifies
// if an operation task's status is set to ABORTING.
func (st *State) NamespaceForTaskAbortingWatcher() string {
	return "custom_operation_task_status_aborting"
}

// NamespaceForTaskLogWatcher returns the name space for watching task
// log messages.
func (st *State) NamespaceForTaskLogWatcher() string {
	return "operation_task_log"
}

// GetUnitUUIDByName returns the unit UUID for the given unit name.
func (s *State) GetUnitUUIDByName(ctx context.Context, unitName coreunit.Name) (string, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return "", internalerrors.Capture(err)
	}

	unitIdent := nameArg{Name: unitName.String()}
	query := `
SELECT uuid AS &uuid.uuid 
FROM   unit 
WHERE  name = $nameArg.name`
	stmt, err := s.Prepare(query, uuid{}, unitIdent)
	if err != nil {
		return "", internalerrors.Errorf("preparing statement: %w", err)
	}

	var result uuid
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, unitIdent).Get(&result)
		if internalerrors.Is(err, sql.ErrNoRows) {
			return internalerrors.Errorf("unit %q not found", unitName.String()).Add(applicationerrors.UnitNotFound)
		} else if err != nil {
			return internalerrors.Errorf("querying unit: %w", err)
		}

		return nil
	})
	if err != nil {
		return "", internalerrors.Errorf("getting unit UUID for %q: %w", unitName.String(), err)
	}
	return result.UUID, nil
}

// GetMachineUUIDByName returns the machine UUID for the given machine name.
func (s *State) GetMachineUUIDByName(ctx context.Context, machineName coremachine.Name) (string, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return "", internalerrors.Capture(err)
	}

	machineIdent := nameArg{Name: machineName.String()}
	query := `
SELECT uuid AS &uuid.uuid 
FROM   machine 
WHERE  name = $nameArg.name`
	stmt, err := s.Prepare(query, uuid{}, machineIdent)
	if err != nil {
		return "", internalerrors.Errorf("preparing statement: %w", err)
	}

	var result uuid
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, machineIdent).Get(&result)
		if internalerrors.Is(err, sql.ErrNoRows) {
			return internalerrors.Errorf("machine %q not found", machineName.String()).Add(machineerrors.MachineNotFound)
		} else if err != nil {
			return internalerrors.Errorf("querying machine: %w", err)
		}

		return nil
	})
	if err != nil {
		return "", internalerrors.Errorf("getting machine UUID for %q: %w", machineName.String(), err)
	}
	return result.UUID, nil
}

// InitialWatchStatementUnitTask returns the namespace (table) and an
// initial query function which returns the list of non-pending task ids for
// the given unit.
func (s *State) InitialWatchStatementUnitTask() (string, string) {
	return "operation_task_status", `
SELECT t.task_id
FROM   operation_task AS t
JOIN   operation_unit_task AS ut ON t.uuid = ut.task_uuid
JOIN   operation_task_status AS ts ON t.uuid = ts.task_uuid
JOIN   operation_task_status_value AS sv ON ts.status_id = sv.id
WHERE  ut.unit_uuid = ?
AND    sv.status != 'pending'`
}

// InitialWatchStatementMachineTask returns the namespace and an initial
// query function which returns the list of task ids for the given machine.
func (s *State) InitialWatchStatementMachineTask() (string, string) {
	return "operation_task_status", `
SELECT t.task_id
FROM   operation_task AS t
JOIN   operation_machine_task AS mt ON t.uuid = mt.task_uuid
WHERE  mt.machine_uuid = ?`
}

// FiltertTaskUUIDsForUnit returns a list of task IDs that corresponds to the
// filtered list of task UUIDs from the provided list that target the given
// unit uuid and are not in pending status.
func (s *State) FilterTaskUUIDsForUnit(ctx context.Context, tUUIDs []string, unitUUID string) ([]string, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return nil, internalerrors.Capture(err)
	}

	type taskUUIDs []string
	taskInputUUIDs := taskUUIDs(tUUIDs)
	unitIdent := uuid{UUID: unitUUID}
	query := `
SELECT t.task_id AS &taskIdent.task_id
FROM   operation_task AS t
JOIN   operation_unit_task AS ut ON t.uuid = ut.task_uuid
JOIN   operation_task_status AS ts ON t.uuid = ts.task_uuid
JOIN   operation_task_status_value AS sv ON ts.status_id = sv.id
WHERE  t.uuid IN ($taskUUIDs[:])
AND    ut.unit_uuid = $uuid.uuid 
AND    sv.status != 'pending'`
	stmt, err := s.Prepare(query, taskIdent{}, taskInputUUIDs, unitIdent)
	if err != nil {
		return nil, internalerrors.Errorf("preparing statement: %w", err)
	}

	var results []taskIdent
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, taskInputUUIDs, unitIdent).GetAll(&results)
		if err != nil && !internalerrors.Is(err, sql.ErrNoRows) {
			return internalerrors.Errorf("querying unit task ids: %w", err)
		}

		return nil
	})
	if err != nil {
		return nil, internalerrors.Errorf("getting task ids for unit %q: %w", unitUUID, err)
	}

	ids := make([]string, len(results))
	for i, result := range results {
		ids[i] = result.ID
	}
	return ids, nil
}

// FilterTaskUUIDsForMachine returns a list of task IDs that corresponds to the
// filtered list of task UUIDs from the provided list that target the given
// machine uuid, including the ones in pending status.
func (s *State) FilterTaskUUIDsForMachine(ctx context.Context, tUUIDs []string, machineUUID string) ([]string, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return nil, internalerrors.Capture(err)
	}

	type taskUUIDs []string
	taskInputUUIDs := taskUUIDs(tUUIDs)
	machineIdent := uuid{UUID: machineUUID}
	query := `
SELECT t.task_id AS &taskIdent.task_id
FROM   operation_task AS t
JOIN   operation_machine_task AS mt ON t.uuid = mt.task_uuid
WHERE  t.uuid IN ($taskUUIDs[:])
AND    mt.machine_uuid = $uuid.uuid`
	stmt, err := s.Prepare(query, taskIdent{}, taskInputUUIDs, machineIdent)
	if err != nil {
		return nil, internalerrors.Errorf("preparing statement: %w", err)
	}

	var results []taskIdent
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, taskInputUUIDs, machineIdent).GetAll(&results)
		if err != nil && !internalerrors.Is(err, sql.ErrNoRows) {
			return internalerrors.Errorf("querying task ids: %w", err)
		}

		return nil
	})
	if err != nil {
		return nil, internalerrors.Errorf("getting task ids for machine %q: %w", machineUUID, err)
	}

	ids := make([]string, len(results))
	for i, result := range results {
		ids[i] = result.ID
	}
	return ids, nil
}
