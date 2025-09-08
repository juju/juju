// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	coreoperation "github.com/juju/juju/core/operation"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain/operation"
	"github.com/juju/juju/internal/errors"
)

// GetAction returns the action identified by its ID.
func (s *Service) GetAction(ctx context.Context, actionID coreoperation.ID) (operation.Action, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	action, err := s.st.GetAction(ctx, actionID.String())
	if err != nil {
		return operation.Action{}, errors.Errorf("retrieving action %q: %w", actionID, err)
	}

	return action, nil
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
