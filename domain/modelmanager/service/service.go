// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/core/database"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/domain"
)

// State defines a interface for interacting with the underlying state.
type State interface {
	// Create takes a model UUID and creates a new model.
	Create(context.Context, coremodel.UUID) error
}

// Service defines a service for interacting with the underlying state.
type Service struct {
	st        State
	dbDeleter database.DBDeleter
}

// NewService returns a new Service for interacting with the underlying state.
func NewService(st State, dbDeleter database.DBDeleter) *Service {
	return &Service{
		st:        st,
		dbDeleter: dbDeleter,
	}
}

// Create takes a model UUID and creates a new model.
func (s *Service) Create(ctx context.Context, uuid coremodel.UUID) error {
	if err := uuid.Validate(); err != nil {
		return errors.Annotatef(err, "validating model uuid %q", uuid)
	}

	err := s.st.Create(ctx, uuid)
	return errors.Annotatef(domain.CoerceError(err), "creating model %q", uuid)
}
