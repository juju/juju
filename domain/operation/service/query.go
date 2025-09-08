// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

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
