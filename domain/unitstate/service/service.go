// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/trace"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/unitstate"
)

// State defines an interface for interacting with the underlying state.
type State interface {
	// SetUnitState persists the input unit state selectively,
	// based on its populated values.
	SetUnitState(context.Context, unitstate.UnitState) error

	// GetUnitState returns the full unit agent state.
	// If no unit with the uuid exists, a [unitstateerrors.UnitNotFound] error
	// is returned.
	// If the units state is empty [unitstateerrors.EmptyUnitState] error is
	// returned.
	GetUnitState(context.Context, coreunit.Name) (unitstate.RetrievedUnitState, error)
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

// SetState persists the input unit state selectively,
// based on its populated values.
func (s *Service) SetState(ctx context.Context, as unitstate.UnitState) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.st.SetUnitState(ctx, as)
}

// GetState returns the full unit state. The state may be empty.
func (s *Service) GetState(ctx context.Context, name coreunit.Name) (unitstate.RetrievedUnitState, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	state, err := s.st.GetUnitState(ctx, name)
	if err != nil {
		return unitstate.RetrievedUnitState{}, err
	}
	return state, nil
}
