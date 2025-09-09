// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"

	"github.com/juju/collections/transform"
	"github.com/juju/juju/domain/operation"
	operationerrors "github.com/juju/juju/domain/operation/errors"
	"github.com/juju/juju/internal/errors"
	internaluuid "github.com/juju/juju/internal/uuid"
)

// GetTask returns the task identified by its ID.
// It returns the task as well as the path to its output in the object store,
// if any. It's up to the caller to retrieve the actual output from the object
// store.
//
// The following errors may be returned:
// - [operationerrors.TaskNotFound] when the task does not exists.
func (s *State) GetTask(ctx context.Context, taskID string) (operation.Task, *string, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return operation.Task{}, nil, errors.Capture(err)
	}

	var (
		result     operation.Task
		outputPath *string
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error

		result, outputPath, err = s.getTask(ctx, tx, taskID)
		return errors.Capture(err)
	})
	if err != nil {
		return operation.Task{}, nil, errors.Errorf("getting action %q: %w", taskID, err)
	}

	return result, outputPath, nil
}

// CancelTask attempts to cancel an enqueued task, identified by its
// ID.
//
// The following errors may be returned:
// - [operationerrors.TaskNotFound] when the task does not exists.
func (s *State) CancelTask(ctx context.Context, taskID string) (operation.Task, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return operation.Task{}, errors.Capture(err)
	}

	var result operation.Task
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Attempt to cancel the task.
		err = s.cancelTask(ctx, tx, taskID)
		if err != nil {
			return errors.Errorf("cancelling task %q: %w", taskID, err)
		}

		result, _, err = s.getTask(ctx, tx, taskID)
		return errors.Capture(err)
	})
	if err != nil {
		return operation.Task{}, errors.Errorf("cancelling action %q: %w", taskID, err)
	}

	return result, nil
}

// cancelTask updates a specific task to cancelled status.
func (s *State) cancelTask(ctx context.Context, tx *sqlair.TX, taskID string) error {
	taskIDParam := taskIdent{ID: taskID}

	currentStatusQuery := `
SELECT ots.status_id AS &taskStatus.status_id
FROM   operation_task_status ots
JOIN   operation_task ot ON ots.task_uuid = ot.uuid
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

	// If the task is in Pending status, then we have to update its status to
	// Cancelled.
	// If the task is already in Running status, then we have to update its
	// status to Aborting.
	// If the status is Completed, Cancelled, Failed, Aborted or Error then
	// there's nothing to do.

	// TODO(nvinuesa): Implement this logic in a future patch.

	return nil
}

func (s *State) getTask(ctx context.Context, tx *sqlair.TX, taskID string) (operation.Task, *string, error) {
	result, err := s.getOperationTask(ctx, tx, taskID)
	if err != nil {
		return operation.Task{}, nil, errors.Capture(err)
	}

	parameters, err := s.getOperationParameters(ctx, tx, result.OperationUUID)
	if err != nil {
		return operation.Task{}, nil, errors.Capture(err)
	}

	logEntries, err := s.getTaskLog(ctx, tx, taskID)
	if err != nil {
		return operation.Task{}, nil, errors.Capture(err)
	}

	task, err := encodeTask(result, parameters, logEntries)
	if err != nil {
		return operation.Task{}, nil, errors.Capture(err)
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
       COALESCE(u.name, m.name) AS &taskResult.receiver,
       oa.charm_action_key AS &taskResult.name,
       o.summary AS &taskResult.summary,
       o.parallel AS &taskResult.parallel,
       o.execution_group AS &taskResult.execution_group,
       o.enqueued_at AS &taskResult.enqueued_at,
       ot.started_at AS &taskResult.started_at,
       ot.completed_at AS &taskResult.completed_at,
       osv.status AS &taskResult.status,
       os.path AS &taskResult.path
FROM operation_task ot
JOIN operation o ON ot.operation_uuid = o.uuid
JOIN operation_task_status ots ON ot.uuid = ots.task_uuid
JOIN operation_task_status_value osv ON ots.status_id = osv.id
LEFT JOIN operation_action oa ON o.uuid = oa.operation_uuid
LEFT JOIN operation_unit_task out ON ot.uuid = out.task_uuid
LEFT JOIN unit u ON out.unit_uuid = u.uuid
LEFT JOIN operation_machine_task omt ON ot.uuid = omt.task_uuid
LEFT JOIN machine m ON omt.machine_uuid = m.uuid
LEFT JOIN operation_task_output oto ON ot.uuid = oto.task_uuid
LEFT JOIN v_object_store_metadata os ON oto.store_uuid = os.uuid
WHERE ot.task_id = $taskIdent.task_id
`
	stmt, err := s.Prepare(query, taskResult{}, ident)
	if err != nil {
		return taskResult{}, errors.Errorf("preparing get action statement: %w", err)
	}

	var result taskResult
	err = tx.Query(ctx, stmt, ident).Get(&result)
	if errors.Is(err, sql.ErrNoRows) {
		return taskResult{}, errors.Errorf("action with task ID %q not found", taskID).Add(operationerrors.TaskNotFound)
	} else if err != nil {
		return taskResult{}, errors.Errorf("querying action with task ID %q: %w", taskID, err)
	}

	return result, nil
}

func (s *State) getOperationParameters(ctx context.Context, tx *sqlair.TX, operationUUIDStr string) ([]taskParameter, error) {
	opUUID := uuid{UUID: operationUUIDStr}

	query := `
SELECT operation_uuid AS &taskParameter.operation_uuid,
       key AS &taskParameter.key,
       value AS &taskParameter.value
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
		return nil, errors.Errorf("querying action parameters: %w", err)
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
JOIN   operation_task ot ON operation_task_log.task_uuid = ot.uuid
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

func encodeTask(task taskResult, parameters []taskParameter, logs []taskLogEntry) (operation.Task, error) {
	actionUUID, err := internaluuid.UUIDFromString(task.OperationUUID)
	if err != nil {
		return operation.Task{}, err
	}

	result := operation.Task{
		UUID:     actionUUID,
		Enqueued: task.EnqueuedAt,
		Status:   task.Status,
	}

	if task.Receiver.Valid {
		result.Receiver = task.Receiver.String
	}
	if task.Name.Valid {
		result.Name = task.Name.String
	}
	if task.Summary.Valid {
		result.Name = task.Summary.String
	}
	if task.ExecutionGroup.Valid {
		result.ExecutionGroup = &task.ExecutionGroup.String
	}
	if task.StartedAt.Valid {
		result.Started = &task.StartedAt.Time
	}
	if task.CompletedAt.Valid {
		result.Completed = &task.CompletedAt.Time
	}

	result.Parameters = transform.SliceToMap(parameters, func(p taskParameter) (string, any) {
		return p.Key, p.Value
	})

	result.Log = make([]operation.TaskLogMessage, len(logs))
	for i, logEntry := range logs {
		result.Log[i] = operation.TaskLogMessage{
			Timestamp: logEntry.CreatedAt,
			Message:   logEntry.Content,
		}
	}

	return result, nil
}
