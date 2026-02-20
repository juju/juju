// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/storageprovisioning"
)

// ImportStorageInstanceArgs represents data to import a storage instance
// and its owner.
type ImportStorageInstanceArgs struct {
	UUID             string
	Life             int
	PoolName         string
	RequestedSizeMiB uint64
	StorageName      string
	StorageKind      string
	StorageID        string
	UnitName         string
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
	UUID                string
	ID                  string
	LifeID              life.Life
	StorageID           string
	StorageInstanceUUID string
	Provisioned         bool
	ProvisionScopeID    storageprovisioning.ProvisionScope
	SizeMiB             uint64
	HardwareID          string
	WWN                 string
	ProviderID          string
	Persistent          bool
	Attachments         []ImportVolumeAttachment
	// If the BlockDevice was not already imported, best effort
	// to create one which can be updated later.
	AttachmentsWithNewBlockDevice []ImportVolumeAttachmentNewBlockDevice
	AttachmentPlans               []ImportVolumeAttachmentPlan
}

// ImportVolumeAttachment represents a volume attachment with
// an existing BlockDevice.
type ImportVolumeAttachment struct {
	UUID            string
	BlockDeviceUUID string
	LifeID          life.Life
	NetNodeUUID     string
	ReadOnly        bool
	ProviderID      string
}

// ImportVolumeAttachmentNewBlockDevice represents a volume attachment
// where a BlockDevice needs to be created.
type ImportVolumeAttachmentNewBlockDevice struct {
	ImportVolumeAttachment
	MachineUUID string
	Provisioned bool
	BusAddress  string
	DeviceLink  string
	DeviceName  string
}

// ImportVolumeAttachmentPlan represents a volume attachment plan
// including storage volume needs to be created.
type ImportVolumeAttachmentPlan struct {
	UUID             string
	DeviceAttributes map[string]string
	DeviceTypeID     int
	LifeID           life.Life
	NetNodeUUID      string
	ProvisionScopeID storageprovisioning.ProvisionScope
}
