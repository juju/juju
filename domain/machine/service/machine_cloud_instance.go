// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/internal/errors"
)

// InstanceID returns the cloud specific instance id for this machine.
// If the machine is not provisioned, it returns a
// [github.com/juju/juju/domain/machine/errors.NotProvisioned]
func (s *Service) InstanceID(ctx context.Context, machineUUID string) (instance.Id, error) {
	instanceId, err := s.st.InstanceID(ctx, machineUUID)
	if err != nil {
		return "", errors.Errorf("retrieving cloud instance id for machine %q %w", machineUUID, err)
	}
	return instance.Id(instanceId), nil
}

// InstanceIDAndName returns the cloud specific instance ID and display name for
// this machine.
// If the machine is not provisioned, it returns a
// [github.com/juju/juju/domain/machine/errors.NotProvisioned]
func (s *Service) InstanceIDAndName(ctx context.Context, machineUUID string) (instance.Id, string, error) {
	instanceID, instanceName, err := s.st.InstanceIDAndName(ctx, machineUUID)
	if err != nil {
		return "", "", errors.Errorf("retrieving cloud instance name for machine %q %w", machineUUID, err)
	}
	return instance.Id(instanceID), instanceName, nil
}

// AvailabilityZone returns the availability zone for the specified machine.
func (s *Service) AvailabilityZone(ctx context.Context, machineUUID string) (string, error) {
	az, err := s.st.AvailabilityZone(ctx, machineUUID)
	return az, errors.Errorf("retrieving availability zone for machine %q %w", machineUUID, err)
}

// HardwareCharacteristics returns the hardware characteristics of the
// of the specified machine.
func (s *Service) HardwareCharacteristics(ctx context.Context, machineUUID string) (*instance.HardwareCharacteristics, error) {
	hc, err := s.st.HardwareCharacteristics(ctx, machineUUID)
	return hc, errors.Errorf("retrieving hardware characteristics for machine %q %w", machineUUID, err)
}

// SetMachineCloudInstance sets an entry in the machine cloud instance table
// along with the instance tags and the link to a lxd profile if any.
func (s *Service) SetMachineCloudInstance(
	ctx context.Context,
	machineUUID string,
	instanceID instance.Id,
	displayName string,
	hardwareCharacteristics *instance.HardwareCharacteristics,
) error {
	return errors.Errorf("setting machine cloud instance for machine %q %w", machineUUID,
		s.st.SetMachineCloudInstance(ctx, machineUUID, instanceID, displayName, hardwareCharacteristics))

}

// DeleteMachineCloudInstance removes an entry in the machine cloud instance
// table along with the instance tags and the link to a lxd profile if any, as
// well as any associated status data.
func (s *Service) DeleteMachineCloudInstance(ctx context.Context, machineUUID string) error {
	return errors.Errorf("deleting machine cloud instance for machine %q %w", machineUUID,
		s.st.DeleteMachineCloudInstance(ctx, machineUUID))

}
