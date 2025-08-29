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

// GetOperationsByIDs returns a list of specified operations, identified by their IDs.
func (s *Service) GetOperationsByIDs(ctx context.Context, operationIDs []string) (operation.QueryResult, error) {
	return operation.QueryResult{}, errors.New("actions in Dqlite not supported")
}
