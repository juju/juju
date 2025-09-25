// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	coreerrors "github.com/juju/juju/core/errors"
	coremachine "github.com/juju/juju/core/machine"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/domain/operation"
	"github.com/juju/juju/internal/errors"
)

// GetOperations returns a list of operations on specified entities, filtered by the
// given parameters.
func (s *Service) GetOperations(ctx context.Context, params operation.QueryArgs) (operation.QueryResult, error) {
	return operation.QueryResult{}, errors.New("actions in Dqlite not supported")
}

// GetOperationByID returns an operation by its ID.
func (s *Service) GetOperationByID(ctx context.Context, operationID string) (operation.OperationInfo, error) {
	return operation.OperationInfo{}, errors.New("actions in Dqlite not supported")
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
