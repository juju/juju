// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"strings"

	"github.com/canonical/sqlair"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/operation"
	operationerrors "github.com/juju/juju/domain/operation/errors"
	"github.com/juju/juju/internal/errors"
)

// GetOperations returns a list of operations on specified entities, filtered by the
// given parameters.
func (st *State) GetOperations(ctx context.Context, params operation.QueryArgs) (operation.QueryResult, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return operation.QueryResult{}, errors.Capture(err)
	}

	var (
		ops       []operationResult
		allTasks  map[string][]taskResult
		allParams map[string][]taskParameter
		// map from operation UUID to a map from task ID to its logs.
		allTaskLogs map[string]map[string][]taskLogEntry
	)
	allTasks = make(map[string][]taskResult)
	allParams = make(map[string][]taskParameter)
	allTaskLogs = make(map[string]map[string][]taskLogEntry)

	// Pagination set-up.
	var (
		limit  = 10 // Default 10 operations per page.
		offset int
	)
	if params.Limit != nil {
		limit = *params.Limit
	}
	if params.Offset != nil {
		offset = *params.Offset
	}

	// Build the query for operations with filters.
	query, queryArgs := st.buildOperationsQuery(params, limit, offset)
	stmt, err := st.Prepare(query, append([]any{operationResult{}}, queryArgs...)...)
	if err != nil {
		return operation.QueryResult{}, errors.Errorf("preparing operations query: %w", err)
	}
	st.logger.Debugf(ctx, "executing query: %q, with args %v", query, queryArgs)

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, queryArgs...).GetAll(&ops)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Capture(err)
		}

		for _, op := range ops {
			// Get all the tasks, parameters and logs for the given operation.
			tasks, parameters, taskLogs, err := st.getFullTasksForOperation(ctx, tx, op.UUID)
			if err != nil {
				return errors.Capture(err)
			}
			allTasks[op.UUID] = tasks
			allParams[op.UUID] = parameters
			allTaskLogs[op.UUID] = taskLogs
		}
		return nil
	})
	if err != nil {
		return operation.QueryResult{}, errors.Capture(err)
	}

	// Encode results
	var opInfos []operation.OperationInfo
	for _, op := range ops {
		opInfo, err := encodeOperationInfo(
			op,
			allTasks[op.UUID],
			allParams[op.UUID],
			allTaskLogs[op.UUID],
		)
		if err != nil {
			return operation.QueryResult{}, errors.Errorf("encoding operation info for operation %q: %w", op.OperationID, err)
		}
		opInfos = append(opInfos, opInfo)
	}

	// Truncated: true if we got limit results (could be more)
	// but only if limit is positive.
	truncated := limit > 0 && len(opInfos) == limit

	return operation.QueryResult{
		Operations: opInfos,
		Truncated:  truncated,
	}, nil
}

