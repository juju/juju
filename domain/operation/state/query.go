// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"strings"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"

	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/operation"
	operationerrors "github.com/juju/juju/domain/operation/errors"
	"github.com/juju/juju/internal/errors"
)

const defaultOperationsLimit = 50

// GetOperations returns a list of operations on specified entities, filtered by the
// given parameters.
func (st *State) GetOperations(ctx context.Context, params operation.QueryArgs) (operation.QueryResult, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return operation.QueryResult{}, errors.Capture(err)
	}

	var (
		ops []operationResult
		// Map from operation UUID to its tasks.
		allTasks map[string][]taskResult
		// Map from operation UUID to its parameters.
		allParams map[string][]taskParameter
		// Map from operation UUID to a map from task ID to its logs.
		allTaskLogs map[string]map[string][]taskLogEntryByOperation
		// Were the results truncated.
		truncated bool
	)
	allTasks = make(map[string][]taskResult)
	allParams = make(map[string][]taskParameter)
	allTaskLogs = make(map[string]map[string][]taskLogEntryByOperation)

	// Pagination set-up.
	paginationParams := queryParams{
		Limit:  defaultOperationsLimit + 1, // +1 to check for truncation.
		Offset: 0,
	}
	// User provided limit is capped at 50 (default and max).
	if params.Limit != nil && *params.Limit > 0 && *params.Limit < defaultOperationsLimit {
		paginationParams.Limit = *params.Limit + 1 // +1 to check for truncation.
	}
	if params.Offset != nil {
		paginationParams.Offset = *params.Offset
	}

	// Build the query for operations with filters.
	query, queryArgs := st.buildOperationsQuery(params)

	// Add pagination parameters to the query arguments.
	queryArgs = append(queryArgs, paginationParams)

	st.logger.Tracef(ctx, "preparing operations query: \n %q \n with arguments: %+v", query, queryArgs)

	stmt, err := st.Prepare(query, append([]any{operationResult{}}, queryArgs...)...)
	if err != nil {
		return operation.QueryResult{}, errors.Errorf("preparing operations query: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, queryArgs...).GetAll(&ops)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Capture(err)
		}
		// Since we are returning one more element than the limit to check for
		// truncation, we trim the results here if needed.
		// The limit is either the user provided limit (capped at 50) or
		// the default limit of 50.
		if len(ops) >= paginationParams.Limit {
			ops = ops[:paginationParams.Limit-1] // -1 to return only the limit requested.
			truncated = true
		}

		// Get the list of operation UUIDs to fetch related data.
		opUUIDs := transform.Slice(ops, func(op operationResult) string {
			return op.UUID
		})
		// Get all the tasks, parameters and logs for the given operation.
		allTasks, allParams, allTaskLogs, err = st.getFullTasksForOperation(ctx, tx, opUUIDs)
		if err != nil {
			return errors.Capture(err)
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
		taskLogs map[string][]taskLogEntryByOperation
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Get operation the root operation.
		op, err = st.getOperation(ctx, tx, operationID)
		if err != nil {
			return errors.Capture(err)
		}

		// Get all the tasks, parameters and logs for the given operation.
		tasksByOp, parametersByOp, taskLogsByOp, err := st.getFullTasksForOperation(ctx, tx, []string{op.UUID})
		if err != nil {
			return errors.Capture(err)
		}
		tasks = tasksByOp[op.UUID]
		parameters = parametersByOp[op.UUID]
		taskLogs = taskLogsByOp[op.UUID]

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
func (st *State) getFullTasksForOperation(ctx context.Context, tx *sqlair.TX, opUUIDs []string) (map[string][]taskResult, map[string][]taskParameter, map[string]map[string][]taskLogEntryByOperation, error) {
	// Get the operation parameters.
	parameters, err := st.getOperationParameters(ctx, tx, opUUIDs)
	if err != nil {
		return nil, nil, nil, errors.Capture(err)
	}

	// Get all tasks for this operation.
	tasks, err := st.getOperationTasks(ctx, tx, opUUIDs)
	if err != nil {
		return nil, nil, nil, errors.Capture(err)
	}

	// Get the task logs.
	logsByOperationAndTask, err := st.getTaskLogs(ctx, tx, opUUIDs)
	if err != nil {
		return nil, nil, nil, errors.Capture(err)
	}

	return tasks, parameters, logsByOperationAndTask, nil
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
	taskLogs map[string][]taskLogEntryByOperation,
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
		logs := taskLogs[t.TaskID]
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
func (st *State) buildOperationsQuery(params operation.QueryArgs) (string, []any) {
	// Define local typed slices for sqlair parameters
	type actionNames []string
	type statuses []string
	type applications []string
	type machines []string
	type units []string

	var args []any

	// Base query: selecting from operation.
	query := `
WITH
ops AS (
	SELECT *
	FROM   operation AS o
	LIMIT $queryParams.limit OFFSET $queryParams.offset
)
SELECT DISTINCT
       o.uuid AS &operationResult.uuid,
       o.operation_id AS &operationResult.operation_id,
       o.summary AS &operationResult.summary,
       o.enqueued_at AS &operationResult.enqueued_at,
       o.started_at AS &operationResult.started_at,
       o.completed_at AS &operationResult.completed_at
FROM ops AS o
`

	// Build WHERE clauses.
	whereClauses := []string{}
	joinClauses := []string{}

	// Check if we need task joins.
	needsTaskJoin := len(params.Status) > 0 ||
		len(params.Applications) > 0 ||
		len(params.Machines) > 0 ||
		len(params.Units) > 0

	// Add task joins if needed.
	if needsTaskJoin {
		joinClauses = append(joinClauses, "JOIN operation_task AS t ON o.uuid = t.operation_uuid")
	}

	// ActionNames filter - need to join with operation_action.
	if len(params.ActionNames) > 0 {
		joinClauses = append(joinClauses, "JOIN operation_action AS oa ON o.uuid = oa.operation_uuid")
		whereClauses = append(whereClauses, "oa.charm_action_key IN ($actionNames[:])")
		args = append(args, actionNames(params.ActionNames))
	}

	// Status filter - need to compute status from tasks.
	if len(params.Status) > 0 {
		statusStrings := make([]string, len(params.Status))
		for i, st := range params.Status {
			statusStrings[i] = st.String()
		}
		joinClauses = append(joinClauses, "JOIN operation_task_status AS ts ON t.uuid = ts.task_uuid")
		joinClauses = append(joinClauses, "JOIN operation_task_status_value AS sv ON ts.status_id = sv.id")
		whereClauses = append(whereClauses, "sv.status IN ($statuses[:])")
		args = append(args, statuses(statusStrings))
	}

	// Receivers filter - need to join with tasks and their targets.
	receiverClauses := []string{}

	if len(params.Applications) > 0 {
		joinClauses = append(joinClauses, "JOIN operation_unit_task AS ut ON t.uuid = ut.task_uuid")
		joinClauses = append(joinClauses, "JOIN unit AS u ON ut.unit_uuid = u.uuid")
		joinClauses = append(joinClauses, "JOIN application AS a ON u.application_uuid = a.uuid")
		receiverClauses = append(receiverClauses, "a.name IN ($applications[:])")
		args = append(args, applications(params.Applications))
	}

	if len(params.Machines) > 0 {
		joinClauses = append(joinClauses, "JOIN operation_machine_task mt ON t.uuid = mt.task_uuid")
		joinClauses = append(joinClauses, "JOIN machine m ON mt.machine_uuid = m.uuid")
		machineNames := make([]string, len(params.Machines))
		for i, name := range params.Machines {
			machineNames[i] = string(name)
		}
		receiverClauses = append(receiverClauses, "m.name IN ($machines[:])")
		args = append(args, machines(machineNames))
	}

	if len(params.Units) > 0 {
		// If we haven't already joined with unit tables for applications
		if len(params.Applications) == 0 {
			joinClauses = append(joinClauses, "JOIN operation_unit_task ut ON t.uuid = ut.task_uuid")
			joinClauses = append(joinClauses, "JOIN unit u ON ut.unit_uuid = u.uuid")
		}
		unitNames := make([]string, len(params.Units))
		for i, name := range params.Units {
			unitNames[i] = string(name)
		}
		receiverClauses = append(receiverClauses, "u.name IN ($units[:])")
		args = append(args, units(unitNames))
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
	query += "\nORDER BY o.operation_id"

	return query, args
}

// accumulateToMap transforms a slice of elements into a map of keys to slices
// of values using the provided transform function.
func accumulateToMap[F any, K comparable, V any](from []F, transform func(F) (K, V)) map[K][]V {
	to := make(map[K][]V)
	for _, oneFrom := range from {
		k, v := transform(oneFrom)
		to[k] = append(to[k], v)
	}
	return to
}

// accumulateToMapOfMap transforms a slice of elements into a map of keys to
// maps of keys to slices of values using the provided transform function.
func accumulateToMapOfMap[F any, K1 comparable, K2 comparable, V any](from []F, transform func(F) (K1, K2, V)) map[K1]map[K2][]V {
	to := make(map[K1]map[K2][]V)
	for _, oneFrom := range from {
		k1, k2, v := transform(oneFrom)
		if to[k1] == nil {
			to[k1] = make(map[K2][]V)
		}
		to[k1][k2] = append(to[k1][k2], v)
	}
	return to
}
