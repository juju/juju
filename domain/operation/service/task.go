// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"encoding/json"
	"io"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/trace"
	coretrace "github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain/operation"
	"github.com/juju/juju/internal/errors"
)

// StartTask marks a task as running and logs the time it was started.
func (s *Service) StartTask(ctx context.Context, id string) error {
	return coreerrors.NotImplemented
}

// FinishTask saves the result of a completed task.
func (s *Service) FinishTask(ctx context.Context, result operation.CompletedTaskResult) error {
	return coreerrors.NotImplemented
}

// ReceiverFromTask returns a receiver string for the task identified.
// The string should satisfy the ActionReceiverTag type.
func (s *Service) ReceiverFromTask(ctx context.Context, id string) (string, error) {
	return "", coreerrors.NotImplemented
}

// GetTask returns the task identified by its ID.
func (s *Service) GetTask(ctx context.Context, taskID string) (operation.TaskInfo, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc(),
		coretrace.WithAttributes(
			trace.StringAttr("task.id", taskID),
		))
	defer span.End()

	task, outputPath, err := s.st.GetTask(ctx, taskID)
	if err != nil {
		return operation.TaskInfo{}, errors.Errorf("retrieving task %q: %w", taskID, err)
	}

	if outputPath != nil {
		// Read output from object store
		output, err := s.readTaskOutput(ctx, *outputPath)
		if err != nil {
			return operation.TaskInfo{}, errors.Errorf("reading task output %q: %w", taskID, err)
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
func (s *Service) CancelTask(ctx context.Context, taskID string) (operation.TaskInfo, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc(),
		coretrace.WithAttributes(
			trace.StringAttr("task.id", taskID),
		))
	defer span.End()

	task, err := s.st.CancelTask(ctx, taskID)
	if err != nil {
		return operation.TaskInfo{}, errors.Errorf("cancelling task %q: %w", taskID, err)
	}

	return task, nil
}
