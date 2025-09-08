// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/clock"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/domain/operation"
)

// State describes the methods that a state implementation must provide to manage
// operation for a model.
type State interface {
	// GetAction returns the action identified by its task ID.
	// It returns the action as well as the path to its output in the object store,
	// if any. It's up to the caller to retrieve the actual output from the object
	// store.
	GetAction(ctx context.Context, actionUUID string) (operation.Action, string, error)
	// CancelAction attempts to cancel an enqueued action, identified by its
	// task ID.
	CancelAction(ctx context.Context, actionUUID string) (operation.Action, error)
}

// Service provides the API for managing operation
type Service struct {
	st                State
	clock             clock.Clock
	logger            logger.Logger
	objectStoreGetter objectstore.ModelObjectStoreGetter
}

// NewService returns a new Service for managing operation
func NewService(st State, clock clock.Clock, logger logger.Logger, objectStoreGetter objectstore.ModelObjectStoreGetter) *Service {
	return &Service{
		st:                st,
		clock:             clock,
		logger:            logger,
		objectStoreGetter: objectStoreGetter,
	}
}
