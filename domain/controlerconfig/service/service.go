// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"github.com/juju/errors"

	"github.com/juju/juju/domain"
)

// State defines an interface for interacting with the underlying state.
type State interface {
	Add(context.Context, string, string) error
	Delete(context.Context, string) error
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

// Add adds a controller config key/value pair.
func (s *Service) Add(ctx context.Context, key string, value string) error {

	err := s.st.Add(ctx, key, value)
	return errors.Annotatef(domain.CoerceError(err), "adding controller config key %q", key)
}

// Delete deletes a controller config key/value pair.
func (s *Service) Delete(ctx context.Context, key string) error {
	err := s.st.Delete(ctx, key)
	return errors.Annotatef(domain.CoerceError(err), "deleting controller config key %q", key)
}
