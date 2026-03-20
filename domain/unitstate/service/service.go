// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/logger"
)

// Service defines a service for interacting with the underlying state.
type Service struct {
	st     State
	logger logger.Logger
}

// NewService returns a new Service for interacting with the underlying state.
func NewService(st State, logger logger.Logger) *Service {
	return &Service{
		st:     st,
		logger: logger,
	}
}

// LeadershipService provides the API for working with unit's state and
// persisting commit hook changes, including those that require leadership
// checks.
type LeadershipService struct {
	*Service
	leaderEnsurer leadership.Ensurer
	logger        logger.Logger
}

// NewLeadershipService returns a new LeadershipService for working with
// the underlying state.
func NewLeadershipService(
	st State,
	leaderEnsurer leadership.Ensurer,
	logger logger.Logger,
) *LeadershipService {
	return &LeadershipService{
		Service:       NewService(st, logger),
		leaderEnsurer: leaderEnsurer,
		logger:        logger,
	}
}
