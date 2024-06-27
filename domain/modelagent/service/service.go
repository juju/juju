// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/version/v2"

	"github.com/juju/juju/core/model"
)

// State provides the state methods needed by the modelagent service.
type State interface {
	// GetModelAgentVersion returns the agent version for the specified model.
	GetModelAgentVersion(ctx context.Context, modelID model.UUID) (version.Number, error)
}

// Service is a modelagent service which can be used to get the running Juju
// agent version for any given model.
type Service struct {
	st State
}

// NewService returns a new modelagent service.
func NewService(st State) *Service {
	return &Service{st: st}
}

// GetModelAgentVersion returns the agent version for the specified model.
// The following errors can be returned:
//   - [errors.NotValid] if the model ID is not valid;
//   - [github.com/juju/juju/domain/model/errors.NotFound] if no model exists
//     for the provided ID.
func (s *Service) GetModelAgentVersion(ctx context.Context, modelID model.UUID) (version.Number, error) {
	if err := modelID.Validate(); err != nil {
		return version.Zero, errors.Annotate(err, "validating model ID")
	}
	return s.st.GetModelAgentVersion(ctx, modelID)
}

// ModelService is a modelagent service which can be used to get the running
// Juju agent version for the current model.
type ModelService struct {
	modelID model.UUID
	*Service
}

// NewModelService returns a new modelagent service scoped to a single model.
func NewModelService(st State, modelID model.UUID) *ModelService {
	return &ModelService{
		modelID: modelID,
		Service: NewService(st),
	}
}

// GetModelAgentVersion returns the agent version for the current model.
// The following errors can be returned:
//   - [errors.NotValid] if the model ID is not valid;
//   - [github.com/juju/juju/domain/model/errors.NotFound] if no model exists
//     for the provided ID.
func (s *ModelService) GetModelAgentVersion(ctx context.Context) (version.Number, error) {
	return s.Service.GetModelAgentVersion(ctx, s.modelID)
}
