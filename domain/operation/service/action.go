// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/errors"
	"github.com/juju/juju/domain/operation"
	"github.com/juju/juju/internal/uuid"
)

// GetAction returns the action identified by its UUID.
func (s *Service) GetAction(ctx context.Context, actionUUID uuid.UUID) (operation.Action, error) {
	return operation.Action{}, errors.NotImplemented
}

// CancelAction attempts to cancel an enqueued action, identified by its
// UUID.
func (s *Service) CancelAction(ctx context.Context, actionUUID uuid.UUID) (operation.Action, error) {
	return operation.Action{}, errors.NotImplemented
}
