// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/passwords"
	"github.com/juju/juju/internal/errors"
	internalpassword "github.com/juju/juju/internal/password"
)

// State gets and sets the state of the service.
type State interface {
	// GetUnitUUID returns the UUID of the unit with the given name, returning
	// an error satisfying [applicationerrors.UnitNotFound] if the unit does not
	// exist.
	GetUnitUUID(context.Context, unit.Name) (unit.UUID, error)

	// SetUnitPasswordHash sets the password hash for the given unit.
	SetUnitPasswordHash(context.Context, unit.UUID, passwords.PasswordHash) error
}

// Service provides the means for interacting with the passwords in a model.
type Service struct {
	st State
}

// NewService returns a new Service.
func NewService(st State) *Service {
	return &Service{
		st: st,
	}
}

// SetUnitPassword sets the password for the given unit.
func (s *Service) SetUnitPassword(ctx context.Context, unitName unit.Name, password string) error {
	if err := unitName.Validate(); err != nil {
		return errors.Capture(err)
	}
	if len(password) < internalpassword.MinAgentPasswordLength {
		return errors.Errorf("password is only %d bytes long, and is not a valid Agent password", len(password))
	}

	unitUUID, err := s.st.GetUnitUUID(ctx, unitName)
	if err != nil {
		return errors.Capture(err)
	}

	return s.st.SetUnitPasswordHash(ctx, unitUUID, hashPassword(password))
}

func hashPassword(password string) passwords.PasswordHash {
	return passwords.PasswordHash(internalpassword.AgentPasswordHash(password))
}
