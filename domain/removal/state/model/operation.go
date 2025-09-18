// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"

	"github.com/juju/juju/internal/errors"
)

// cleanupTasksAndOperationsByUnitUUID deletes the tasks which belong to the unit.
// It eventually removes the operation where the tasks belong if they came empty.
// It returns the paths of operation_task_output to allow cleanup
// of the object datastore in the service layer.
func (st *State) cleanupTasksAndOperationsByUnitUUID(ctx context.Context, tx *sqlair.TX, unitUUID string) ([]string, error) {
	// Get all tasks and operation associated with the unit
	taskUUIDs, err := st.getTaskUUIDsByUnitUUID(ctx, tx, unitUUID)
	if err != nil {
		return nil, errors.Errorf("getting task UUIDs: %w", err)
	}
	opUUIDs, err := st.getOperationByTaskUUIDs(ctx, tx, taskUUIDs)
	if err != nil {
		return nil, errors.Errorf("getting operation UUIDs: %w", err)
	}
	return st.deleteTaskAndCleanupOperations(ctx, tx, opUUIDs, taskUUIDs)
}

// cleanupTasksAndOperationsByMachineUUID deletes the tasks which belong to the machine.
// It eventually removes the operation where the tasks belong if they came empty.
// It returns the paths of operation_task_output to allow cleanup
// of the object datastore in the service layer.
func (st *State) cleanupTasksAndOperationsByMachineUUID(ctx context.Context, tx *sqlair.TX, machineUUID string) ([]string, error) {
	taskUUIDs, err := st.getTaskUUIDsByMachineUUID(ctx, tx, machineUUID)
	if err != nil {
		return nil, errors.Errorf("getting task UUIDs: %w", err)
	}
	opUUIDs, err := st.getOperationByTaskUUIDs(ctx, tx, taskUUIDs)
	if err != nil {
		return nil, errors.Errorf("getting operation UUIDs: %w", err)
	}
	return st.deleteTaskAndCleanupOperations(ctx, tx, opUUIDs, taskUUIDs)
}

// deleteTaskAndCleanupOperations removes tasks and operations that are empty,
// by uuids.
// Tasks are deleted; then operations are deleted if they have no more tasks.
// Returns a list of paths to the deleted task outputs so they can
// be removed from the object store later.
func (st *State) deleteTaskAndCleanupOperations(ctx context.Context, tx *sqlair.TX, operationUUIDs,
	taskUUIDs []string) ([]string, error) {

	// Delete the tasks.
	storePaths, err := st.deleteTaskByUUIDs(ctx, tx, taskUUIDs)
	if err != nil {
		return nil, errors.Errorf("deleting task UUIDs: %w", err)
	}

	// Filter operations without tasks
	filteredOperationUUIDs, err := st.filterEmptyOperationUUIDs(ctx, tx, operationUUIDs)
	if err != nil {
		return nil, errors.Errorf("filtering empty operations: %w", err)
	}

	// Delete empty operations
	err = st.deleteOperationByUUIDs(ctx, tx, filteredOperationUUIDs)
	if err != nil {
		return nil, errors.Errorf("deleting operation UUIDs: %w", err)
	}

	return storePaths, nil
}

// getTaskUUIDsByUnitUUID returns the UUIDs of tasks which belong to the unit.
func (st *State) getTaskUUIDsByUnitUUID(ctx context.Context, tx *sqlair.TX, unitUUID string) ([]string, error) {
	type task entityUUID
	type unit entityUUID

	stmt, err := st.Prepare(`
SELECT task_uuid AS &task.uuid
FROM   operation_unit_task
WHERE  unit_uuid = $unit.uuid;`, task{}, unit{})

	if err != nil {
		return nil, errors.Capture(err)
	}

	var result []task
	if err := tx.Query(ctx, stmt, unit{UUID: unitUUID}).GetAll(&result); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Errorf("running task UUIDs query: %w", err)
	}

	return transform.Slice(result, func(t task) string {
		return t.UUID
	}), nil
}

// getTaskUUIDsByMachineUUID returns the UUIDs of tasks which belong to the machine.
func (st *State) getTaskUUIDsByMachineUUID(ctx context.Context, tx *sqlair.TX, machineUUID string) ([]string,
	error) {
	type task entityUUID
	type machine entityUUID

	stmt, err := st.Prepare(`
SELECT task_uuid AS &task.uuid
FROM   operation_machine_task
WHERE  machine_uuid = $machine.uuid;`, task{}, machine{})

	if err != nil {
		return nil, errors.Capture(err)
	}

	var result []task
	if err := tx.Query(ctx, stmt, machine{UUID: machineUUID}).GetAll(&result); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Errorf("running task UUIDs query: %w", err)
	}

	return transform.Slice(result, func(t task) string {
		return t.UUID
	}), nil
}

