// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/unitstate"
	"github.com/juju/juju/internal/errors"
)

// State defines an interface for interacting with the underlying state.
type State interface {
	domain.AtomicStateBase

	// GetUnitUUIDForName returns the UUID for
	// the unit identified by the input name.
	GetUnitUUIDForName(domain.AtomicContext, coreunit.Name) (coreunit.UUID, error)

	// EnsureUnitStateRecord ensures that there is a record
	// for the agent state for the unit with the input UUID.
	EnsureUnitStateRecord(domain.AtomicContext, coreunit.UUID) error

	// UpdateUnitStateUniter updates the agent uniter
	// state for the unit with the input UUID.
	UpdateUnitStateUniter(domain.AtomicContext, coreunit.UUID, string) error

	// UpdateUnitStateStorage updates the agent storage
	// state for the unit with the input UUID.
	UpdateUnitStateStorage(domain.AtomicContext, coreunit.UUID, string) error

	// UpdateUnitStateSecret updates the agent secret
	// state for the unit with the input UUID.
	UpdateUnitStateSecret(domain.AtomicContext, coreunit.UUID, string) error

	// SetUnitStateCharm replaces the agent charm
	// state for the unit with the input UUID.
	SetUnitStateCharm(domain.AtomicContext, coreunit.UUID, map[string]string) error

	// SetUnitStateRelation replaces the agent relation
	// state for the unit with the input UUID.
	SetUnitStateRelation(domain.AtomicContext, coreunit.UUID, map[int]string) error

	// GetUnitState returns the full unit agent state.
	// If no unit with the uuid exists, a [unitstateerrors.UnitNotFound] error
	// is returned.
	// If the units state is empty [unitstateerrors.EmptyUnitState] error is
	// returned.
	GetUnitState(ctx context.Context, name coreunit.Name) (unitstate.RetrievedUnitState, error)
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
	return s.st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		uuid, err := s.st.GetUnitUUIDForName(ctx, as.Name)
		if err != nil {
			return errors.Errorf("getting unit UUID for %s: %w", as.Name, err)
		}

		if err = s.st.EnsureUnitStateRecord(ctx, uuid); err != nil {
			return errors.Errorf("ensuring state record for %s: %w", as.Name, err)
		}

		if as.UniterState != nil {
			if err = s.st.UpdateUnitStateUniter(ctx, uuid, *as.UniterState); err != nil {
				return errors.Errorf("setting uniter state for %s: %w", as.Name, err)
			}
		}

		if as.StorageState != nil {
			if err = s.st.UpdateUnitStateStorage(ctx, uuid, *as.StorageState); err != nil {
				return errors.Errorf("setting storage state for %s: %w", as.Name, err)
			}
		}

		if as.SecretState != nil {
			if err = s.st.UpdateUnitStateSecret(ctx, uuid, *as.SecretState); err != nil {
				return errors.Errorf("setting secret state for %s: %w", as.Name, err)
			}
		}

		if as.CharmState != nil {
			if err = s.st.SetUnitStateCharm(ctx, uuid, *as.CharmState); err != nil {
				return errors.Errorf("setting charm state for %s: %w", as.Name, err)
			}
		}

		if as.RelationState != nil {
			if err = s.st.SetUnitStateRelation(ctx, uuid, *as.RelationState); err != nil {
				return errors.Errorf("setting relation state for %s: %w", as.Name, err)
			}
		}

		return nil
	})
}

// GetState returns the full unit state. The state may be empty.
func (s *Service) GetState(ctx context.Context, name coreunit.Name) (unitstate.RetrievedUnitState, error) {
	state, err := s.st.GetUnitState(ctx, name)
	if err != nil {
		return unitstate.RetrievedUnitState{}, err
	}
	return state, nil
}
