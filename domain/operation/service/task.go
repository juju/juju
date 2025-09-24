// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"encoding/json"
	"io"
	"strings"

	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain/operation"
	operationerrors "github.com/juju/juju/domain/operation/errors"
	"github.com/juju/juju/domain/operation/internal"
	"github.com/juju/juju/internal/errors"
)

// StartTask marks a task as running and logs the time it was started.
func (s *Service) StartTask(ctx context.Context, taskID string) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc(),
		trace.WithAttributes(
			trace.StringAttr("task.id", taskID),
		))
	defer span.End()

	return s.st.StartTask(ctx, taskID)
}

// FinishTask saves the result of a completed task.
func (s *Service) FinishTask(ctx context.Context, result operation.CompletedTaskResult) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc(),
		trace.WithAttributes(
			trace.StringAttr("task.id", result.TaskID),
		))
	defer span.End()

	if err := result.Validate(); err != nil {
		return errors.Capture(err)
	}

	// Use the task's UUID as the path in the object store for
	// it's results
	taskUUID, err := s.st.GetTaskUUIDByID(ctx, result.TaskID)
	if err != nil {
		return errors.Capture(err)
	}

	storeUUID, removeResultsFromStore, err := s.storeTaskResults(ctx, taskUUID, result.Results)
	if err != nil {
		return errors.Errorf("putting task result %q in store: %w", result.TaskID, err)
	}

	defer func() {
		if err != nil && removeResultsFromStore != nil {
			removeResultsFromStore()
		}
	}()

	err = s.st.FinishTask(ctx, internal.CompletedTask{
		TaskUUID:  taskUUID,
		StoreUUID: storeUUID,
		Status:    result.Status,
		Message:   result.Message,
	})
	return errors.Capture(err)
}

func (s *Service) storeTaskResults(ctx context.Context, taskUUID string, results map[string]interface{}) (string, func(), error) {
	if results == nil {
		return "", nil, nil
	}

	object, err := json.Marshal(results)
	if err != nil {
		return "", nil, errors.Errorf("failed to serialize results: %w", err)
	}

	// Save output
	store, err := s.objectStoreGetter.GetObjectStore(ctx)
	if err != nil {
		return "", nil, errors.Errorf("getting object store: %w", err)
	}

	size := int64(len(object))
	reader := strings.NewReader(string(object))
	storeUUID, err := store.Put(
		ctx,
		taskUUID,
		reader,
		size,
	)
	if err != nil {
		return "", nil, errors.Errorf("failed to store results: %w", err)
	}

	removeResultsFromStore := func() {
		rErr := store.Remove(ctx, taskUUID)
		if rErr != nil {
			s.logger.Errorf(ctx, "removing task result %s from store: %w", rErr)
		}
	}

	return storeUUID.String(), removeResultsFromStore, nil
}

// GetReceiverFromTaskID returns a receiver string for the task identified.
// The string should satisfy the ActionReceiverTag type.
func (s *Service) GetReceiverFromTaskID(ctx context.Context, taskID string) (string, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc(),
		trace.WithAttributes(
			trace.StringAttr("task.id", taskID),
		))
	defer span.End()

	receiver, err := s.st.GetReceiverFromTaskID(ctx, taskID)
	return receiver, errors.Capture(err)
}

// GetPendingTaskByTaskID return a struct containing the data required to
// run a task. The task must have a status of pending.
// The following errors may be returned:
// - [operationerrors.TaskNotPending] if the task exists but does not have
// a pending status.
func (s *Service) GetPendingTaskByTaskID(ctx context.Context, taskID string) (operation.TaskArgs, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc(),
		trace.WithAttributes(
			trace.StringAttr("task.id", taskID),
		))
	defer span.End()

	task, _, err := s.st.GetTask(ctx, taskID)
	if err != nil {
		return operation.TaskArgs{}, errors.Errorf("getting pending task %q: %w", taskID, err)
	}
	if task.Status != status.Pending {
		return operation.TaskArgs{}, errors.Errorf("task %q: %w", taskID, operationerrors.TaskNotPending)
	}

	retVal := operation.TaskArgs{
		ActionName:     task.ActionName,
		ExecutionGroup: deptr(task.ExecutionGroup),
		IsParallel:     task.IsParallel,
		Parameters:     task.Parameters,
	}
	return retVal, nil
}

// GetTask returns the task identified by its ID.
func (s *Service) GetTask(ctx context.Context, taskID string) (operation.Task, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc(),
		trace.WithAttributes(
			trace.StringAttr("task.id", taskID),
		))
	defer span.End()

	task, outputPath, err := s.st.GetTask(ctx, taskID)
	if err != nil {
		return operation.Task{}, errors.Errorf("retrieving task %q: %w", taskID, err)
	}

	if outputPath != nil {
		// Read output from object store
		output, err := s.readTaskOutput(ctx, *outputPath)
		if err != nil {
			return operation.Task{}, errors.Errorf("reading task output %q: %w", taskID, err)
		}
		task.Output = output
	}

	return task, nil
}

// readTaskOutput reads and unmarshals task output from the object store.
func (s *Service) readTaskOutput(ctx context.Context, path string) (map[string]any, error) {
	objectStore, err := s.objectStoreGetter.GetObjectStore(ctx)
	if err != nil {
		return nil, errors.Errorf("getting object store: %w", err)
	}

	reader, _, err := objectStore.Get(ctx, path)
	if err != nil {
		return nil, errors.Errorf("reading task output from object store at path %q: %w", path, err)
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, errors.Errorf("reading task output data: %w", err)
	}

	// Unmarshal JSON data into map[string]any.
	var outputData map[string]any
	if err := json.Unmarshal(data, &outputData); err != nil {
		return nil, errors.Errorf("unmarshaling task output: %w", err)
	}

	return outputData, nil
}

// CancelTask attempts to cancel an enqueued task, identified by its
// ID.
func (s *Service) CancelTask(ctx context.Context, taskID string) (operation.Task, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc(),
		trace.WithAttributes(
			trace.StringAttr("task.id", taskID),
		))
	defer span.End()

	task, err := s.st.CancelTask(ctx, taskID)
	if err != nil {
		return operation.Task{}, errors.Errorf("cancelling task %q: %w", taskID, err)
	}

	return task, nil
}

// LogTaskMessage stores the message for the given task ID.
func (s *Service) LogTaskMessage(ctx context.Context, taskID, message string) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc(),
		trace.WithAttributes(
			trace.StringAttr("task.id", taskID),
		))
	defer span.End()

	return errors.Capture(s.st.LogTaskMessage(ctx, taskID, message))
}

// GetTaskStatusByID returns the status of the given task.
func (s *Service) GetTaskStatusByID(ctx context.Context, taskID string) (string, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc(),
		trace.WithAttributes(
			trace.StringAttr("task.id", taskID),
		))
	defer span.End()

	status, err := s.st.GetTaskStatusByID(ctx, taskID)
	if err != nil {
		return "", errors.Errorf("retrieving task status %q: %w", taskID, err)
	}
	return status, nil
}

func deptr[T any](v *T) T {
	var zero T
	if v == nil {
		return zero
	}
	return *v
}
