// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"encoding/json"
	"io"

	coreoperation "github.com/juju/juju/core/operation"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain/operation"
	"github.com/juju/juju/internal/errors"
)

// GetAction returns the action identified by its ID.
func (s *Service) GetAction(ctx context.Context, actionID coreoperation.ID) (operation.Action, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	action, outputPath, err := s.st.GetAction(ctx, actionID.String())
	if err != nil {
		return operation.Action{}, errors.Errorf("retrieving action %q: %w", actionID, err)
	}

	if outputPath != "" {
		// Read output from object store
		output, err := s.readTaskOutput(ctx, outputPath)
		if err != nil {
			return operation.Action{}, errors.Errorf("reading task output %q: %w", actionID, err)
		}
		action.Output = output
	}

	return action, nil
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

// CancelAction attempts to cancel an enqueued action, identified by its ID.
func (s *Service) CancelAction(ctx context.Context, actionID coreoperation.ID) (operation.Action, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	action, err := s.st.CancelAction(ctx, actionID.String())
	if err != nil {
		return operation.Action{}, errors.Errorf("cancelling action %q: %w", actionID, err)
	}

	return action, nil
}
