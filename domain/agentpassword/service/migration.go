// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/agentpassword"
	"github.com/juju/juju/internal/errors"
)

// MigrationState is the state required for migrating passwords.
type MigrationState interface {
	// GetAllUnitPasswordHashes returns a map of unit names to password hashes.
	GetAllUnitPasswordHashes(context.Context) (agentpassword.UnitPasswordHashes, error)

	// GetUnitUUID returns the UUID of the unit with the given name.
	GetUnitUUID(context.Context, unit.Name) (unit.UUID, error)

	// SetUnitPasswordHash sets the password hash for the given unit.
	SetUnitPasswordHash(context.Context, unit.UUID, agentpassword.PasswordHash) error

	// GetAllMachinePasswordHashes returns a map of unit names to password
	// hashes.
	GetAllMachinePasswordHashes(context.Context) (agentpassword.MachinePasswordHashes, error)

	// GetMachineUUID returns the UUID of the machine with the given name.
	GetMachineUUID(context.Context, machine.Name) (machine.UUID, error)

	// SetMachinePasswordHash sets the password hash for the given machine.
	SetMachinePasswordHash(context.Context, machine.UUID, agentpassword.PasswordHash) error
}

// MigrationService provides the API for migrating passwords.
type MigrationService struct {
	st MigrationState
}

// NewMigrationService returns a new service reference wrapping the input state.
func NewMigrationService(
	st MigrationState,
) *MigrationService {
	return &MigrationService{
		st: st,
	}
}

// GetAllUnitPasswordHashes returns a map of unit names to password hashes.
func (s *MigrationService) GetAllUnitPasswordHashes(ctx context.Context) (agentpassword.UnitPasswordHashes, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.st.GetAllUnitPasswordHashes(ctx)
}

// SetUnitPasswordHash sets the password hash for the given unit.
func (s *MigrationService) SetUnitPasswordHash(ctx context.Context, unitName unit.Name, passwordHash agentpassword.PasswordHash) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := unitName.Validate(); err != nil {
		return err
	}

	unitUUID, err := s.st.GetUnitUUID(ctx, unitName)
	if err != nil {
		return errors.Errorf("getting unit UUID: %w", err)
	}

	return s.st.SetUnitPasswordHash(ctx, unitUUID, passwordHash)
}

// GetAllMachinePasswordHashes returns a map of machine names to password hashes.
func (s *MigrationService) GetAllMachinePasswordHashes(ctx context.Context) (agentpassword.MachinePasswordHashes, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.st.GetAllMachinePasswordHashes(ctx)
}

// SetMachinePasswordHash sets the password hash for the given machine.
func (s *MigrationService) SetMachinePasswordHash(ctx context.Context, machineName machine.Name, passwordHash agentpassword.PasswordHash) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := machineName.Validate(); err != nil {
		return err
	}

	machineUUID, err := s.st.GetMachineUUID(ctx, machineName)
	if err != nil {
		return errors.Errorf("getting machine UUID: %w", err)
	}

	return s.st.SetMachinePasswordHash(ctx, machineUUID, passwordHash)
}
