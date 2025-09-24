// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	"github.com/juju/collections/transform"

	coredatabase "github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	coremachine "github.com/juju/juju/core/machine"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	operationerrors "github.com/juju/juju/domain/operation/errors"
	"github.com/juju/juju/domain/operation/internal"
	"github.com/juju/juju/internal/errors"
	internaluuid "github.com/juju/juju/internal/uuid"
)

// State is used to access the database.
type State struct {
	*domain.StateBase
	clock  clock.Clock
	logger logger.Logger
}

// NewState creates a state to access the database.
func NewState(factory coredatabase.TxnRunnerFactory, clock clock.Clock, logger logger.Logger) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
		clock:     clock,
		logger:    logger,
	}
}

// GetIDsForAbortingTaskOfReceiver returns a slice of task IDs for any
// task with the given receiver UUID and having a status of Aborting.
func (st *State) GetIDsForAbortingTaskOfReceiver(
	ctx context.Context,
	receiverUUID internaluuid.UUID,
) ([]string, error) {
	return nil, coreerrors.NotImplemented
}

// GetLatestTaskLogsByUUID returns a slice of log messages newer than
// the cursor provided. A new cursor is returned with the latest value.
func (st *State) GetLatestTaskLogsByUUID(
	ctx context.Context,
	taskUUID string,
	cursor time.Time,
) ([]internal.TaskLogMessage, time.Time, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, cursor, errors.Capture(err)
	}

	paging := pagination{
		Cursor: cursor,
	}
	task := taskLogEntry{TaskUUID: taskUUID}
	query := `
SELECT * AS &taskLogEntry.*
FROM   operation_task_log
WHERE  task_uuid = $taskLogEntry.task_uuid
AND    created_at > $pagination.cursor
ORDER BY created_at ASC`
	stmt, err := st.Prepare(query, paging, task)
	if err != nil {
		return nil, cursor, errors.Errorf("preparing statement: %w", err)
	}

	var results []taskLogEntry
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, task, paging).GetAll(&results)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return errors.Errorf("querying: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, cursor, errors.Errorf("getting task logs for %q: %w", taskUUID, err)
	}

	logs := transform.Slice(results, func(in taskLogEntry) internal.TaskLogMessage {
		return internal.TaskLogMessage{
			Message:   in.Content,
			Timestamp: in.CreatedAt,
		}
	})

	nextCursor := cursor
	if len(logs) > 0 {
		nextCursor = logs[len(logs)-1].Timestamp
	}

	return logs, nextCursor, nil
}

// GetTaskIDsByUUIDsFilteredByReceiverUUID returns task IDs of the tasks
// provided having the given receiverUUID.
func (st *State) GetTaskIDsByUUIDsFilteredByReceiverUUID(
	ctx context.Context,
	receiverUUID internaluuid.UUID,
	taskUUIDs []string,
) ([]string, error) {
	return nil, coreerrors.NotImplemented
}

// GetTaskUUIDByID returns the task UUID for the given task ID.
func (st *State) GetTaskUUIDByID(ctx context.Context, taskID string) (string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	task := taskIdent{ID: taskID}
	query := `
SELECT uuid AS &uuid.uuid
FROM   operation_task
WHERE  task_id = $taskIdent.task_id`
	stmt, err := st.Prepare(query, uuid{}, task)
	if err != nil {
		return "", errors.Errorf("preparing statement: %w", err)
	}

	var result uuid
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, task).Get(&result)
		if errors.Is(err, sql.ErrNoRows) {
			return operationerrors.TaskNotFound
		} else if err != nil {
			return errors.Errorf("querying task: %w", err)
		}
		return nil
	})
	if err != nil {
		return "", errors.Errorf("getting task UUID for %q: %w", taskID, err)
	}
	return result.UUID, nil
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

