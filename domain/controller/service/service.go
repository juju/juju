// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/trace"
)

// State defines an interface for interacting with the underlying state.
type State interface {
	ControllerModelUUID(ctx context.Context) (model.UUID, error)
}

// Service defines a service for interacting with the underlying state.
type Service struct {
	st State
}

// NewService returns a new Service for interacting with the underlying state.
func NewService(st State) *Service {
	return &Service{
		st: st,
	}
}

// ControllerModelUUID returns the model UUID of the controller model.
func (s *Service) ControllerModelUUID(ctx context.Context) (_ model.UUID, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	return s.st.ControllerModelUUID(ctx)
}
