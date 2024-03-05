// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/errors"
)

// State describes retrieval and persistence methods for storage.
type State interface {
	SetFlag(ctx context.Context, flag string, value bool, description string) error
	GetFlag(context.Context, string) (bool, error)
}

// Service provides the API for working with external controllers.
type Service struct {
	st State
}

// NewService returns a new service reference wrapping the input state.
func NewService(st State) *Service {
	return &Service{
		st: st,
	}
}

// SetFlag sets the value of a flag.
// Description is used to describe the flag and it's potential state.
func (s *Service) SetFlag(ctx context.Context, flag string, value bool, description string) error {
	return s.st.SetFlag(ctx, flag, value, description)
}

// GetFlag returns the value of a flag.
func (s *Service) GetFlag(ctx context.Context, flag string) (bool, error) {
	value, err := s.st.GetFlag(ctx, flag)
	if err != nil && !errors.Is(err, errors.NotFound) {
		return false, err
	}
	return value, nil
}
