// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/clock"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/logger"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain/machine"
	"github.com/juju/juju/internal/errors"
)

// MigrationState defines the state interface for migration operations.
type MigrationState interface {
	// GetMachinesForExport returns all machines in the model for export.
	GetMachinesForExport(ctx context.Context) ([]machine.ExportMachine, error)

	// GetHardwareCharacteristics returns the hardware characteristics struct with
	// data retrieved from the machine cloud instance table.
	GetHardwareCharacteristics(context.Context, coremachine.UUID) (*instance.HardwareCharacteristics, error)

	// GetInstanceID returns the cloud specific instance id for this machine.
	GetInstanceID(context.Context, coremachine.UUID) (string, error)
}

// MigrationService provides the API for migrating applications.
type MigrationService struct {
	st     MigrationState
	clock  clock.Clock
	logger logger.Logger
}

// NewMigrationService returns a new service reference wrapping the input state.
func NewMigrationService(
	st MigrationState,
	clock clock.Clock,
	logger logger.Logger,
) *MigrationService {
	return &MigrationService{
		st:     st,
		clock:  clock,
		logger: logger,
	}
}

// GetMachines returns all the machines in the model.
func (s *MigrationService) GetMachines(ctx context.Context) ([]machine.ExportMachine, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.st.GetMachinesForExport(ctx)
}

// GetInstanceID returns the cloud specific instance id for this machine.
// If the machine is not provisioned, it returns a
// [github.com/juju/juju/domain/machine/errors.NotProvisioned]
func (s *MigrationService) GetInstanceID(ctx context.Context, machineUUID coremachine.UUID) (instance.Id, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	instanceId, err := s.st.GetInstanceID(ctx, machineUUID)
	if err != nil {
		return "", errors.Errorf("retrieving cloud instance id for machine %q: %w", machineUUID, err)
	}
	return instance.Id(instanceId), nil
}

// GetHardwareCharacteristics returns the hardware characteristics of the
// of the specified machine.
func (s *MigrationService) GetHardwareCharacteristics(ctx context.Context, machineUUID coremachine.UUID) (*instance.HardwareCharacteristics, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	hc, err := s.st.GetHardwareCharacteristics(ctx, machineUUID)
	if err != nil {
		return hc, errors.Errorf("retrieving hardware characteristics for machine %q: %w", machineUUID, err)
	}
	return hc, nil
}
