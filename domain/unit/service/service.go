// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/errors"
)

// State describes retrieval and persistence methods for units.
type State interface {
	// DeleteUnit deletes the input unit entity.
	DeleteUnit(context.Context, string) error
}

// Service provides the API for working with units.
type Service struct {
	st State
}

// NewService returns a new service reference wrapping the input state.
func NewService(st State) *Service {
	return &Service{
		st: st,
	}
}

// Delete deletes the specified unit.
func (s *Service) Delete(ctx context.Context, unitName string) error {
	err := s.st.DeleteUnit(ctx, unitName)
	return errors.Annotatef(err, "deleting unit %q", unitName)
}
