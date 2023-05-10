// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/errors"
)

// State defines a interface for interacting with the underlying state.
type State interface {
	Create(context.Context, UUID) error
	Delete(context.Context, UUID) error
}

// Service defines a service for interacting with the underlying state.
type Service struct {
	st State
}

// NewService returns a new Service for interacting with the underlying state.
func NewService(st State) *Service {
	return &Service{
		st: st,
	}
}

// Create takes a model UUID and creates a new model.
func (s *Service) Create(ctx context.Context, uuid UUID) error {
	if err := uuid.Validate(); err != nil {
		return errors.Annotatef(err, "validating model uuid %q", uuid)
	}

	err := s.st.Create(ctx, uuid)
	return errors.Annotatef(err, "creating model %q", uuid)
}

// Delete takes a model UUID and deletes the model if it exists.
func (s *Service) Delete(ctx context.Context, uuid UUID) error {
	if err := uuid.Validate(); err != nil {
		return errors.Annotatef(err, "validating model uuid %q", uuid)
	}

	err := s.st.Delete(ctx, uuid)
	return errors.Annotatef(err, "deleting model %q", uuid)
}
