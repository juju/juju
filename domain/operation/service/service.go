// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/clock"

	"github.com/juju/juju/core/logger"
)

// State describes the methods that a state implementation must provide to manage
// operation for a model.
type State interface{}

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
