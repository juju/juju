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
	"github.com/juju/juju/internal/uuid"
)

// GetAction returns the action identified by its task ID.
// It returns the action as well as the path to its output in the object store,
// if any. It's up to the caller to retrieve the actual output from the object
// store.
//
// The following errors may be returned:
// - [operationerrors.ActionNotFound] when the action does not exists.
func (s *State) GetAction(ctx context.Context, taskID string) (operation.Action, string, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return operation.Action{}, "", errors.Capture(err)
	}

	var (
		result     operation.Action
		outputPath string
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error

		result, err = s.getActionByTaskID(ctx, tx, taskID)
		if err != nil {
			return errors.Capture(err)
		}
		// Retrieve the output path, if any.
		outputPath, err = s.getTaskOutput(ctx, tx, taskID)
		return errors.Capture(err)
	})
	if err != nil {
		return operation.Action{}, "", errors.Errorf("getting action %q: %w", taskID, err)
	}

	return result, outputPath, nil
}

// CancelAction attempts to cancel an enqueued action, identified by its
// task ID.
//
// The following errors may be returned:
// - [operationerrors.ActionNotFound] when the action does not exists.
func (s *State) CancelAction(ctx context.Context, taskID string) (operation.Action, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return operation.Action{}, errors.Capture(err)
	}

	var result operation.Action
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Attempt to cancel the task.
		err = s.cancelTask(ctx, tx, taskID)
		if err != nil {
			return errors.Errorf("cancelling task %q: %w", taskID, err)
		}

		result, err = s.getActionByTaskID(ctx, tx, taskID)
		return errors.Capture(err)
	})
	if err != nil {
		return operation.Action{}, errors.Errorf("cancelling action %q: %w", taskID, err)
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
		return errors.Errorf("task with ID %q not found", taskID).Add(operationerrors.ActionNotFound)
	} else if err != nil {
		return errors.Errorf("querying current status for task ID %q: %w", taskID, err)
	}

	// If the task is already enqueued, then we have to update its status to
	// Aborted.
	// If the task is in Pending status, then we have co update its status to
	// Cancelled.
	// If the status is Completed, Cancelled, Failed, Aborted or Error then
	// there's nothing to do.

	// TODO(nvinuesa): Implement this logic in a future patch.

	return nil
}

func (s *State) getActionByTaskID(ctx context.Context, tx *sqlair.TX, taskID string) (operation.Action, error) {
	result, err := s.getOperationTask(ctx, tx, taskID)
	if err != nil {
		return operation.Action{}, errors.Capture(err)
	}

	parameters, err := s.getOperationParameters(ctx, tx, result.OperationUUID)
	if err != nil {
		return operation.Action{}, errors.Capture(err)
	}

	logEntries, err := s.getTaskLog(ctx, tx, taskID)
	if err != nil {
		return operation.Action{}, errors.Capture(err)
	}

	action, err := encodeAction(result, parameters, nil, logEntries)
	if err != nil {
		return operation.Action{}, errors.Capture(err)
	}
	return action, nil
}

func (s *State) getOperationTask(ctx context.Context, tx *sqlair.TX, taskID string) (actionResult, error) {
	ident := taskIdent{ID: taskID}

	query := `
SELECT o.uuid AS &actionResult.operation_uuid,
       COALESCE(u.name, m.name) AS &actionResult.receiver,
       ca.key AS &actionResult.name,
       o.summary AS &actionResult.summary,
       o.parallel AS &actionResult.parallel,
       o.execution_group AS &actionResult.execution_group,
       o.enqueued_at AS &actionResult.enqueued_at,
       ot.started_at AS &actionResult.started_at,
       ot.completed_at AS &actionResult.completed_at,
       ot.uuid AS task_uuid
FROM operation_task ot
JOIN operation o ON ot.operation_uuid = o.uuid
JOIN operation_action oa ON o.uuid = oa.operation_uuid
JOIN charm_action ca ON oa.charm_uuid = ca.charm_uuid AND oa.charm_action_key = ca.key
LEFT JOIN operation_unit_task out ON ot.uuid = out.task_uuid
LEFT JOIN unit u ON out.unit_uuid = u.uuid
LEFT JOIN operation_machine_task omt ON ot.uuid = omt.task_uuid
LEFT JOIN machine m ON omt.machine_uuid = m.uuid
WHERE ot.task_id = $taskIdent.task_id
`
	stmt, err := s.Prepare(query, actionResult{}, ident)
	if err != nil {
		return actionResult{}, errors.Errorf("preparing get action statement: %w", err)
	}

	var result actionResult
	err = tx.Query(ctx, stmt, ident).Get(&result)
	if errors.Is(err, sql.ErrNoRows) {
		return actionResult{}, errors.Errorf("action with task ID %q not found", taskID).Add(operationerrors.ActionNotFound)
	} else if err != nil {
		return actionResult{}, errors.Errorf("querying action with task ID %q: %w", taskID, err)
	}

	return result, nil
}

