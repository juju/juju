// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	coreblockdevice "github.com/juju/juju/core/blockdevice"
	"github.com/juju/juju/domain/blockdevice"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/network"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/domain/storageprovisioning"
)

// ImportStorageInstanceArgs represents data to import a storage instance
// and its owner.
type ImportStorageInstanceArgs struct {
	UUID              string
	Life              life.Life
	PoolName          string
	RequestedSizeMiB  uint64
	StorageName       string
	StorageKind       string
	StorageInstanceID string
	UnitUUID          string
}

// ImportStorageInstanceAttachmentArgs represents data to import a storage
// instance attachment.
type ImportStorageInstanceAttachmentArgs struct {
	UUID                string
	StorageInstanceUUID string
	UnitUUID            string
	Life                life.Life
}

// ImportFilesystemIAASArgs represents data to import a filesystem.
type ImportFilesystemIAASArgs struct {
	UUID                string
	ID                  string
	Life                life.Life
	SizeInMiB           uint64
	ProviderID          string
	StorageInstanceUUID string
	Scope               storageprovisioning.ProvisionScope
}

// ImportFilesystemAttachmentIAASArgs represents data to import filesystem attachments.
type ImportFilesystemAttachmentIAASArgs struct {
	UUID           string
	FilesystemUUID string
	NetNodeUUID    string
	Scope          storageprovisioning.ProvisionScope
	Life           life.Life
	MountPoint     string
	ReadOnly       bool
}

// ImportVolumeArgs represents a volume definition used when importing
// volumes into the model.
type ImportVolumeArgs struct {
	UUID                domainstorage.VolumeUUID
	ID                  string
	LifeID              life.Life
	StorageInstanceID   string
	StorageInstanceUUID domainstorage.StorageInstanceUUID
	Provisioned         bool
	ProvisionScopeID    storageprovisioning.ProvisionScope
	SizeMiB             uint64
	HardwareID          string
	WWN                 string
	ProviderID          string
	Persistent          bool
	Attachments         []ImportVolumeAttachmentArgs
	AttachmentPlans     []ImportVolumeAttachmentPlanArgs
}

// ImportVolumeAttachmentArgs represents a volume attachment with
// an existing BlockDevice.
type ImportVolumeAttachmentArgs struct {
	UUID            domainstorage.VolumeAttachmentUUID
	BlockDeviceUUID blockdevice.BlockDeviceUUID
	LifeID          life.Life
	NetNodeUUID     network.NetNodeUUID
	ReadOnly        bool
	ProviderID      string
}

// ImportVolumeAttachmentPlanArgs represents a volume attachment plan
// including storage volume needs to be created.
type ImportVolumeAttachmentPlanArgs struct {
	UUID             domainstorage.VolumeAttachmentPlanUUID
	DeviceAttributes map[string]string
	DeviceTypeID     *domainstorage.VolumeDeviceType
	LifeID           life.Life
	NetNodeUUID      network.NetNodeUUID
	ProvisionScopeID storageprovisioning.ProvisionScope
}

// BlockDevice adds the UUID to a core BlockDevice.
type BlockDevice struct {
	UUID blockdevice.BlockDeviceUUID
	coreblockdevice.BlockDevice
}
