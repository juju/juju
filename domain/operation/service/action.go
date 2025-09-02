// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	coreaction "github.com/juju/juju/core/action"
	"github.com/juju/juju/domain/operation"
)

// GetAction returns the action identified by its UUID.
func (s *Service) GetAction(ctx context.Context, actionUUID coreaction.UUID) (operation.Action, error) {
	return operation.Action{}, nil
}

// CancelAction attempts to cancel an enqueued action, identified by its
// UUID.
func (s *Service) CancelAction(ctx context.Context, actionUUID coreaction.UUID) (operation.Action, error) {
	return operation.Action{}, nil
}
