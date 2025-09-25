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
	"github.com/juju/juju/domain/operation/internal"
	"github.com/juju/juju/internal/errors"
)

// GetTask returns the task identified by its ID.
// It returns the task as well as the path to its output in the object store,
// if any. It's up to the caller to retrieve the actual output from the object
// store.
//
// The following errors may be returned:
// - [operationerrors.TaskNotFound] when the task does not exists.
func (st *State) GetTask(ctx context.Context, taskID string) (operation.Task, *string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return operation.Task{}, nil, errors.Capture(err)
	}

	var (
		result     operation.Task
		outputPath *string
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error

		result, outputPath, err = st.getTask(ctx, tx, taskID)
		return errors.Capture(err)
	})
	if err != nil {
		return operation.Task{}, nil, errors.Errorf("getting task %q: %w", taskID, err)
	}

	return result, outputPath, nil
}

// GetMachineTaskIDsWithStatus retrieves all task IDs for a machine specified by
// name and a status filter.
func (s *State) GetMachineTaskIDsWithStatus(ctx context.Context, machineName string, statusFilter string) ([]string, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	type search struct {
		Machine string `db:"machine"`
		Status  string `db:"status"`
	}

	term := search{
		Machine: machineName,
		Status:  statusFilter,
	}

	stmt, err := s.Prepare(`
SELECT ot.task_id AS &taskIdent.task_id
FROM   operation_task AS ot
JOIN   operation_machine_task AS omt ON ot.uuid = omt.task_uuid
JOIN   operation_task_status AS ots ON ot.uuid = ots.task_uuid
JOIN   operation_task_status_value AS status ON ots.status_id = status.id
JOIN   machine ON omt.machine_uuid = machine.uuid
WHERE  machine.name = $search.machine
AND    status.status = $search.status`, term, taskIdent{})
	if err != nil {
		return nil, errors.Errorf("preparing statement for fetching tasks with status %q for machine %q: %w", statusFilter, machineName, err)
	}

	var tasks []taskIdent
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if qerr := tx.Query(ctx, stmt, term).GetAll(&tasks); qerr != nil && !errors.Is(qerr, sqlair.ErrNoRows) {
			return errors.Capture(qerr)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Errorf("fetching tasks with status %q for machine %q: %w", statusFilter, machineName, err)
	}
	return transform.Slice(tasks, func(task taskIdent) string {
		return task.ID
	}), nil
}

// GetTaskStatusByID returns the status of the given task.
func (st *State) GetTaskStatusByID(ctx context.Context, taskID string) (string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	stmt, err := st.Prepare(
		`
SELECT otsv.status AS &taskStatus.*
FROM   operation_task AS ot
JOIN   operation_task_status AS ots ON ot.uuid = ots.task_uuid
JOIN   operation_task_status_value AS otsv ON ots.status_id = otsv.id
WHERE  ot.task_id = $taskIdent.task_id
`, taskIdent{}, taskStatus{})
	if err != nil {
		return "", errors.Errorf("preparing task status statement: %w", err)
	}

	var status taskStatus
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		ident := taskIdent{ID: taskID}

		err = tx.Query(ctx, stmt, ident).Get(&status)
		if errors.Is(err, sql.ErrNoRows) {
			return operationerrors.TaskNotFound
		} else if err != nil {
			return errors.Errorf("getting task status: %w", err)
		}

		return nil
	})
	if err != nil {
		return "", errors.Errorf("task %q: %w", taskID, err)
	}

	return status.Status, nil
}

