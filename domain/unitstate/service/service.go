// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/unitstate"
	"github.com/juju/juju/internal/errors"
)

// State defines an interface for interacting with the underlying state.
type State interface {
	domain.AtomicStateBase

	// GetUnitUUIDForName returns the UUID for
	// the unit identified by the input name.
	GetUnitUUIDForName(domain.AtomicContext, string) (string, error)

	// EnsureUnitStateRecord ensures that there is a record
	// for the agent state for the unit with the input UUID.
	EnsureUnitStateRecord(domain.AtomicContext, string) error

	// UpdateUnitStateUniter updates the agent uniter
	// state for the unit with the input UUID.
	UpdateUnitStateUniter(domain.AtomicContext, string, string) error

	// UpdateUnitStateStorage updates the agent storage
	// state for the unit with the input UUID.
	UpdateUnitStateStorage(domain.AtomicContext, string, string) error

	// UpdateUnitStateSecret updates the agent secret
	// state for the unit with the input UUID.
	UpdateUnitStateSecret(domain.AtomicContext, string, string) error

	// SetUnitStateCharm replaces the agent charm
	// state for the unit with the input UUID.
	SetUnitStateCharm(domain.AtomicContext, string, map[string]string) error

	// SetUnitStateRelation replaces the agent relation
	// state for the unit with the input UUID.
	SetUnitStateRelation(domain.AtomicContext, string, map[int]string) error
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

// SetState persists the input agent state selectively,
// based on its populated values.
func (s *Service) SetState(ctx context.Context, as unitstate.AgentState) error {
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