// getOperationByTaskUUIDs returns the UUIDs of operations that contains the tasks.
func (st *State) getOperationByTaskUUIDs(ctx context.Context, tx *sqlair.TX, taskUUIDs []string) ([]string, error) {
	if len(taskUUIDs) == 0 {
		return nil, nil
	}

	type operation entityUUID
	tasks := uuids(taskUUIDs)

	stmt, err := st.Prepare(`
SELECT DISTINCT operation_uuid AS &operation.uuid
FROM   operation_task
WHERE  uuid IN ($uuids[:])`, operation{}, tasks)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var result []operation
	if err := tx.Query(ctx, stmt, tasks).GetAll(&result); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Errorf("running operation UUIDs query: %w", err)
	}

	return transform.Slice(result, func(op operation) string {
		return op.UUID
	}), nil
}

// filterEmptyOperationUUIDs filters the provided list of operation UUIDs and
// only returns the ones that don't have any task associated to it.
func (st *State) filterEmptyOperationUUIDs(ctx context.Context, tx *sqlair.TX, operationUUIDs []string) ([]string, error) {
	if len(operationUUIDs) == 0 {
		return nil, nil
	}
	type operation entityUUID

	stmt, err := st.Prepare(`
SELECT DISTINCT o.uuid AS &operation.uuid
FROM operation AS o
LEFT JOIN operation_task AS ot ON o.uuid = ot.operation_uuid 
WHERE o.uuid IN ($uuids[:])
AND ot.uuid IS NULL`, operation{}, uuids{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var result []operation
	if err := tx.Query(ctx, stmt, uuids(operationUUIDs)).GetAll(&result); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Errorf("running get empty operation query: %w", err)
	}

	return transform.Slice(result, func(op operation) string {
		return op.UUID
	}), nil
}

// deleteTaskByUUIDs deletes the tasks and their associated dependencies by
// their UUIDs.
// It returns the path of object_store_metadata_path to allow cleanup
// of the object datastore in the service layer.
func (st *State) deleteTaskByUUIDs(ctx context.Context, tx *sqlair.TX, toDelete []string) ([]string, error) {
	if len(toDelete) == 0 {
		return nil, nil
	}

	// get store paths
	stores, err := st.getObjectStorePathByTaskUUIDs(ctx, tx, toDelete)
	if err != nil {
		return nil, errors.Errorf("getting task store uuids: %w", err)
	}

	// Delete tasks and rows that reference them in other tables
	tasks := uuids(toDelete)
	for _, query := range []string{
		`DELETE FROM operation_unit_task WHERE task_uuid IN ($uuids[:])`,
		`DELETE FROM operation_machine_task WHERE task_uuid IN ($uuids[:])`,
		`DELETE FROM operation_task_output WHERE task_uuid IN ($uuids[:])`,
		`DELETE FROM operation_task_status WHERE task_uuid IN ($uuids[:])`,
		`DELETE FROM operation_task_log WHERE task_uuid IN ($uuids[:])`,
		`DELETE FROM operation_task WHERE uuid IN ($uuids[:])`,
	} {
		stmt, err := st.Prepare(query, tasks)
		if err != nil {
			return nil, errors.Capture(err)
		}
		if err := tx.Query(ctx, stmt, tasks).Run(); err != nil {
			return nil, errors.Errorf("deleting reference to operation_task in table %q: %w", query, err)
		}
	}

	return stores, nil
}

// getObjectStorePathByTaskUUIDs retrieves object store paths associated with
// the given task UUIDs from the database.
// It returns a slice of paths or an error if the operation fails.
func (st *State) getObjectStorePathByTaskUUIDs(
	ctx context.Context,
	tx *sqlair.TX,
	taskUUIDs []string,
) ([]string, error) {
	type store struct {
		Path string `db:"path"`
	}
	tasks := uuids(taskUUIDs)
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
		return nil, errors.Capture(err)
	}
	return transform.Slice(stores, func(v store) string {
		return v.Path
	}), nil
}

// deleteOperationByUUIDs deletes the operations and their associated dependencies by
// their UUIDs.
func (st *State) deleteOperationByUUIDs(ctx context.Context, tx *sqlair.TX, toDelete []string) error {

	operations := uuids(toDelete)

	// Delete operations and rows that reference them in other tables
	for _, query := range []string{
		`DELETE FROM operation_action WHERE operation_uuid IN ($uuids[:])`,
		`DELETE FROM operation_parameter WHERE operation_uuid IN ($uuids[:])`,
		`DELETE FROM operation WHERE uuid IN ($uuids[:])`,
	} {
		stmt, err := st.Prepare(query, operations)
		if err != nil {
			return errors.Capture(err)
		}
		if err := tx.Query(ctx, stmt, operations).Run(); err != nil {
			return errors.Errorf("deleting reference to operation in table %q: %w", query, err)
		}
	}
	return nil
}
