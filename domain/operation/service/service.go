// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/clock"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/domain/operation"
)

// State describes the methods that a state implementation must provide to manage
// operation for a model.
type State interface {
	// GetAction returns the action identified by its UUID.
	GetAction(ctx context.Context, actionUUID string) (operation.Action, error)
	// CancelAction attempts to cancel an enqueued action, identified by its
	// UUID.
	CancelAction(ctx context.Context, actionUUID string) (operation.Action, error)
}

// Service provides the API for managing operation
type Service struct {
	st     State
	clock  clock.Clock
	logger logger.Logger
}

// NewService returns a new Service for managing operation
func NewService(st State, clock clock.Clock, logger logger.Logger) *Service {
	return &Service{
		st:     st,
		clock:  clock,
		logger: logger,
	}
}
