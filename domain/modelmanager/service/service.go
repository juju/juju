// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/domain"
)

// State defines a interface for interacting with the underlying state.
type State interface {
	Create(context.Context, UUID) error
	Delete(context.Context, UUID) error
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
func (s *Service) Create(ctx context.Context, uuid UUID) error {
	if err := uuid.Validate(); err != nil {
		return errors.Annotatef(err, "validating model uuid %q", uuid)
	}

	err := s.st.Create(ctx, uuid)
	return errors.Annotatef(domain.CoerceError(err), "creating model %q", uuid)
}

// Delete takes a model UUID and deletes the model if it exists.
func (s *Service) Delete(ctx context.Context, uuid UUID) error {
	if err := uuid.Validate(); err != nil {
		return errors.Annotatef(err, "validating model uuid %q", uuid)
	}

	// Deletion of the model in state should prevent any future requests to
	// acquire the tracked db from the db manager.
	if err := s.st.Delete(ctx, uuid); err != nil {
		if errors.Is(err, domain.ErrNoRecord) {
			return nil
		}
		return errors.Annotatef(domain.CoerceError(err), "deleting model %q", uuid)
	}

	err := s.dbDeleter.DeleteDB(uuid.String())
	return errors.Annotatef(err, "stopping model %q", uuid)
}