// StartTask sets the task start time and updates the status to running.
// Returns [operationerrors.TaskNotFound] if the task does not exist,
// and [operationerrors.TaskNotPending] if the task is not pending.
func (st *State) StartTask(ctx context.Context, taskID string) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	updateStartedAt := taskTime{
		TaskID: taskID,
		Time:   time.Now().UTC(),
	}
	updateStartedStmt, err := st.Prepare(`
UPDATE operation_task
SET    started_at = $taskTime.time
WHERE  task_id = $taskTime.task_id
`, updateStartedAt)
	if err != nil {
		return errors.Errorf("preparing status update started at for task ID %q: %w", taskID, err)
	}

	task := taskIdent{ID: taskID}
	updateStatus := taskStatus{
		Status:    corestatus.Running.String(),
		UpdatedAt: updateStartedAt.Time,
	}
	type status struct {
		Status string `db:"status"`
	}
	startingStatus := status{Status: corestatus.Pending.String()}

	updateStatusStmt, err := st.Prepare(`
UPDATE operation_task_status
SET    status_id = (
           SELECT id FROM operation_task_status_value WHERE status = $taskStatus.status
       ), 
       updated_at = $taskStatus.updated_at
WHERE  task_uuid = (
           SELECT uuid FROM operation_task WHERE task_id = $taskIdent.task_id
       )
AND    status_id = (
           SELECT id FROM operation_task_status_value WHERE status = $status.status
       )
`, task, updateStatus, startingStatus)
	if err != nil {
		return errors.Errorf("preparing status update statement for task ID %q: %w", taskID, err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var outcome sqlair.Outcome

		err = tx.Query(ctx, updateStartedStmt, updateStartedAt).Get(&outcome)
		if err != nil {
			return errors.Errorf("updating start time of task ID %q: %w", taskID, err)
		}
		if err = verifyOneOutcome(outcome, operationerrors.TaskNotFound); err != nil {
			return errors.Errorf("task %q: %w", taskID, err)
		}

		outcome = sqlair.Outcome{}
		err = tx.Query(ctx, updateStatusStmt, task, updateStatus, startingStatus).Get(&outcome)
		if err != nil {
			return errors.Errorf("updating status of task ID %q: %w", taskID, err)
		}
		if err = verifyOneOutcome(outcome, operationerrors.TaskNotPending); err != nil {
			return errors.Errorf("task %q: %w", taskID, err)
		}
		return nil
	})

	if err != nil {
		return errors.Errorf("starting task %q: %w", taskID, err)
	}

	return nil
}

// verifyOneOutcome returns the provided error if the outcome's
// rows affected is not 1.
func verifyOneOutcome(outcome sqlair.Outcome, returnError error) error {
	n, err := outcome.Result().RowsAffected()
	if err != nil {
		return errors.Capture(err)
	}
	if n == 0 {
		return returnError
	}
	return nil
}

// FinishTask updates the task status to an inactive status value
// and saves a reference to its results in the object store. If the
// task's operation has no active tasks, mark the completed time for
// the operation.
// Returns [operationerrors.TaskNotFound] if the task does not exist.
func (st *State) FinishTask(ctx context.Context, task internal.CompletedTask) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	completedTime := time.Now().UTC()

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := st.updateOperationTaskStatus(ctx, tx, task.TaskUUID, task.Status, task.Message, completedTime); err != nil {
			return errors.Capture(err)
		}

		if err := st.insertOperationTaskOutput(ctx, tx, task.TaskUUID, task.StoreUUID); err != nil {
			return errors.Capture(err)
		}

		if err := st.maybeCompleteOperation(ctx, tx, task.TaskUUID, completedTime); err != nil {
			return errors.Capture(err)
		}

		return nil
	})
	if err != nil {
		return errors.Errorf("%w", err)
	}
	return nil
}

func (st *State) updateOperationTaskStatus(
	ctx context.Context,
	tx *sqlair.TX,
	taskUUID, status, message string,
	completedTime time.Time,
) error {
	statusValue := taskStatus{
		TaskUUID:  taskUUID,
		Status:    status,
		Message:   message,
		UpdatedAt: completedTime,
	}

	stmt, err := st.Prepare(`
UPDATE operation_task_status
SET    message = $taskStatus.message,
       updated_at = $taskStatus.updated_at,
       status_id = (
           SELECT id FROM operation_task_status_value WHERE status = $taskStatus.status
       )
WHERE task_uuid = $taskStatus.task_uuid
`, taskStatus{})
	if err != nil {
		return errors.Errorf("preparing update task status statement: %w", err)
	}

	var outcome sqlair.Outcome
	err = tx.Query(ctx, stmt, statusValue).Get(&outcome)
	if err != nil {
		return errors.Errorf("updating task status %q: %w", taskUUID, err)
	}

	if err := verifyOneOutcome(outcome, operationerrors.TaskNotFound); err != nil {
		return errors.Errorf("task %q: %w", taskUUID, err)
	}
	return nil
}

