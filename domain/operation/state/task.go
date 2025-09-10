// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"time"

	"github.com/canonical/sqlair"

	"github.com/juju/collections/transform"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/domain/operation"
	operationerrors "github.com/juju/juju/domain/operation/errors"
	"github.com/juju/juju/internal/errors"
)

// GetTask returns the task identified by its ID.
// It returns the task as well as the path to its output in the object store,
// if any. It's up to the caller to retrieve the actual output from the object
// store.
//
// The following errors may be returned:
// - [operationerrors.TaskNotFound] when the task does not exists.
func (s *State) GetTask(ctx context.Context, taskID string) (operation.TaskInfo, *string, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return operation.TaskInfo{}, nil, errors.Capture(err)
	}

	var (
		result     operation.TaskInfo
		outputPath *string
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error

		result, outputPath, err = s.getTask(ctx, tx, taskID)
		return errors.Capture(err)
	})
	if err != nil {
		return operation.TaskInfo{}, nil, errors.Errorf("getting task %q: %w", taskID, err)
	}

	return result, outputPath, nil
}

// CancelTask attempts to cancel an enqueued task, identified by its
// ID.
//
// The following errors may be returned:
// - [operationerrors.TaskNotFound] when the task does not exists.
func (s *State) CancelTask(ctx context.Context, taskID string) (operation.TaskInfo, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return operation.TaskInfo{}, errors.Capture(err)
	}

	var result operation.TaskInfo
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Attempt to cancel the task.
		err = s.cancelTask(ctx, tx, taskID)
		if err != nil {
			return errors.Capture(err)
		}

		result, _, err = s.getTask(ctx, tx, taskID)
		return errors.Capture(err)
	})
	if err != nil {
		return operation.TaskInfo{}, errors.Errorf("cancelling task %q: %w", taskID, err)
	}

	return result, nil
}

// cancelTask updates a specific task to cancelled status.
func (s *State) cancelTask(ctx context.Context, tx *sqlair.TX, taskID string) error {
	taskIDParam := taskIdent{ID: taskID}

	currentStatusQuery := `
SELECT otsv.status AS &taskStatus.status
FROM   operation_task AS ot
JOIN   operation_task_status AS ots ON ots.task_uuid = ot.uuid
JOIN   operation_task_status_value AS otsv ON ots.status_id = otsv.id
WHERE  ot.task_id = $taskIdent.task_id
`
	currentStatusStmt, err := s.Prepare(currentStatusQuery, taskStatus{}, taskIDParam)

	var currentStatus taskStatus
	err = tx.Query(ctx, currentStatusStmt, taskIDParam).Get(&currentStatus)
	if errors.Is(err, sql.ErrNoRows) {
		return errors.Errorf("task with ID %q not found", taskID).Add(operationerrors.TaskNotFound)
	} else if err != nil {
		return errors.Errorf("querying current status for task ID %q: %w", taskID, err)
	}

	// This is the update query, which will update the status to Cancelled or Aborting.
	updateStatusQuery := `
UPDATE operation_task_status
SET    status_id = $taskStatus.status_id,
       updated_at = $taskStatus.updated_at
FROM   operation_task AS ot
WHERE  operation_task_status.task_uuid = ot.uuid
AND    ot.task_id = $taskIdent.task_id
`
	updateStatusStmt, err := s.Prepare(updateStatusQuery, taskStatus{}, taskIDParam)
	if err != nil {
		return errors.Errorf("preparing update status statement for task ID %q: %w", taskID, err)
	}

	// If the status is Completed, Cancelled, Failed, Aborted or Error then
	// there's nothing to do.
	if currentStatus.Status == corestatus.Completed.String() ||
		currentStatus.Status == corestatus.Cancelled.String() ||
		currentStatus.Status == corestatus.Failed.String() ||
		currentStatus.Status == corestatus.Aborted.String() ||
		currentStatus.Status == corestatus.Error.String() {

		return nil
	}

	newStatus := taskStatus{
		UpdatedAt: time.Now().UTC(),
	}
	if currentStatus.Status == corestatus.Pending.String() {
		// If the task is in Pending status, then we have to update its status to
		// Cancelled.
		newStatus.Status = corestatus.Cancelled.String()
	} else if currentStatus.Status == corestatus.Running.String() {
		// If the task is already in Running status, then we have to update its
		// status to Aborting.
		newStatus.Status = corestatus.Cancelled.String()
	}
	err = tx.Query(ctx, updateStatusStmt, newStatus, taskIDParam).Run()
	if err != nil {
		return errors.Errorf("updating status for cancelled task ID %q: %w", taskID, err)
	}

	return nil
}

func (s *State) getTask(ctx context.Context, tx *sqlair.TX, taskID string) (operation.TaskInfo, *string, error) {
	result, err := s.getOperationTask(ctx, tx, taskID)
	if err != nil {
		return operation.TaskInfo{}, nil, errors.Capture(err)
	}

	parameters, err := s.getOperationParameters(ctx, tx, result.OperationUUID)
	if err != nil {
		return operation.TaskInfo{}, nil, errors.Capture(err)
	}

	logEntries, err := s.getTaskLog(ctx, tx, taskID)
	if err != nil {
		return operation.TaskInfo{}, nil, errors.Capture(err)
	}

	task, err := encodeTask(result, parameters, logEntries)
	if err != nil {
		return operation.TaskInfo{}, nil, errors.Capture(err)
	}

	var outputPath *string
	if result.OutputPath.Valid {
		outputPath = &result.OutputPath.String
	}

	return task, outputPath, nil
}

