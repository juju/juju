// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/agentpassword"
	passworderrors "github.com/juju/juju/domain/agentpassword/errors"
	"github.com/juju/juju/internal/errors"
	internalpassword "github.com/juju/juju/internal/password"
)

// State gets and sets the state of the service.
type State interface {
	// GetUnitUUID returns the UUID of the unit with the given name.
	GetUnitUUID(context.Context, unit.Name) (unit.UUID, error)

	// SetUnitPasswordHash sets the password hash for the given unit.
	SetUnitPasswordHash(context.Context, unit.UUID, agentpassword.PasswordHash) error

	// MatchesUnitPasswordHash checks if the password is valid or not against
	// the password hash stored in the database.
	MatchesUnitPasswordHash(context.Context, unit.UUID, agentpassword.PasswordHash) (bool, error)

	// GetMachineUUID returns the UUID of the machine with the given name.
	GetMachineUUID(context.Context, machine.Name) (machine.UUID, error)

	// SetMachinePasswordHash sets the password hash for the given machine.
	SetMachinePasswordHash(context.Context, machine.UUID, agentpassword.PasswordHash) error

	// MatchesMachinePasswordHashWithNonce checks if the password is valid or
	// not against the password hash with the nonce stored in the database.
	MatchesMachinePasswordHashWithNonce(context.Context, machine.UUID, agentpassword.PasswordHash, string) (bool, error)
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

// SetUnitPassword sets the password for the given unit. If the unit does not
// exist, an error satisfying [passworderrors.UnitNotFound] is returned.
func (s *Service) SetUnitPassword(ctx context.Context, unitName unit.Name, password string) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := unitName.Validate(); err != nil {
		return errors.Capture(err)
	}
	if len(password) < internalpassword.MinAgentPasswordLength {
		return errors.Errorf("password is only %d bytes long, and is not a valid Agent password: %w", len(password), passworderrors.InvalidPassword)
	}

	unitUUID, err := s.st.GetUnitUUID(ctx, unitName)
	if err != nil {
		return errors.Capture(err)
	}

	return s.st.SetUnitPasswordHash(ctx, unitUUID, hashPassword(password))
}

// MatchesUnitPasswordHash checks if the password is valid or not against the
// password hash stored in the database.
func (s *Service) MatchesUnitPasswordHash(ctx context.Context, unitName unit.Name, password string) (bool, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := unitName.Validate(); err != nil {
		return false, errors.Capture(err)
	}

	// An empty password is never valid.
	if password == "" {
		return false, passworderrors.EmptyPassword
	} else if len(password) < internalpassword.MinAgentPasswordLength {
		return false, errors.Errorf("password is only %d bytes long, and is not a valid Agent password: %w", len(password), passworderrors.InvalidPassword)
	}

	unitUUID, err := s.st.GetUnitUUID(ctx, unitName)
	if err != nil {
		return false, errors.Capture(err)
	}

	return s.st.MatchesUnitPasswordHash(ctx, unitUUID, hashPassword(password))
}

// SetMachinePassword sets the password for the given machine. If the machine does not
// exist, an error satisfying [passworderrors.UnitNotFound] is returned.
func (s *Service) SetMachinePassword(ctx context.Context, machineName machine.Name, password string) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := machineName.Validate(); err != nil {
		return errors.Capture(err)
	}
	if len(password) < internalpassword.MinAgentPasswordLength {
		return errors.Errorf("password is only %d bytes long, and is not a valid Agent password: %w", len(password), passworderrors.InvalidPassword)
	}

	machineUUID, err := s.st.GetMachineUUID(ctx, machineName)
	if err != nil {
		return errors.Capture(err)
	}

	return s.st.SetMachinePasswordHash(ctx, machineUUID, hashPassword(password))
}

// MatchesMachinePasswordHashWithNonce checks if the password with a nonce is
// valid or not against the password hash stored in the database.
func (s *Service) MatchesMachinePasswordHashWithNonce(ctx context.Context, machineName machine.Name, password, nonce string) (bool, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := machineName.Validate(); err != nil {
		return false, errors.Capture(err)
	}

	// An empty password is never valid.
	if password == "" {
		return false, passworderrors.EmptyPassword
	} else if len(password) < internalpassword.MinAgentPasswordLength {
		return false, errors.Errorf("password is only %d bytes long, and is not a valid Agent password: %w", len(password), passworderrors.InvalidPassword)
	}

	if nonce == "" {
		return false, passworderrors.EmptyNonce
	}

	unitUUID, err := s.st.GetMachineUUID(ctx, machineName)
	if err != nil {
		return false, errors.Capture(err)
	}

	return s.st.MatchesMachinePasswordHashWithNonce(ctx, unitUUID, hashPassword(password), nonce)
}

func hashPassword(p string) agentpassword.PasswordHash {
	return agentpassword.PasswordHash(internalpassword.AgentPasswordHash(p))
}
