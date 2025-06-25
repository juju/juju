// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import "github.com/juju/juju/core/logger"

// Service provides the API for working with the network domain.
type Service struct {
	st     State
	logger logger.Logger
}

// NewService returns a new service reference wrapping the input state.
func NewService(st State, logger logger.Logger) *Service {
	return &Service{
		st:     st,
		logger: logger,
	}
}
