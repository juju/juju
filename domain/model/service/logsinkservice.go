// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	coremodel "github.com/juju/juju/core/model"
)

// LogSinkState is the model state required by the provide service. This is
// the model database state, not the controller state.
type LogSinkState interface {
	// GetModel returns a the model info.
	GetModel(context.Context) (coremodel.ModelInfo, error)
}

// LogSinkService defines a service for interacting with the underlying model
// state, as opposed to the controller state.
type LogSinkService struct {
	st LogSinkState
}

// NewLogSinkService returns a new Service for interacting with a models state.
func NewLogSinkService(st LogSinkState) *LogSinkService {
	return &LogSinkService{
		st: st,
	}
}

// Model returns model info for the current service.
//
// The following error types can be expected to be returned:
// - [modelerrors.NotFound]: When the model is not found for a given uuid.
func (s *LogSinkService) Model(ctx context.Context) (coremodel.ModelInfo, error) {
	return s.st.GetModel(ctx)
}
