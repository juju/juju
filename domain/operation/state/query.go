// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/operation"
	operationerrors "github.com/juju/juju/domain/operation/errors"
	"github.com/juju/juju/internal/errors"
)

// GetOperations returns a list of operations on specified entities, filtered by the
// given parameters.
func (s *State) GetOperations(ctx context.Context, params operation.QueryArgs) (operation.QueryResult, error) {
	return operation.QueryResult{}, errors.New("actions in Dqlite not supported")
}

// GetOperationByID returns an operation by its ID.
func (s *State) GetOperationByID(ctx context.Context, operationID string) (operation.OperationInfo, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return operation.OperationInfo{}, errors.Capture(err)
	}

	var (
		op         operationResult
		tasks      []taskResult
		parameters []taskParameter
		// logs is a map from task ID to a list of log entries. We need it
		// as such to be passed at the end to the encode function.
		taskLogs map[string][]taskLogEntry
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Get operation the root operation.
		op, err = s.getOperation(ctx, tx, operationID)
		if err != nil {
			return errors.Capture(err)
		}

		// Get the operation parameters.
		parameters, err = s.getOperationParameters(ctx, tx, op.UUID)
		if err != nil {
			return errors.Capture(err)
		}

		// Get all tasks for this operation.
		tasks, err = s.getOperationTasks(ctx, tx, op.UUID)
		if err != nil {
			return errors.Capture(err)
		}

		taskLogs = make(map[string][]taskLogEntry)
		for _, task := range tasks {
			// Get the task logs.
			logs, err := s.getTaskLog(ctx, tx, task.TaskID)
			if err != nil {
				return errors.Capture(err)
			}
			taskLogs[task.TaskID] = logs
		}

		return nil
	})
	if err != nil {
		return operation.OperationInfo{}, errors.Capture(err)
	}

	opInfo, err := encodeOperationInfo(op, tasks, parameters, taskLogs)
	if err != nil {
		return operation.OperationInfo{}, errors.Errorf("encoding operation info for operation %q: %w", operationID, err)
	}
	return opInfo, nil
}

// getOperation retrieves the operation row for a given operation_id.
func (s *State) getOperation(ctx context.Context, tx *sqlair.TX, oID string) (operationResult, error) {
	ident := operationID{OperationID: oID}
	query := `
SELECT uuid AS &operationResult.uuid,
       operation_id AS &operationResult.operation_id,
       summary AS &operationResult.summary,
       enqueued_at AS &operationResult.enqueued_at,
       started_at AS &operationResult.started_at,
       completed_at AS &operationResult.completed_at
FROM   operation
WHERE  operation_id = $operationID.operation_id
`
	var op operationResult
	stmt, err := s.Prepare(query, operationResult{}, ident)
	if err != nil {
		return operationResult{}, errors.Errorf("preparing operation query: %w", err)
	}
	err = tx.Query(ctx, stmt, ident).Get(&op)
	if errors.Is(err, sqlair.ErrNoRows) {
		return operationResult{}, errors.Errorf("operation %q not found", oID).Add(operationerrors.OperationNotFound)
	} else if err != nil {
		return operationResult{}, errors.Errorf("querying operation: %w", err)
	}
	return op, nil
}

// encodeOperationInfo maps an operation along with its tasks (logs for each
// task provided as a map from task IDs to logs) to a resulting OperationInfo.
func encodeOperationInfo(
	op operationResult,
	tasks []taskResult,
	parameters []taskParameter,
	taskLogs map[string][]taskLogEntry,
) (operation.OperationInfo, error) {
	var opInfo operation.OperationInfo
	opInfo.OperationID = op.OperationID
	opInfo.Summary = ""
	if op.Summary.Valid {
		opInfo.Summary = op.Summary.String
	}
	opInfo.Enqueued = op.EnqueuedAt
	if op.StartedAt.Valid {
		opInfo.Started = op.StartedAt.Time
	}
	if op.CompletedAt.Valid {
		opInfo.Completed = op.CompletedAt.Time
	}

	var machines []operation.MachineTaskResult
	var units []operation.UnitTaskResult
	for _, t := range tasks {
		// Retrieve the logs for the task.
		logs, ok := taskLogs[t.TaskID]
		if !ok {
			// We don't break if we don't have logs for a particular task.
			logs = []taskLogEntry{}
		}

		encodedTask, err := encodeTask(t.TaskID, t, parameters, logs)
		if err != nil {
			return operation.OperationInfo{}, errors.Errorf("encoding task %q: %w", t.TaskID, err)
		}

		// Tasks can be either unit or machine tasks. We deduce this from the
		// validity of the UnitName.
		if t.UnitName.Valid {
			units = append(units, operation.UnitTaskResult{
				TaskInfo:     encodedTask.TaskInfo,
				ReceiverName: unit.Name(encodedTask.Receiver),
			})
		} else {
			machines = append(machines, operation.MachineTaskResult{
				TaskInfo:     encodedTask.TaskInfo,
				ReceiverName: coremachine.Name(encodedTask.Receiver),
			})
		}
	}
	opInfo.Machines = machines
	opInfo.Units = units

	return opInfo, nil
}
