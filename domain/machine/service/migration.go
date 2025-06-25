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
	domainmachine "github.com/juju/juju/domain/machine"
	"github.com/juju/juju/internal/errors"
)

// MigrationState defines the state interface for migration operations.
type MigrationState interface {
	// CreateMachine creates or updates the specified machine.
	// Adds a row to machine table, as well as a row to the net_node table.
	CreateMachine(ctx context.Context, args domainmachine.CreateMachineArgs) (coremachine.Name, error)

	// GetMachinesForExport returns all machines in the model for export.
	GetMachinesForExport(ctx context.Context) ([]machine.ExportMachine, error)

	// GetHardwareCharacteristics returns the hardware characteristics struct with
	// data retrieved from the machine cloud instance table.
	GetHardwareCharacteristics(context.Context, string) (*instance.HardwareCharacteristics, error)

	// SetMachineCloudInstance sets an entry in the machine cloud instance table
	// along with the instance tags and the link to a lxd profile if any.
	SetMachineCloudInstance(context.Context, string, instance.Id, string, string, *instance.HardwareCharacteristics) error

	// GetInstanceID returns the cloud specific instance id for this machine.
	GetInstanceID(context.Context, string) (string, error)
}

// MigrationService provides the API for migrating applications.
type MigrationService struct {
	st            MigrationState
	statusHistory StatusHistory
	clock         clock.Clock
	logger        logger.Logger
}

// NewMigrationService returns a new service reference wrapping the input state.
func NewMigrationService(
	st MigrationState,
	statusHistory StatusHistory,
	clock clock.Clock,
	logger logger.Logger,
) *MigrationService {
	return &MigrationService{
		st:            st,
		statusHistory: statusHistory,
		clock:         clock,
		logger:        logger,
	}
}

// CreateMachine creates the specified machine.
// It returns a MachineAlreadyExists error if a machine with the same name
// already exists.
func (s *MigrationService) CreateMachine(ctx context.Context, machineName coremachine.Name, nonce *string) (coremachine.UUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	// Make new UUIDs for the net-node and the machine.
	// We want to do this in the service layer so that if retries are invoked at
	// the state layer we don't keep regenerating.
	nodeUUID, machineUUID, err := createUUIDs()
	if err != nil {
		return "", errors.Errorf("creating machine %q: %w", machineName, err)
	}
	err = s.st.CreateMachine(ctx, machineName, nodeUUID, machineUUID, nonce)
	if err != nil {
		return machineUUID, errors.Errorf("creating machine %q: %w", machineName, err)
	}

	if err := recordCreateMachineStatusHistory(ctx, s.statusHistory, machineName, s.clock); err != nil {
		s.logger.Infof(ctx, "failed recording machine status history: %w", err)
	}

	return machineUUID, nil
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

	instanceId, err := s.st.GetInstanceID(ctx, machineUUID.String())
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

	hc, err := s.st.GetHardwareCharacteristics(ctx, machineUUID.String())
	if err != nil {
		return hc, errors.Errorf("retrieving hardware characteristics for machine %q: %w", machineUUID, err)
	}
	return hc, nil
}

// SetMachineCloudInstance sets an entry in the machine cloud instance table
// along with the instance tags and the link to a lxd profile if any.
func (s *MigrationService) SetMachineCloudInstance(
	ctx context.Context,
	machineUUID coremachine.UUID,
	instanceID instance.Id,
	displayName, nonce string,
	hardwareCharacteristics *instance.HardwareCharacteristics,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := s.st.SetMachineCloudInstance(ctx, machineUUID, instanceID, displayName, nonce, hardwareCharacteristics); err != nil {
		return errors.Errorf("setting machine cloud instance for machine %q: %w", machineUUID, err)
	}
	return nil
}