// count returns the number of records in the specified table.
func (st *State) count(ctx context.Context, tx *sqlair.TX, table string) (int, error) {
	type result struct {
		Count int `db:"count"`
	}
	stmt, err := st.Prepare(fmt.Sprintf(`
SELECT COUNT(*) as &result.count
FROM   %q`, table),
		result{})
	if err != nil {
		return 0, errors.Capture(err)
	}

	var count result
	if err := tx.Query(ctx, stmt).Get(&count); err != nil {
		return 0, errors.Errorf("counting element from table %q", err)
	}
	return count.Count, nil

}

// deleteOperationByUUIDs deletes the operations and their associated tasks.
// It returns the storeUUIDs that used to be linked with the deleted tasks
func (st *State) deleteOperationByUUIDs(ctx context.Context, tx *sqlair.TX, toDelete []string) ([]string, error) {
	// Get all tasks associated with the operations to delete.
	taskUUIDs, err := st.getTaskUUIDsByOperationUUIDs(ctx, tx, toDelete)
	if err != nil {
		return nil, errors.Errorf("getting task UUIDs: %w", err)
	}

	// Delete the tasks.
	storePaths, err := st.deleteTaskByUUIDs(ctx, tx, taskUUIDs)
	if err != nil {
		return nil, errors.Errorf("deleting task UUIDs: %w", err)
	}

	// Delete the operations dependencies.
	for _, table := range []string{
		"operation_action",
		"operation_parameter",
	} {
		if err := st.removeByUUIDs(ctx, tx, table, "operation_uuid", toDelete); err != nil {
			return nil, errors.Errorf("deleting %s by operation UUIDs: %w", table, err)
		}
	}

	// Delete the operations.
	if err := st.removeByUUIDs(ctx, tx, "operation", "uuid", toDelete); err != nil {
		return nil, errors.Errorf("deleting operation by UUIDs: %w", err)
	}

	return storePaths, nil
}

// deleteTaskByUUIDs deletes the tasks and their associated dependencies by
// their UUIDs.
// It returns the storeUUIDs of operation_task_output to allow cleanup
// of the object datastore in the service layer.
func (st *State) deleteTaskByUUIDs(ctx context.Context, tx *sqlair.TX, toDelete []string) ([]string, error) {
	// get store paths
	type store path
	tasks := uuids(toDelete)
	stmt, err := st.Prepare(`
SELECT &store.path
FROM object_store_metadata_path AS osp
JOIN operation_task_output AS oto ON osp.metadata_uuid = oto.store_uuid
WHERE task_uuid IN ($uuids[:])`, tasks, store{})
	if err != nil {
		return nil, errors.Capture(err)
	}
	var stores []store
	if err := tx.Query(ctx, stmt, tasks).GetAll(&stores); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Errorf("getting task store uuids: %w", err)
	}

	// Delete the tasks dependencies.
	for _, table := range []string{
		"operation_unit_task",
		"operation_machine_task",
		"operation_task_output",
		"operation_task_status",
		"operation_task_log",
	} {
		if err := st.removeByUUIDs(ctx, tx, table, "task_uuid", toDelete); err != nil {
			return nil, errors.Errorf("deleting %s by task UUIDs: %w", table, err)
		}
	}

	// delete the tasks
	if err := st.removeByUUIDs(ctx, tx, "operation_task", "uuid", toDelete); err != nil {
		return nil, errors.Errorf("deleting task by UUIDs: %w", err)
	}

	return transform.Slice(stores, func(v store) string {
		return v.Path
	}), nil
}

// getTaskUUIDsByOperationUUIDs returns the UUIDs of the tasks associated with
// the operations in the input list.
func (st *State) getTaskUUIDsByOperationUUIDs(ctx context.Context, tx *sqlair.TX,
	operationUUIDs []string) ([]string, error) {
	type task uuid

	toGet := uuids(operationUUIDs)

	stmt, err := st.Prepare(`
SELECT &task.uuid
FROM   operation_task
WHERE  operation_uuid IN ($uuids[:])`, toGet, task{})
	if err != nil {
		return nil, errors.Capture(err)
	}
	var taskUUIDs []task
	if err := tx.Query(ctx, stmt, toGet).GetAll(&taskUUIDs); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Capture(err)
	}

	return transform.Slice(taskUUIDs, func(t task) string { return t.UUID }), nil
}

