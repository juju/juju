// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/collections/transform"
	"github.com/juju/description/v10"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/domain/operation"
	"github.com/juju/juju/domain/operation/service"
	"github.com/juju/juju/domain/operation/state"
	"github.com/juju/juju/internal/errors"
)

// RegisterExport registers the export operations with the given coordinator.
func RegisterExport(coordinator Coordinator,
	objectStoreGetter objectstore.ModelObjectStoreGetter,
	clock clock.Clock,
	logger logger.Logger) {
	coordinator.Add(&exportOperation{
		logger:            logger,
		clock:             clock,
		objectStoreGetter: objectStoreGetter})
}

// ExportService provides a subset of the operation domain service
// methods needed for export.
type ExportService interface {
	// GetOperations returns a list of operations on specified entities, filtered by the
	// given parameters.
	GetOperations(ctx context.Context, params operation.QueryArgs) (operation.QueryResult, error)
}

// exportOperation describes the export process for the operation domain.
type exportOperation struct {
	modelmigration.BaseOperation

	// injected dependencies.
	objectStoreGetter objectstore.ModelObjectStoreGetter
	clock             clock.Clock
	logger            logger.Logger

	// initialized during Setup.
	exportService ExportService
}

// Name returns the name of this operation.
func (e *exportOperation) Name() string {
	return "export operations"
}

// Setup implements Operation.
func (e *exportOperation) Setup(scope modelmigration.Scope) error {
	e.exportService = service.NewService(
		state.NewState(scope.ModelDB(), e.clock, e.logger),
		e.clock,
		e.logger,
		e.objectStoreGetter,
		nil) // No leadership service needed for export.
	return nil
}

// Execute performs the export.
func (e *exportOperation) Execute(ctx context.Context, model description.Model) error {
	// Get all operations.
	ops, err := e.getAllOperations(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	for _, op := range ops {
		e.exportOperation(ctx, model, op)
	}

	return nil
}

// getAllOperations returns all operations, handling pagination.
func (e *exportOperation) getAllOperations(ctx context.Context) ([]operation.OperationInfo, error) {
	var result []operation.OperationInfo
	for {
		ops, err := e.exportService.GetOperations(ctx, operation.QueryArgs{Offset: ptr(len(result))})
		if err != nil {
			return nil, errors.Errorf("getting all operations from offset %d: %w", len(result), err)
		}
		result = append(result, ops.Operations...)
		if !ops.Truncated {
			break
		}
	}
	return result, nil
}

// exportOperation exports an operation to the given model.
func (e *exportOperation) exportOperation(ctx context.Context,
	model description.Model,
	op operation.OperationInfo,
) {
	current := e.toDescriptionOperation(op)
	for _, task := range op.Machines {
		model.AddAction(e.toDescriptionAction(&current, task.ReceiverName.String(), task.TaskInfo))
	}
	for _, task := range op.Units {
		model.AddAction(e.toDescriptionAction(&current, task.ReceiverName.String(), task.TaskInfo))
	}

	model.AddOperation(current)
}

// toDescriptionOperation converts an operation to a description.OperationArgs.
func (e *exportOperation) toDescriptionOperation(op operation.OperationInfo) description.OperationArgs {
	return description.OperationArgs{
		Id:        op.OperationID,
		Summary:   op.Summary,
		Enqueued:  op.Enqueued,
		Started:   op.Started,
		Completed: op.Completed,
		Status:    op.Status.String(),

		// the task counts will be increased while looping through the tasks.
		CompleteTaskCount: 0,
		SpawnedTaskCount:  0,
	}
}

// toDescriptionAction converts a task to a description.ActionArgs. It also increments
// the task counts in the operation.
func (e *exportOperation) toDescriptionAction(
	op *description.OperationArgs,
	receiverName string,
	info operation.TaskInfo,
) description.ActionArgs {
	op.SpawnedTaskCount++
	if !info.Completed.IsZero() {
		op.CompleteTaskCount++
	}
	return description.ActionArgs{
		Id:             info.ID,
		Receiver:       receiverName,
		Name:           info.ActionName,
		Operation:      op.Id,
		Parameters:     info.Parameters,
		Parallel:       info.IsParallel,
		ExecutionGroup: zeroNilPtr(info.ExecutionGroup),
		Enqueued:       info.Enqueued,
		Started:        info.Started,
		Completed:      info.Completed,
		Status:         info.Status.String(),
		Message:        info.Message,
		Results:        info.Output,
		Messages:       transform.Slice(info.Log, toMessage),
	}
}

// taskMessage is a description.ActionMessage implementation for operation.TaskLog.
type taskMessage struct {
	operation.TaskLog
}

// Timestamp implements description.ActionMessage.
func (t taskMessage) Timestamp() time.Time {
	return t.TaskLog.Timestamp
}

// Message implements description.ActionMessage.
func (t taskMessage) Message() string {
	return t.TaskLog.Message
}

// toMessage converts a task log to a description.ActionMessage.
func toMessage(log operation.TaskLog) description.ActionMessage {
	return taskMessage{
		TaskLog: log,
	}
}

// zeroNilPtr returns the zero value of v if v is nil, otherwise it returns v.
func zeroNilPtr[T any](v *T) T {
	var zero T
	if v == nil {
		return zero
	}
	return *v
}

// ptr returns a pointer to v.
func ptr[T any](v T) *T {
	return &v
}
