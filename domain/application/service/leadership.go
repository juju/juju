// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
)

// LeadershipState is an interface that provides methods to get the application
// leadership.
type LeadershipState interface {
	// GetApplicationLeadership returns the leadership information for the
	// model applications
	GetApplicationLeadership(ctx context.Context) (map[string]string, error)
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

// GetApplicationLeadership returns the leadership information for the model
// applications.
func (s *LeadershipService) GetApplicationLeadership(ctx context.Context) (map[string]string, error) {
	return s.st.GetApplicationLeadership(ctx)
}