// removeByUUIDs removes the records from the specified table where the
// specified field matches the input list of UUIDs.
func (st *State) removeByUUIDs(ctx context.Context, tx *sqlair.TX, table, field string,
	toDelete []string) error {
	uuidToDelete := uuids(toDelete)
	stmt, err := st.Prepare(fmt.Sprintf(`
DELETE FROM %q
WHERE  %q IN ($uuids[:])`, table, field), uuidToDelete)
	if err != nil {
		return errors.Capture(err)
	}
	return tx.Query(ctx, stmt, uuidToDelete).Run()
}

// GetUnitUUIDByName returns the unit UUID for the given unit name.
func (st *State) GetUnitUUIDByName(ctx context.Context, unitName coreunit.Name) (string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	unitIdent := nameArg{Name: unitName.String()}
	query := `
SELECT uuid AS &uuid.uuid 
FROM   unit 
WHERE  name = $nameArg.name`
	stmt, err := st.Prepare(query, uuid{}, unitIdent)
	if err != nil {
		return "", errors.Errorf("preparing statement: %w", err)
	}

	var result uuid
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, unitIdent).Get(&result)
		if errors.Is(err, sql.ErrNoRows) {
			return errors.Errorf("unit %q not found", unitName.String()).Add(applicationerrors.UnitNotFound)
		} else if err != nil {
			return errors.Errorf("querying unit: %w", err)
		}

		return nil
	})
	if err != nil {
		return "", errors.Errorf("getting unit UUID for %q: %w", unitName.String(), err)
	}
	return result.UUID, nil
}

// GetMachineUUIDByName returns the machine UUID for the given machine name.
func (st *State) GetMachineUUIDByName(ctx context.Context, machineName coremachine.Name) (string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	machineIdent := nameArg{Name: machineName.String()}
	query := `
SELECT uuid AS &uuid.uuid 
FROM   machine 
WHERE  name = $nameArg.name`
	stmt, err := st.Prepare(query, uuid{}, machineIdent)
	if err != nil {
		return "", errors.Errorf("preparing statement: %w", err)
	}

	var result uuid
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, machineIdent).Get(&result)
		if errors.Is(err, sql.ErrNoRows) {
			return errors.Errorf("machine %q not found", machineName.String()).Add(machineerrors.MachineNotFound)
		} else if err != nil {
			return errors.Errorf("querying machine: %w", err)
		}

		return nil
	})
	if err != nil {
		return "", errors.Errorf("getting machine UUID for %q: %w", machineName.String(), err)
	}
	return result.UUID, nil
}

// InitialWatchStatementUnitTask returns the namespace and an initial query
// function which returns the list of (only PENDING or ABORTING status) task ids
// for the given unit.
func (st *State) InitialWatchStatementUnitTask() (string, string) {
	return "custom_operation_task_status_pending_or_aborting", `
SELECT t.task_id
FROM   operation_task AS t
JOIN   operation_unit_task AS ut ON t.uuid = ut.task_uuid
JOIN   operation_task_status AS ts ON t.uuid = ts.task_uuid
JOIN   operation_task_status_value AS sv ON ts.status_id = sv.id
WHERE  ut.unit_uuid = ?
AND    (
       sv.status = 'pending'
       OR
       sv.status = 'aborting'
)`
}

// InitialWatchStatementMachineTask returns the namespace and an initial
// query function which returns the list of (only PENDING status) task ids for
// the given machine.
func (st *State) InitialWatchStatementMachineTask() (string, string) {
	return "custom_operation_task_status_pending", `
SELECT t.task_id
FROM   operation_task AS t
JOIN   operation_machine_task AS mt ON t.uuid = mt.task_uuid
JOIN   operation_task_status AS ts ON t.uuid = ts.task_uuid
JOIN   operation_task_status_value AS sv ON ts.status_id = sv.id
WHERE  mt.machine_uuid = ?
AND    sv.status = 'pending'`
}