// GetOperationByID returns an operation by its ID.
//
// The following errors may be returned:
// - [operationerrors.OperationNotFound]: when the operation was not found.
func (st *State) GetOperationByID(ctx context.Context, operationID string) (operation.OperationInfo, error) {
	db, err := st.DB(ctx)
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
		op, err = st.getOperation(ctx, tx, operationID)
		if err != nil {
			return errors.Capture(err)
		}

		// Get all the tasks, parameters and logs for the given operation.
		tasks, parameters, taskLogs, err = st.getFullTasksForOperation(ctx, tx, op.UUID)
		if err != nil {
			return errors.Capture(err)
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

// getFullTasksForOperation retrieves all tasks for a given operation,
// including their logs and the operation parameters.
func (st *State) getFullTasksForOperation(ctx context.Context, tx *sqlair.TX, opUUID string) ([]taskResult, []taskParameter, map[string][]taskLogEntry, error) {
	// Get the operation parameters.
	parameters, err := st.getOperationParameters(ctx, tx, opUUID)
	if err != nil {
		return nil, nil, nil, errors.Capture(err)
	}

	// Get all tasks for this operation.
	tasks, err := st.getOperationTasks(ctx, tx, opUUID)
	if err != nil {
		return nil, nil, nil, errors.Capture(err)
	}

	taskLogs := make(map[string][]taskLogEntry)
	for _, task := range tasks {
		// Get the task logs.
		logs, err := st.getTaskLog(ctx, tx, task.TaskID)
		if err != nil {
			return nil, nil, nil, errors.Capture(err)
		}
		taskLogs[task.TaskID] = logs
	}

	return tasks, parameters, taskLogs, nil
}

// getOperation retrieves the operation row for a given operation_id.
func (st *State) getOperation(ctx context.Context, tx *sqlair.TX, oID string) (operationResult, error) {
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
	stmt, err := st.Prepare(query, operationResult{}, ident)
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

		encodedTask, err := encodeTask(t, parameters, logs)
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

// buildOperationsQuery constructs the SQL query and arguments for filtering operations
func (st *State) buildOperationsQuery(params operation.QueryArgs, limit, offset int) (string, []any) {
	// Define local typed slices for sqlair parameters
	type actionNames []string
	type statuses []string
	type applications []string
	type machines []string
	type units []string
	type leaderApps []string

	var args []any

	// Add pagination parameters
	paginationParams := queryParams{Limit: limit, Offset: offset}
	args = append(args, paginationParams)

	// Base query - we need to ensure we join with tasks and their receivers properly
	query := `
SELECT o.uuid AS &operationResult.uuid,
       o.operation_id AS &operationResult.operation_id,
       o.summary AS &operationResult.summary,
       o.enqueued_at AS &operationResult.enqueued_at,
       o.started_at AS &operationResult.started_at,
       o.completed_at AS &operationResult.completed_at
FROM   operation o`

	// Build WHERE clauses
	whereClauses := []string{}
	joinClauses := []string{}
	needsTaskJoin := false

	// Check if we need task joins
	needsTaskJoin = len(params.Status) > 0 ||
		len(params.Receivers.Applications) > 0 ||
		len(params.Receivers.Machines) > 0 ||
		len(params.Receivers.Units) > 0 ||
		len(params.Receivers.LeaderUnit) > 0

	// ActionNames filter - need to join with operation_action
	if len(params.ActionNames) > 0 {
		joinClauses = append(joinClauses, "JOIN operation_action oa ON o.uuid = oa.operation_uuid")
		whereClauses = append(whereClauses, "oa.charm_action_key IN ($actionNames[:])")
		args = append(args, actionNames(params.ActionNames))
	}

	// Add task joins if needed
	if needsTaskJoin {
		joinClauses = append(joinClauses, "JOIN operation_task t ON o.uuid = t.operation_uuid")
	}

	// Status filter - need to compute status from tasks
	if len(params.Status) > 0 {
		statusStrings := make([]string, len(params.Status))
		for i, st := range params.Status {
			statusStrings[i] = st.String()
		}
		joinClauses = append(joinClauses, "JOIN operation_task_status ts ON t.uuid = ts.task_uuid")
		joinClauses = append(joinClauses, "JOIN operation_task_status_value sv ON ts.status_id = sv.id")
		whereClauses = append(whereClauses, "sv.status IN ($statuses[:])")
		args = append(args, statuses(statusStrings))
	}

	// Receivers filter - need to join with tasks and their targets
	receiverClauses := []string{}

	if len(params.Receivers.Applications) > 0 {
		joinClauses = append(joinClauses, "LEFT JOIN operation_unit_task ut ON t.uuid = ut.task_uuid")
		joinClauses = append(joinClauses, "LEFT JOIN unit u ON ut.unit_uuid = u.uuid")
		joinClauses = append(joinClauses, "LEFT JOIN application a ON u.application_uuid = a.uuid")
		receiverClauses = append(receiverClauses, "a.name IN ($applications[:])")
		args = append(args, applications(params.Receivers.Applications))
	}

	if len(params.Receivers.Machines) > 0 {
		joinClauses = append(joinClauses, "LEFT JOIN operation_machine_task mt ON t.uuid = mt.task_uuid")
		joinClauses = append(joinClauses, "LEFT JOIN machine m ON mt.machine_uuid = m.uuid")
		machineNames := make([]string, len(params.Receivers.Machines))
		for i, name := range params.Receivers.Machines {
			machineNames[i] = string(name)
		}
		receiverClauses = append(receiverClauses, "m.name IN ($machines[:])")
		args = append(args, machines(machineNames))
	}

	if len(params.Receivers.Units) > 0 {
		// If we haven't already joined with unit tables for applications
		if len(params.Receivers.Applications) == 0 {
			joinClauses = append(joinClauses, "LEFT JOIN operation_unit_task ut ON t.uuid = ut.task_uuid")
			joinClauses = append(joinClauses, "LEFT JOIN unit u ON ut.unit_uuid = u.uuid")
		}
		unitNames := make([]string, len(params.Receivers.Units))
		for i, name := range params.Receivers.Units {
			unitNames[i] = string(name)
		}
		receiverClauses = append(receiverClauses, "u.name IN ($units[:])")
		args = append(args, units(unitNames))
	}

	if len(params.Receivers.LeaderUnit) > 0 {
		// Leader unit filtering is more complex - would need leader election info
		// For now, we'll treat it as application filtering
		if len(params.Receivers.Applications) == 0 && len(params.Receivers.Units) == 0 {
			joinClauses = append(joinClauses, "LEFT JOIN operation_unit_task ut ON t.uuid = ut.task_uuid")
			joinClauses = append(joinClauses, "LEFT JOIN unit u ON ut.unit_uuid = u.uuid")
			joinClauses = append(joinClauses, "LEFT JOIN application a ON u.application_uuid = a.uuid")
		} else if len(params.Receivers.Applications) == 0 {
			joinClauses = append(joinClauses, "LEFT JOIN application a ON u.application_uuid = a.uuid")
		}
		receiverClauses = append(receiverClauses, "a.name IN ($leaderApps[:])")
		args = append(args, leaderApps(params.Receivers.LeaderUnit))
	}

	if len(receiverClauses) > 0 {
		whereClauses = append(whereClauses, "("+strings.Join(receiverClauses, " OR ")+")")
	}

	// Assemble the query
	if len(joinClauses) > 0 {
		query += "\n" + strings.Join(joinClauses, "\n")
	}
	if len(whereClauses) > 0 {
		query += "\nWHERE " + strings.Join(whereClauses, " AND ")
	}
	query += "\nORDER BY o.enqueued_at DESC\nLIMIT $queryParams.limit OFFSET $queryParams.offset"

	return query, args
}
