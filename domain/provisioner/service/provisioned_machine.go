// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	coreblockdevice "github.com/juju/juju/core/blockdevice"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	domainblockdevice "github.com/juju/juju/domain/blockdevice"
	domainnetwork "github.com/juju/juju/domain/network"
	"github.com/juju/juju/domain/provisioner"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/domain/storageprovisioning"
	"github.com/juju/juju/internal/errors"
)

// CompletionService records the complete result of successful machine
// provisioning. It composes the existing domain services that own each part of
// the model instead of duplicating their persistence logic.
type CompletionService struct {
	machineService     MachineInstanceService
	networkService     MachineNetworkService
	storageService     StorageProvisioningService
	blockDeviceService BlockDeviceService
}

// MachineInstanceService records the cloud identity for a machine.
type MachineInstanceService interface {
	SetMachineCloudInstance(
		ctx context.Context,
		machineUUID machine.UUID,
		instanceID instance.Id,
		displayName, nonce string,
		hardwareCharacteristics *instance.HardwareCharacteristics,
	) error
}

// MachineNetworkService records machine networking and provider identities.
type MachineNetworkService interface {
	SetMachineNetConfig(context.Context, machine.UUID, []domainnetwork.NetInterface) error
	SetProviderNetConfig(context.Context, machine.UUID, []domainnetwork.NetInterface) error
}

// StorageProvisioningService records provisioned volume and attachment data.
type StorageProvisioningService interface {
	SetVolumeProvisionedInfo(context.Context, string, storageprovisioning.VolumeProvisionedInfo) error
	GetVolumeAttachmentUUIDForVolumeIDMachine(context.Context, string, machine.UUID) (domainstorage.VolumeAttachmentUUID, error)
	SetVolumeAttachmentProvisionedInfo(context.Context, domainstorage.VolumeAttachmentUUID, storageprovisioning.VolumeAttachmentProvisionedInfo) error
	GetVolumeAttachmentPlanUUIDForVolumeIDMachine(context.Context, string, machine.UUID) (domainstorage.VolumeAttachmentPlanUUID, error)
	SetVolumeAttachmentPlanProvisionedInfo(context.Context, domainstorage.VolumeAttachmentPlanUUID, storageprovisioning.VolumeAttachmentPlanProvisionedInfo) error
}

// BlockDeviceService creates a block-device record when an attachment exposes
// a device to the machine.
type BlockDeviceService interface {
	MatchOrCreateBlockDevice(context.Context, machine.UUID, coreblockdevice.BlockDevice) (domainblockdevice.BlockDeviceUUID, error)
}

// NewCompletionService returns a service that records successful provisioning
// results using the existing machine, networking, storage, and block-device
// domain services.
func NewCompletionService(
	machineService MachineInstanceService,
	networkService MachineNetworkService,
	storageService StorageProvisioningService,
	blockDeviceService BlockDeviceService,
) *CompletionService {
	return &CompletionService{
		machineService:     machineService,
		networkService:     networkService,
		storageService:     storageService,
		blockDeviceService: blockDeviceService,
	}
}

// RecordProvisionedMachine records all data returned by a successful provider
// StartInstance call. The cloud-instance write is deliberately last: it emits
// the notification that causes the instance-poller to reconcile the provider
// status, so the poller never observes a newly registered instance before its
// network result is available.
func (s *CompletionService) RecordProvisionedMachine(
	ctx context.Context,
	machineUUID machine.UUID,
	info provisioner.ProvisionedMachineInfo,
) error {
	if err := s.recordVolumes(ctx, machineUUID, info); err != nil {
		return errors.Errorf("recording provisioned volumes for machine %q: %w", machineUUID, err)
	}
	if err := s.recordNetwork(ctx, machineUUID, info); err != nil {
		return errors.Errorf("recording provisioned network for machine %q: %w", machineUUID, err)
	}
	if err := s.machineService.SetMachineCloudInstance(
		ctx,
		machineUUID,
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
	if err := s.networkService.SetMachineNetConfig(ctx, machineUUID, nics); err != nil {
		return errors.Capture(err)
	}
	// SetMachineNetConfig creates the devices and addresses. The provider merge
	// then records their provider IDs, which cannot be inferred from machine
	// network observations alone.
	if err := s.networkService.SetProviderNetConfig(ctx, machineUUID, nics); err != nil {
		return errors.Capture(err)
	}
	return nil
}

func (s *CompletionService) recordVolumes(
	ctx context.Context,
	machineUUID machine.UUID,
	info provisioner.ProvisionedMachineInfo,
) error {
	for _, volume := range info.Volumes {
		if err := s.storageService.SetVolumeProvisionedInfo(ctx, volume.VolumeID, storageprovisioning.VolumeProvisionedInfo{
			ProviderID: volume.ProviderID,
			SizeMiB:    volume.SizeMiB,
			HardwareID: volume.HardwareID,
			WWN:        volume.WWN,
			Persistent: volume.Persistent,
		}); err != nil {
			return errors.Errorf("recording volume %q: %w", volume.VolumeID, err)
		}
	}

	for volumeID, attachment := range info.VolumeAttachments {
		if err := s.recordVolumeAttachment(ctx, machineUUID, volumeID, attachment); err != nil {
			return errors.Errorf("recording attachment for volume %q: %w", volumeID, err)
		}
	}
	return nil
}

func (s *CompletionService) recordVolumeAttachment(
	ctx context.Context,
	machineUUID machine.UUID,
	volumeID string,
	attachment provisioner.ProvisionedVolumeAttachment,
) error {
	attachmentUUID, err := s.storageService.GetVolumeAttachmentUUIDForVolumeIDMachine(ctx, volumeID, machineUUID)
	if err != nil {
		return errors.Capture(err)
	}

	provisionedInfo := storageprovisioning.VolumeAttachmentProvisionedInfo{
		ReadOnly: attachment.ReadOnly,
	}
	if attachment.DeviceName != "" || attachment.DeviceLink != "" || attachment.BusAddress != "" {
		device := coreblockdevice.BlockDevice{
			DeviceName: attachment.DeviceName,
			BusAddress: attachment.BusAddress,
		}
		if attachment.DeviceLink != "" {
			device.DeviceLinks = []string{attachment.DeviceLink}
		}
		blockDeviceUUID, err := s.blockDeviceService.MatchOrCreateBlockDevice(ctx, machineUUID, device)
		if err != nil {
			return errors.Capture(err)
		}
		provisionedInfo.BlockDeviceUUID = &blockDeviceUUID
	}

	if err := s.storageService.SetVolumeAttachmentProvisionedInfo(ctx, attachmentUUID, provisionedInfo); err != nil {
		return errors.Capture(err)
	}
	if attachment.Plan == nil {
		return nil
	}

	deviceType, err := domainstorage.ParseVolumeDeviceType(attachment.Plan.DeviceType)
	if err != nil {
		return errors.Capture(err)
	}
	planUUID, err := s.storageService.GetVolumeAttachmentPlanUUIDForVolumeIDMachine(ctx, volumeID, machineUUID)
	if err != nil {
		return errors.Capture(err)
	}
	return s.storageService.SetVolumeAttachmentPlanProvisionedInfo(ctx, planUUID, storageprovisioning.VolumeAttachmentPlanProvisionedInfo{
		DeviceType:       deviceType,
		DeviceAttributes: attachment.Plan.DeviceAttributes,
	})
}
