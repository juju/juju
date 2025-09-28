// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/collections/set"

	coreerrors "github.com/juju/juju/core/errors"
	coremachine "github.com/juju/juju/core/machine"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain/operation"
	"github.com/juju/juju/internal/errors"
)

// statusCompletedOrder is a pre-defined order of statuses that we use to sort
// the each task status to get the overall status of an operation.
var statusCompletedOrder = []corestatus.Status{
	corestatus.Error,
	corestatus.Failed,
	corestatus.Cancelled,
	corestatus.Completed,
}

// statusOrder is a pre-defined order of statuses that we use to sort the
// each task status to get the overall status of an operation.
var statusActiveOrder = []corestatus.Status{
	corestatus.Running,
	corestatus.Aborting,
	corestatus.Pending,
}

// GetOperations returns a list of operations on specified entities, filtered by the
// given parameters.
func (s *Service) GetOperations(ctx context.Context, params operation.QueryArgs) (operation.QueryResult, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	s.logger.Debugf(ctx, "querying operations with params: %v", params)
	res, err := s.st.GetOperations(ctx, params)
	if err != nil {
		return operation.QueryResult{}, errors.Capture(err)
	}
	// Now we must deduce the status each operation.
	for i, op := range res.Operations {
		tasks := make([]operation.TaskInfo, 0, len(op.Units)+len(op.Machines))
		for _, u := range op.Units {
			tasks = append(tasks, u.TaskInfo)
		}
		for _, m := range op.Machines {
			tasks = append(tasks, m.TaskInfo)
		}
		status, err := operationStatus(tasks)
		if err != nil {
			// This is a programming error, since all tasks should have a known
			// status. We don't block though, set it to pending and continue.
			status = corestatus.Pending
			s.logger.Errorf(ctx, "getting status of operation %q: %w", op.OperationID, err)
		}
		res.Operations[i].Status = status
	}

	return res, nil
}

// GetOperationByID returns an operation by its ID.
func (s *Service) GetOperationByID(ctx context.Context, operationID string) (operation.OperationInfo, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	res, err := s.st.GetOperationByID(ctx, operationID)
	if err != nil {
		return operation.OperationInfo{}, errors.Capture(err)
	}
	// Now we must deduce the status of the operation.
	tasks := make([]operation.TaskInfo, 0, len(res.Units)+len(res.Machines))
	for _, u := range res.Units {
		tasks = append(tasks, u.TaskInfo)
	}
	for _, m := range res.Machines {
		tasks = append(tasks, m.TaskInfo)
	}
	status, err := operationStatus(tasks)
	if err != nil {
		// This is a programming error, since all tasks should have a known
		// status. We don't block though, set it to pending and continue.
		status = corestatus.Pending
		s.logger.Errorf(ctx, "getting status of operation %q: %w", operationID, err)
	}
	res.Status = status

	return res, nil
}

// operationStatus returns the status of an operation. The status is always
// computed on the fly, derived from the independent status of each task.
func operationStatus(tasks []operation.TaskInfo) (corestatus.Status, error) {
	// An operation with an empty set of tasks is error status.
	if len(tasks) == 0 {
		return corestatus.Error, nil
	}

	// First we create a set of all the actual task statuses.
	statusStats := set.NewStrings()
	for _, s := range tasks {
		statusStats.Add(s.Status.String())
	}
	// First check the tasks for active statuses.
	for _, status := range statusActiveOrder {
		if statusStats.Contains(status.String()) {
			return status, nil
		}
	}
	// If no active statuses, check for completed statuses.
	for _, status := range statusCompletedOrder {
		if statusStats.Contains(status.String()) {
			return status, nil
		}
	}

	// If we get here, all tasks are in unknown status. This is a programming
	// error, one which the DDL should protect us against.
	return "", errors.Errorf("unknown status")
}

// GetMachineTaskIDsWithStatus retrieves the task IDs for a specific machine name and status.
func (s *Service) GetMachineTaskIDsWithStatus(ctx context.Context,
	name coremachine.Name, wantedStatus corestatus.Status) ([]string, error) {

	if !wantedStatus.KnownTaskStatus() {
		return nil, errors.Errorf("unknown task status %q", wantedStatus).Add(coreerrors.NotValid)
	}

	if err := name.Validate(); err != nil {
		return nil, errors.Capture(err)
	}

	ids, err := s.st.GetMachineTaskIDsWithStatus(ctx, name.String(), wantedStatus.String())
	if err != nil {
		return nil, errors.Capture(err)
	}

	return ids, nil
}
