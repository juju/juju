// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/errors"
)

// LeadershipState is an interface that provides methods to get the application
// leadership.
type LeadershipState interface {
	// GetApplicationLeadershipForModel returns the leadership information for
	// the model applications
	GetApplicationLeadershipForModel(ctx context.Context, modelUUID model.UUID) (map[string]string, error)
}

// LeadershipService provides the lease read capabilities.
type LeadershipService struct {
	st LeadershipState
}

// NewLeadershipService creates a new LeadershipService.
func NewLeadershipService(st LeadershipState) *LeadershipService {
	return &LeadershipService{
		st: st,
	}
}

// GetApplicationLeadershipForModel returns the leadership information for the
// model applications.
func (s *LeadershipService) GetApplicationLeadershipForModel(ctx context.Context, modelUUID model.UUID) (map[string]string, error) {
	if err := modelUUID.Validate(); err != nil {
		return nil, errors.Capture(err)
	}
	return s.st.GetApplicationLeadershipForModel(ctx, modelUUID)
}
