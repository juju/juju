// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/trace"
	domainnetwork "github.com/juju/juju/domain/network"
	"github.com/juju/juju/domain/provisioner"
	"github.com/juju/juju/internal/errors"
)

// CompletionState is the state interface required by CompletionService
// for writing successful provisioning results. All methods execute their
// writes directly against the model database.
type CompletionState interface {
	// SetMachineCloudInstance records the cloud identity for a machine.
	// It must be called last in RecordProvisionedMachine because it fires
	// the change-stream notification watched by the instance-poller.
	SetMachineCloudInstance(
		ctx context.Context,
		machineUUID string,
		instanceID instance.Id,
		displayName, nonce string,
		hardwareCharacteristics *instance.HardwareCharacteristics,
	) error

	// RecordProvisionedMachineNetConfig writes link-layer devices, IP
	// addresses, and their provider IDs in a single transaction.
	RecordProvisionedMachineNetConfig(
		ctx context.Context,
		machineUUID string,
		nics []domainnetwork.NetInterface,
	) error

	// RecordProvisionedVolumes writes provider-assigned volume identities
	// in a single transaction.
	RecordProvisionedVolumes(
		ctx context.Context,
		volumes []provisioner.ProvisionedVolume,
	) error

	// RecordProvisionedVolumeAttachments writes volume attachment state
	// (block devices, read-only flags, attachment plans) in a single
	// transaction.
	RecordProvisionedVolumeAttachments(
		ctx context.Context,
		machineUUID string,
		attachments map[string]provisioner.ProvisionedVolumeAttachment,
	) error
}

// CompletionService records the complete result of a successful machine
// provisioning call. It calls the provisioner state directly instead of
// delegating to other domain services, which would be a forbidden
// service-to-service call.
type CompletionService struct {
	st CompletionState
}

// NewCompletionService returns a CompletionService that persists provisioning
// results through the provisioner state.
func NewCompletionService(st CompletionState) *CompletionService {
	return &CompletionService{st: st}
}

// RecordProvisionedMachine persists all data returned by a successful provider
// StartInstance call. The cloud-instance write is deliberately last: it emits
// the notification that causes the instance-poller to reconcile the provider
// status, so the poller never observes a newly registered instance before its
// network result is available.
func (s *CompletionService) RecordProvisionedMachine(
	ctx context.Context,
	machineUUID machine.UUID,
	info provisioner.ProvisionedMachineInfo,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := s.st.RecordProvisionedVolumes(ctx, info.Volumes); err != nil {
		return errors.Errorf("recording provisioned volumes for machine %q: %w", machineUUID, err)
	}
	if err := s.st.RecordProvisionedVolumeAttachments(ctx, machineUUID.String(), info.VolumeAttachments); err != nil {
		return errors.Errorf("recording provisioned volume attachments for machine %q: %w", machineUUID, err)
	}
	if err := s.recordNetwork(ctx, machineUUID, info); err != nil {
		return errors.Errorf("recording provisioned network for machine %q: %w", machineUUID, err)
	}
	if err := s.st.SetMachineCloudInstance(
		ctx,
		machineUUID.String(),
		info.InstanceID,
		info.DisplayName,
		info.Nonce,
		info.HardwareCharacteristics,
	); err != nil {
		return errors.Errorf("recording cloud instance for machine %q: %w", machineUUID, err)
	}
	return nil
}

func (s *CompletionService) recordNetwork(
	ctx context.Context,
	machineUUID machine.UUID,
	info provisioner.ProvisionedMachineInfo,
) error {
	if len(info.NetworkConfig) == 0 {
		return nil
	}
	nics := domainnetwork.ProviderNetInterfaces(info.NetworkConfig)
	return errors.Capture(s.st.RecordProvisionedMachineNetConfig(ctx, machineUUID.String(), nics))
}