func (s *State) getOperationParameters(ctx context.Context, tx *sqlair.TX, operationUUIDStr string) ([]actionParameter, error) {
	opUUID := operationUUID{UUID: operationUUIDStr}

	query := `
SELECT operation_uuid AS &actionParameter.operation_uuid,
       key AS &actionParameter.key,
       value AS &actionParameter.value
FROM   operation_parameter
WHERE  operation_uuid = $operationUUID.uuid
`

	stmt, err := s.Prepare(query, actionParameter{}, opUUID)
	if err != nil {
		return nil, errors.Errorf("preparing parameters statement: %w", err)
	}

	var parameters []actionParameter
	err = tx.Query(ctx, stmt, opUUID).GetAll(&parameters)
	if errors.Is(err, sql.ErrNoRows) {
		return []actionParameter{}, nil
	} else if err != nil {
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
	if errors.Is(err, sql.ErrNoRows) {
		return []taskLogEntry{}, nil
	} else if err != nil {
		return nil, errors.Errorf("querying task log entries: %w", err)
	}

	return logEntries, nil
}

func (s *State) getTaskOutput(ctx context.Context, tx *sqlair.TX, taskID string) (string, error) {
	ident := taskIdent{ID: taskID}

	// Get the path from the object store metadata
	pathQuery := `
SELECT path AS &objectStorePath.path
FROM   operation_task ot
JOIN   operation_task_output oto ON ot.uuid = oto.task_uuid
JOIN   v_object_store_metadata os ON oto.store_uuid = os.uuid
WHERE  ot.task_id = $taskIdent.task_id
`
	pathStmt, err := s.Prepare(pathQuery, objectStorePath{}, ident)
	if err != nil {
		return "", errors.Errorf("preparing object store path statement: %w", err)
	}

	var storePath objectStorePath
	err = tx.Query(ctx, pathStmt, ident).Get(&storePath)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", errors.Errorf("querying object store path: %w", err)
	}

	return storePath.Path, nil
}

func encodeAction(ar actionResult, parameters []actionParameter, output []taskOutputParameter, logs []taskLogEntry) (operation.Action, error) {
	actionUUID, err := uuid.UUIDFromString(ar.OperationUUID)
	if err != nil {
		return operation.Action{}, err
	}

	action := operation.Action{
		UUID:     actionUUID,
		Enqueued: ar.EnqueuedAt,
	}

	if ar.Receiver.Valid {
		action.Receiver = ar.Receiver.String
	}
	if ar.Name.Valid {
		action.Name = ar.Name.String
	}
	if ar.Summary.Valid {
		action.Name = ar.Summary.String
	}
	if ar.ExecutionGroup.Valid {
		action.ExecutionGroup = &ar.ExecutionGroup.String
	}
	if ar.StartedAt.Valid {
		action.Started = &ar.StartedAt.Time
	}
	if ar.CompletedAt.Valid {
		action.Completed = &ar.CompletedAt.Time
	}

	action.Parameters = transform.SliceToMap(parameters, func(p actionParameter) (string, any) {
		return p.Key, p.Value
	})

	action.Log = make([]operation.ActionMessage, len(logs))
	for i, logEntry := range logs {
		action.Log[i] = operation.ActionMessage{
			Timestamp: logEntry.CreatedAt,
			Message:   logEntry.Content,
		}
	}

	action.Output = transform.SliceToMap(output, func(o taskOutputParameter) (string, any) {
		return o.Key, o.Value
	})

	return action, nil
}
