// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	"github.com/juju/collections/transform"

	coredatabase "github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/domain"
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

// GetPaginatedTaskLogsByUUID returns a paginated slice of log messages and
// the page number.
func (st *State) GetPaginatedTaskLogsByUUID(
	ctx context.Context,
	taskUUID string,
	page int,
) ([]internal.TaskLogMessage, int, error) {
	// TODO: return log messages in order from oldest to newest.
	return nil, 0, coreerrors.NotImplemented
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
	return "", coreerrors.NotImplemented
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
		return 0, errors.Capture(err)
	}
	return count.Count, nil

}

// deleteOperationByUUIDs deletes the operations and their associated tasks.
func (st *State) deleteOperationByUUIDs(ctx context.Context, tx *sqlair.TX, toDelete []string) error {
	// Get all tasks associated with the operations to delete.
	taskUUIDs, err := st.getTaskUUIDsByOperationUUIDs(ctx, tx, toDelete)
	if err != nil {
		return errors.Errorf("getting task UUIDs: %w", err)
	}

	// Delete the tasks.
	if err := st.deleteTaskByUUIDs(ctx, tx, taskUUIDs); err != nil {
		return errors.Errorf("deleting task UUIDs: %w", err)
	}

	// Delete the operations dependencies.
	for _, table := range []string{
		"operation_action",
		"operation_parameter",
	} {
		if err := st.removeByUUIDs(ctx, tx, table, "operation_uuid", toDelete); err != nil {
			return errors.Errorf("deleting %s by operation UUIDs: %w", table, err)
		}
	}

	// Delete the operations.
	if err := st.removeByUUIDs(ctx, tx, "operation", "uuid", toDelete); err != nil {
		return errors.Errorf("deleting operation by UUIDs: %w", err)
	}

	return nil
}

// deleteTaskByUUIDs deletes the tasks and their associated dependencies by
// their UUIDs
func (st *State) deleteTaskByUUIDs(ctx context.Context, tx *sqlair.TX, toDelete []string) error {
	// Delete the tasks dependencies.
	for _, table := range []string{
		"operation_unit_task",
		"operation_machine_task",
		"operation_task_output",
		"operation_task_status",
		"operation_task_log",
	} {
		if err := st.removeByUUIDs(ctx, tx, table, "task_uuid", toDelete); err != nil {
			return errors.Errorf("deleting %s by task UUIDs: %w", table, err)
		}
	}

	// delete the tasks
	if err := st.removeByUUIDs(ctx, tx, "operation_task", "uuid", toDelete); err != nil {
		return errors.Errorf("deleting task by UUIDs: %w", err)
	}

	return nil
}

// getTaskUUIDsByOperationUUIDs returns the UUIDs of the tasks associated with
// the operations in the input list.
func (st *State) getTaskUUIDsByOperationUUIDs(ctx context.Context, tx *sqlair.TX,
	operationUUIDs []string) ([]string, error) {
	type uuids []string

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
	type uuids []string
	uuidToDelete := uuids(toDelete)
	stmt, err := st.Prepare(fmt.Sprintf(`
DELETE FROM %q
WHERE  %q IN ($uuids[:])`, table, field), uuidToDelete)
	if err != nil {
		return errors.Capture(err)
	}
	return tx.Query(ctx, stmt, uuidToDelete).Run()
}