// FiltertTaskUUIDsForUnit returns a list of task IDs that corresponds to the
// filtered list of task UUIDs from the provided list that target the given
// unit uuid.
// NOTE: This function does not perform any check on the status of the tasks.
// Although the status check is needed for the notification watcher, we already
// have a custom changelog trigger, which only fires on PENDING and ABORTING
// statuses, which makes this filter only needed for unit UUID filtering.
func (st *State) FilterTaskUUIDsForUnit(ctx context.Context, tUUIDs []string, unitUUID string) ([]string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	type taskUUIDs []string
	taskInputUUIDs := taskUUIDs(tUUIDs)
	unitIdent := uuid{UUID: unitUUID}
	query := `
SELECT t.task_id AS &taskIdent.task_id
FROM   operation_task AS t
JOIN   operation_unit_task AS ut ON t.uuid = ut.task_uuid
WHERE  t.uuid IN ($taskUUIDs[:])
AND    ut.unit_uuid = $uuid.uuid`
	stmt, err := st.Prepare(query, taskIdent{}, taskInputUUIDs, unitIdent)
	if err != nil {
		return nil, errors.Errorf("preparing statement: %w", err)
	}

	var results []taskIdent
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, taskInputUUIDs, unitIdent).GetAll(&results)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return errors.Errorf("querying unit task ids: %w", err)
		}

		return nil
	})
	if err != nil {
		return nil, errors.Errorf("getting task ids for unit %q: %w", unitUUID, err)
	}

	ids := transform.Slice(results, func(res taskIdent) string {
		return res.ID
	})
	return ids, nil
}

// FilterTaskUUIDsForMachine returns a list of task IDs that corresponds to the
// filtered list of task UUIDs from the provided list that target the given
// machine uuid.
// NOTE: This function does not perform any check on the status of the tasks.
// Although the status check is needed for the notification watcher, we already
// have a custom changelog trigger, which only fires on PENDING status, which
// makes this filter only needed for machine UUID filtering.
func (st *State) FilterTaskUUIDsForMachine(ctx context.Context, tUUIDs []string, machineUUID string) ([]string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
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
	stmt, err := st.Prepare(query, taskIdent{}, taskInputUUIDs, machineIdent)
	if err != nil {
		return nil, errors.Errorf("preparing statement: %w", err)
	}

	var results []taskIdent
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, taskInputUUIDs, machineIdent).GetAll(&results)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return errors.Errorf("querying task ids: %w", err)
		}

		return nil
	})
	if err != nil {
		return nil, errors.Errorf("getting task ids for machine %q: %w", machineUUID, err)
	}

	ids := transform.Slice(results, func(res taskIdent) string {
		return res.ID
	})
	return ids, nil
}

// deleteStoreEntryByUUIDs deletes all entries from the object store metadata
// tables that match the provided UUIDs and returns the list of paths that were
// associated with those UUIDs.
func (st *State) deleteStoreEntryByUUIDs(ctx context.Context, toDeleteUUIDs []string) ([]string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	toDelete := uuids(toDeleteUUIDs)

	selectPathsStmt, err := st.Prepare(`
SELECT &path.path
FROM   object_store_metadata_path
WHERE  metadata_uuid IN ($uuids[:])`, path{}, toDelete)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var paths []path
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Read all paths first so we can return them after deletion.
		if err := tx.Query(ctx, selectPathsStmt, toDelete).GetAll(&paths); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("getting paths for store UUIDs: %w", err)
		}

		if err := st.removeByUUIDs(ctx, tx, "object_store_metadata_path", "metadata_uuid", toDeleteUUIDs); err != nil {
			return errors.Errorf("deleting object_store_metadata_path by UUIDs: %w", err)
		}
		if err := st.removeByUUIDs(ctx, tx, "object_store_metadata", "uuid", toDeleteUUIDs); err != nil {
			return errors.Errorf("deleting object_store_metadata by UUIDs: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}
	return transform.Slice(paths, func(r path) string { return r.Path }), nil
}
