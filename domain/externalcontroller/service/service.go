// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/core/crossmodel"
)

// State describes retrieval and persistence methods for storage.
type State interface {
	// Controller returns the controller record.
	Controller(ctx context.Context, controllerUUID string) (*crossmodel.ControllerInfo, error)

	// UpdateExternalController persists the input controller
	// record and associates it with the input model UUIDs.
	UpdateExternalController(ctx context.Context, ec crossmodel.ControllerInfo, modelUUIDs []string) error
}

// Service provides the API for working with external controllers.
type Service struct {
	st State
}

// NewService returns a new service reference wrapping the input state.
func NewService(st State) *Service {
	return &Service{st}
}

// Controller returns the controller record.
func (s *Service) Controller(
	ctx context.Context,
	controllerUUID string,
) (*crossmodel.ControllerInfo, error) {

	return nil, nil
}

// UpdateExternalController persists the input controller
// record and associates it with the input model UUIDs.
func (s *Service) UpdateExternalController(
	ctx context.Context, ec crossmodel.ControllerInfo, modelUUIDs ...string,
) error {
	err := s.st.UpdateExternalController(ctx, ec, modelUUIDs)
	return errors.Annotate(err, "updating external controller state")
}
