// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/internal/errors"
)

// GetInstanceID returns the cloud specific instance id for this machine.
// If the machine is not provisioned, it returns a
// [github.com/juju/juju/domain/machine/errors.NotProvisioned]
func (s *Service) GetInstanceID(ctx context.Context, machineUUID machine.UUID) (instance.Id, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	instanceId, err := s.st.GetInstanceID(ctx, machineUUID)
	if err != nil {
		return "", errors.Errorf("retrieving cloud instance id for machine %q: %w", machineUUID, err)
	}
	return instance.Id(instanceId), nil
}

// GetInstanceIDAndName returns the cloud specific instance ID and display name for
// this machine.
// If the machine is not provisioned, it returns a
// [github.com/juju/juju/domain/machine/errors.NotProvisioned]
func (s *Service) GetInstanceIDAndName(ctx context.Context, machineUUID machine.UUID) (instance.Id, string, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	instanceID, instanceName, err := s.st.GetInstanceIDAndName(ctx, machineUUID)
	if err != nil {
		return "", "", errors.Errorf("retrieving cloud instance name for machine %q: %w", machineUUID, err)
	}
	return instance.Id(instanceID), instanceName, nil
}

// AvailabilityZone returns the availability zone for the specified machine.
func (s *Service) AvailabilityZone(ctx context.Context, machineUUID machine.UUID) (string, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	az, err := s.st.AvailabilityZone(ctx, machineUUID)
	if err != nil {
		return az, errors.Errorf("retrieving availability zone for machine %q: %w", machineUUID, err)
	}
	return az, nil
}

// GetHardwareCharacteristics returns the hardware characteristics of the
// of the specified machine.
func (s *Service) GetHardwareCharacteristics(ctx context.Context, machineUUID machine.UUID) (*instance.HardwareCharacteristics, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	hc, err := s.st.GetHardwareCharacteristics(ctx, machineUUID)
	if err != nil {
		return hc, errors.Errorf("retrieving hardware characteristics for machine %q: %w", machineUUID, err)
	}
	return hc, nil
}

// SetMachineCloudInstance sets an entry in the machine cloud instance table
// along with the instance tags and the link to a lxd profile if any.
func (s *Service) SetMachineCloudInstance(
	ctx context.Context,
	machineUUID machine.UUID,
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

// DeleteMachineCloudInstance removes an entry in the machine cloud instance
// table along with the instance tags and the link to a lxd profile if any, as
// well as any associated status data.
func (s *Service) DeleteMachineCloudInstance(ctx context.Context, machineUUID machine.UUID) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := s.st.DeleteMachineCloudInstance(ctx, machineUUID); err != nil {
		return errors.Errorf("deleting machine cloud instance for machine %q: %w", machineUUID, err)
	}
	return nil
}
