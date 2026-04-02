// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/trace"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/unitstate"
)

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

	if err := name.Validate(); err != nil {
		return unitstate.RetrievedUnitState{}, err
	}

	state, err := s.st.GetUnitState(ctx, name.String())
	if err != nil {
		return unitstate.RetrievedUnitState{}, err
	}
	return state, nil
}
