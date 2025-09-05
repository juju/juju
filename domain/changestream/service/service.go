// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain/changestream"
)

// State defines an interface for interacting with the underlying state.
type State interface {
	// Prune prunes the change log up to the lowest watermark across all
	// controllers. It returns the number of rows pruned.
	Prune(ctx context.Context, currentWindow changestream.Window) (changestream.Window, int64, error)
}

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

// Prune prunes the change log up to the lowest watermark across all
// controllers for the model.
func (s *Service) Prune(ctx context.Context, currentWindow changestream.Window) (changestream.Window, int64, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.st.Prune(ctx, currentWindow)
}