func (st *State) insertOperationTaskOutput(
	ctx context.Context,
	tx *sqlair.TX,
	taskUUID, storeUUID string,
) error {

	store := outputStore{
		TaskUUID:  taskUUID,
		StoreUUID: storeUUID,
	}

	stmt, err := st.Prepare(`
INSERT INTO operation_task_output (*)
VALUES ($outputStore.*)
`, store)
	if err != nil {
		return errors.Errorf("preparing insert task output store statement: %w", err)
	}

	err = tx.Query(ctx, stmt, store).Run()
	if err != nil {
		return errors.Errorf("inserting task %q output: %w", taskUUID, err)
	}

	return nil
}

func (st *State) maybeCompleteOperation(
	ctx context.Context,
	tx *sqlair.TX,
	taskUUID string,
	completedTime time.Time,
) error {
	tUUID := taskUUIDTime{
		TaskUUID: taskUUID,
		Time:     completedTime,
	}
	statuses := corestatus.ActiveTaskStatuses()

	type status []string

	// How many active tasks does this operation have?
	stmt, err := st.Prepare(`
WITH
-- Get the last updated task operation uuid
task AS (
    SELECT operation_uuid
    FROM   operation_task
    WHERE  uuid = $taskUUIDTime.task_uuid
),
-- Check if any of its task is not completed
not_completed AS (
    SELECT operation_uuid AS uuid
    FROM   operation_task AS ot
    JOIN   operation_task_status AS ots ON ot.uuid = ots.task_uuid
    JOIN   operation_task_status_value AS otsv ON ots.status_id = otsv.id
    WHERE  ot.operation_uuid = (SELECT operation_uuid FROM task)
    AND    otsv.status IN ($status[:])
    LIMIT  1
)
-- update the task operation only if it is completed
UPDATE operation
SET    completed_at = $taskUUIDTime.time
WHERE  operation.uuid = (SELECT operation_uuid FROM task)
AND    operation.uuid NOT IN not_completed
`, status{}, taskUUIDTime{})
	if err != nil {
		return errors.Errorf("preparing maybe complete operation statement: %w", err)
	}

	err = tx.Query(ctx, stmt, tUUID, status(statuses)).Run()
	if err != nil {
		return errors.Errorf("maybe complete operation : %w", err)
	}

	return nil
}

// GetReceiverFromTaskID returns a receiver string for the task identified.
// The string should satisfy the ActionReceiverTag type.
// Returns [operationerrors.TaskNotFound] if the task does not exist.
func (st *State) GetReceiverFromTaskID(ctx context.Context, taskID string) (string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	stmt, err := st.Prepare(`
SELECT    COALESCE(u.name, m.name) AS &nameArg.name
FROM      operation_task AS ot
LEFT JOIN operation_unit_task AS out ON ot.uuid = out.task_uuid
LEFT JOIN unit AS u ON out.unit_uuid = u.uuid
LEFT JOIN operation_machine_task AS omt ON ot.uuid = omt.task_uuid
LEFT JOIN machine AS m ON omt.machine_uuid = m.uuid
WHERE     ot.task_id = $taskIdent.task_id
`, taskIdent{}, nameArg{})
	if err != nil {
		return "", errors.Errorf("preparing task receiver statement: %w", err)
	}

	var receiver nameArg
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, taskIdent{ID: taskID}).Get(&receiver)
		if errors.Is(err, sql.ErrNoRows) {
			return errors.Errorf("task %q: %w", taskID, operationerrors.TaskNotFound)
		}
		return errors.Capture(err)
	})
	if err != nil {
		return "", errors.Errorf("getting receiver for task %q: %w", taskID, err)
	}
	return receiver.Name, nil
}

// CancelTask attempts to cancel an enqueued task, identified by its
// ID.
//
// The following errors may be returned:
// - [operationerrors.TaskNotFound] when the task does not exists.
func (st *State) CancelTask(ctx context.Context, taskID string) (operation.Task, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return operation.Task{}, errors.Capture(err)
	}

	var result operation.Task
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Attempt to cancel the task.
		err = st.cancelTask(ctx, tx, taskID)
		if err != nil {
			return errors.Capture(err)
		}

		result, _, err = st.getTask(ctx, tx, taskID)
		return errors.Capture(err)
	})
	if err != nil {
		return operation.Task{}, errors.Errorf("cancelling task %q: %w", taskID, err)
	}

	return result, nil
}

