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

// statusOrder is a pre-defined order of statuses that we use to sort the
// each task status to get the overall status of an operation.
var statusOrder = []corestatus.Status{
	corestatus.Error,
	corestatus.Running,
	corestatus.Pending,
	corestatus.Failed,
	corestatus.Cancelled,
	corestatus.Completed,
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
		res.Operations[i].Status = operationStatus(tasks)
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
	res.Status = operationStatus(tasks)

	return res, nil
}

// operationStatus returns the status of an operation. The status is always
// computed on the fly, derived from the independent status of each task.
func operationStatus(tasks []operation.TaskInfo) corestatus.Status {
	// An operation with an empty set of tasks is pending.
	if len(tasks) == 0 {
		return corestatus.Pending
	}

	// First we create a set of all the actual task statuses.
	statusStats := set.NewStrings()
	for _, s := range tasks {
		statusStats.Add(s.Status.String())
	}
	// Then we check each one in the order of the pre-defined status.
	for _, status := range statusOrder {
		if statusStats.Contains(status.String()) {
			return status
		}
	}

	// If the operation has at least one task, one status should match, but if
	// it doesn't we return pending.
	return corestatus.Pending
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
