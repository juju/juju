// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain/operation"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// GetAction returns the action identified by its UUID.
func (s *Service) GetAction(ctx context.Context, actionUUID uuid.UUID) (operation.Action, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	action, err := s.st.GetAction(ctx, actionUUID.String())
	if err != nil {
		return operation.Action{}, errors.Errorf("retrieving action %q: %w", actionUUID, err)
	}

	return action, nil
}

// CancelAction attempts to cancel an enqueued action, identified by its
// UUID.
func (s *Service) CancelAction(ctx context.Context, actionUUID uuid.UUID) (operation.Action, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	action, err := s.st.CancelAction(ctx, actionUUID.String())
	if err != nil {
		return operation.Action{}, errors.Errorf("cancelling action %q: %w", actionUUID, err)
	}

	return action, nil
}