// cancelTask updates a specific task to cancelled status.
func (st *State) cancelTask(ctx context.Context, tx *sqlair.TX, taskID string) error {
	taskIDParam := taskIdent{ID: taskID}

	currentStatusQuery := `
SELECT otsv.status AS &taskStatus.status
FROM   operation_task AS ot
JOIN   operation_task_status AS ots ON ots.task_uuid = ot.uuid
JOIN   operation_task_status_value AS otsv ON ots.status_id = otsv.id
WHERE  ot.task_id = $taskIdent.task_id
`
	currentStatusStmt, err := st.Prepare(currentStatusQuery, taskStatus{}, taskIDParam)
	if err != nil {
		return errors.Errorf("preparing retrieve current status statement for task ID %q: %w", taskID, err)
	}

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
	updateStatusStmt, err := st.Prepare(updateStatusQuery, taskStatus{}, taskIDParam)
	if err != nil {
		return errors.Errorf("preparing update status statement for task ID %q: %w", taskID, err)
	}

	// If the status is Completed, Cancelled, Failed, Aborted or Error then
	// there'st nothing to do.
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
		newStatus.Status = corestatus.Aborting.String()
	}
	err = tx.Query(ctx, updateStatusStmt, newStatus, taskIDParam).Run()
	if err != nil {
		return errors.Errorf("updating status for cancelled task ID %q: %w", taskID, err)
	}

	return nil
}

func (st *State) getTask(ctx context.Context, tx *sqlair.TX, taskID string) (operation.Task, *string, error) {
	result, err := st.getOperationTask(ctx, tx, taskID)
	if err != nil {
		return operation.Task{}, nil, errors.Capture(err)
	}

	parameters, err := st.getOperationParameters(ctx, tx, result.OperationUUID)
	if err != nil {
		return operation.Task{}, nil, errors.Capture(err)
	}

	logEntries, err := st.getTaskLog(ctx, tx, taskID)
	if err != nil {
		return operation.Task{}, nil, errors.Capture(err)
	}

	task, err := encodeTask(taskID, result, parameters, logEntries)
	if err != nil {
		return operation.Task{}, nil, errors.Capture(err)
	}

	var outputPath *string
	if result.OutputPath.Valid {
		outputPath = &result.OutputPath.String
	}

	return task, outputPath, nil
}

func (st *State) getOperationTask(ctx context.Context, tx *sqlair.TX, taskID string) (taskResult, error) {
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
	stmt, err := st.Prepare(query, taskResult{}, ident)
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

func (st *State) getOperationParameters(ctx context.Context, tx *sqlair.TX, operationUUIDStr string) ([]taskParameter, error) {
	opUUID := uuid{UUID: operationUUIDStr}

	query := `
SELECT * AS &taskParameter.*
FROM   operation_parameter
WHERE  operation_uuid = $uuid.uuid
`

	stmt, err := st.Prepare(query, taskParameter{}, opUUID)
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

func (st *State) getTaskLog(ctx context.Context, tx *sqlair.TX, taskID string) ([]taskLogEntry, error) {
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
	stmt, err := st.Prepare(query, taskLogEntry{}, taskIdent{})
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

func encodeTask(taskID string, task taskResult, parameters []taskParameter, logs []taskLogEntry) (operation.Task, error) {
	result := operation.Task{
		TaskInfo: operation.TaskInfo{
			ID:       taskID,
			Enqueued: task.EnqueuedAt,
			Status:   corestatus.Status(task.Status),
		},
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

// LogTaskMessage stores the message for the given task ID.
func (st *State) LogTaskMessage(ctx context.Context, taskID, message string) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	stmt, err := st.Prepare(
		`
INSERT INTO operation_task_log (task_uuid, content, created_at)
SELECT ot.uuid,
       $taskLogEntry.content,
       $taskLogEntry.created_at
FROM   operation_task AS ot
WHERE  ot.task_id = $taskIdent.task_id
`, taskLogEntry{}, taskIdent{})
	if err != nil {
		return errors.Errorf("preparing log statement: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		ident := taskIdent{ID: taskID}
		content := taskLogEntry{
			Content:   message,
			CreatedAt: time.Now().UTC(),
		}

		err = tx.Query(ctx, stmt, ident, content).Run()
		if errors.Is(err, sql.ErrNoRows) {
			return operationerrors.TaskNotFound
		} else if err != nil {
			return errors.Errorf("inserting task log entry: %w", err)
		}

		return nil
	})
	if err != nil {
		return errors.Errorf("logging task %q: %w", taskID, err)
	}

	return nil
}
