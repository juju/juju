// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	coreaction "github.com/juju/juju/core/action"
	"github.com/juju/juju/domain/operation"
)

// GetActions returns the list of actions identified by their UUIDs.
func (s *Service) GetActions(ctx context.Context, actionUUIDs []coreaction.UUID) ([]operation.Action, error) {
	return nil, nil
}

// CancelAction attempts to cancel enqueued actions, identified by their
// UUIDs.
func (s *Service) CancelAction(ctx context.Context, actionUUIDs []coreaction.UUID) ([]operation.Action, error) {
	return nil, nil
}
