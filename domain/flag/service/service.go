// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
)

// State describes retrieval and persistence methods for storage.
type State interface {
	SetFlag(context.Context, string, bool) error
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
func (s *Service) SetFlag(ctx context.Context, flag string, value bool) error {
	return s.st.SetFlag(ctx, flag, value)
}

// GetFlag returns the value of a flag.
func (s *Service) GetFlag(ctx context.Context, flag string) (bool, error) {
	return s.st.GetFlag(ctx, flag)
}
