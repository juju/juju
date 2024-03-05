// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/domain/application"
)

// State describes retrieval and persistence methods for applications.
type State interface {
	// UpsertApplication persists the input Application entity.
	UpsertApplication(context.Context, string, ...application.AddUnitParams) error

	// DeleteApplication deletes the input Application entity.
	DeleteApplication(context.Context, string) error

	// AddUnits adds the specified units to the application.
	AddUnits(ctx context.Context, applicationName string, args ...application.AddUnitParams) error
}

// Service provides the API for working with applications.
type Service struct {
	st State
}

// NewService returns a new service reference wrapping the input state.
func NewService(st State) *Service {
	return &Service{
		st: st,
	}
}

// CreateApplication creates the specified application and units if required.
func (s *Service) CreateApplication(ctx context.Context, name string, params AddApplicationParams, units ...AddUnitParams) error {
	args := make([]application.AddUnitParams, len(units))
	for i, u := range units {
		args[i] = application.AddUnitParams{
			UnitName: u.UnitName,
		}
	}
	err := s.st.UpsertApplication(ctx, name, args...)
	return errors.Annotatef(err, "saving application %q", name)
}

// AddUnits adds units to the application.
func (s *Service) AddUnits(ctx context.Context, name string, units ...AddUnitParams) error {
	args := make([]application.AddUnitParams, len(units))
	for i, u := range units {
		args[i] = application.AddUnitParams{
			UnitName: u.UnitName,
		}
	}
	err := s.st.AddUnits(ctx, name, args...)
	return errors.Annotatef(err, "adding units to application %q", name)
}

// UpsertCAASUnit records the existence of a unit in a caas model.
func (s *Service) UpsertCAASUnit(ctx context.Context, name string, unit UpsertCAASUnitParams) error {
	args := application.AddUnitParams{
		UnitName: unit.UnitName,
	}
	err := s.st.UpsertApplication(ctx, name, args)
	return errors.Annotatef(err, "saving application %q", name)
}

// DeleteApplication deletes the specified application.
func (s *Service) DeleteApplication(ctx context.Context, name string) error {
	err := s.st.DeleteApplication(ctx, name)
	return errors.Annotatef(err, "deleting application %q", name)
}