func (s *State) getOperationTask(ctx context.Context, tx *sqlair.TX, taskID string) (taskResult, error) {
	ident := taskIdent{ID: taskID}

	query := `
SELECT o.uuid AS &taskResult.operation_uuid,
       COALESCE(u.name, m.name, "") AS &taskResult.receiver,
       oa.charm_action_key AS &taskResult.name,
       o.summary AS &taskResult.summary,
       o.parallel AS &taskResult.parallel,
       o.execution_group AS &taskResult.execution_group,
       o.enqueued_at AS &taskResult.enqueued_at,
       ot.started_at AS &taskResult.started_at,
       ot.completed_at AS &taskResult.completed_at,
       osv.status AS &taskResult.status,
       os.path AS &taskResult.path
FROM operation_task AS ot
JOIN operation AS o ON ot.operation_uuid = o.uuid
JOIN operation_task_status AS ots ON ot.uuid = ots.task_uuid
JOIN operation_task_status_value AS osv ON ots.status_id = osv.id
LEFT JOIN operation_action AS oa ON o.uuid = oa.operation_uuid
LEFT JOIN operation_unit_task AS out ON ot.uuid = out.task_uuid
LEFT JOIN unit AS u ON out.unit_uuid = u.uuid
LEFT JOIN operation_machine_task AS omt ON ot.uuid = omt.task_uuid
LEFT JOIN machine AS m ON omt.machine_uuid = m.uuid
LEFT JOIN operation_task_output AS oto ON ot.uuid = oto.task_uuid
LEFT JOIN v_object_store_metadata AS os ON oto.store_uuid = os.uuid
WHERE ot.task_id = $taskIdent.task_id
`
	stmt, err := s.Prepare(query, taskResult{}, ident)
	if err != nil {
		return taskResult{}, errors.Errorf("preparing get task statement: %w", err)
	}

	var result taskResult
	err = tx.Query(ctx, stmt, ident).Get(&result)
	if errors.Is(err, sql.ErrNoRows) {
		return taskResult{}, errors.Errorf("task with ID %q not found", taskID).Add(operationerrors.TaskNotFound)
	} else if err != nil {
		return taskResult{}, errors.Errorf("querying task with ID %q: %w", taskID, err)
	}

	return result, nil
}

func (s *State) getOperationParameters(ctx context.Context, tx *sqlair.TX, operationUUIDStr string) ([]taskParameter, error) {
	opUUID := uuid{UUID: operationUUIDStr}

	query := `
SELECT * AS &taskParameter.*
FROM   operation_parameter
WHERE  operation_uuid = $uuid.uuid
`

	stmt, err := s.Prepare(query, taskParameter{}, opUUID)
	if err != nil {
		return nil, errors.Errorf("preparing parameters statement: %w", err)
	}

	var parameters []taskParameter
	err = tx.Query(ctx, stmt, opUUID).GetAll(&parameters)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, errors.Errorf("querying task parameters: %w", err)
	}

	return parameters, nil
}

func (s *State) getTaskLog(ctx context.Context, tx *sqlair.TX, taskID string) ([]taskLogEntry, error) {
	ident := taskIdent{ID: taskID}

	query := `
SELECT task_uuid AS &taskLogEntry.task_uuid,
       content AS &taskLogEntry.content,
       created_at AS &taskLogEntry.created_at
FROM   operation_task_log
JOIN   operation_task AS ot ON operation_task_log.task_uuid = ot.uuid
WHERE  ot.task_id = $taskIdent.task_id
ORDER BY created_at ASC
`
	stmt, err := s.Prepare(query, taskLogEntry{}, taskIdent{})
	if err != nil {
		return nil, errors.Errorf("preparing log statement: %w", err)
	}

	var logEntries []taskLogEntry
	err = tx.Query(ctx, stmt, ident).GetAll(&logEntries)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, errors.Errorf("querying task log entries: %w", err)
	}

	return logEntries, nil
}

func encodeTask(task taskResult, parameters []taskParameter, logs []taskLogEntry) (operation.TaskInfo, error) {
	result := operation.TaskInfo{
		Enqueued: task.EnqueuedAt,
		Status:   corestatus.Status(task.Status),
		Receiver: task.Receiver,
	}

	if task.Name.Valid {
		result.ActionName = task.Name.String
	}
	if task.ExecutionGroup.Valid {
		result.ExecutionGroup = &task.ExecutionGroup.String
	}
	if task.StartedAt.Valid {
		result.Started = task.StartedAt.Time
	}
	if task.CompletedAt.Valid {
		result.Completed = task.CompletedAt.Time
	}

	result.Parameters = transform.SliceToMap(parameters, func(p taskParameter) (string, any) {
		return p.Key, p.Value
	})

	result.Log = make([]operation.TaskLog, len(logs))
	for i, logEntry := range logs {
		result.Log[i] = operation.TaskLog{
			Timestamp: logEntry.CreatedAt,
			Message:   logEntry.Content,
		}
	}

	return result, nil
}
