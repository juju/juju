// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"database/sql"

	"github.com/juju/juju/domain/life"
)

// attachmentLife represents the current life value of either a filesystem or
// volume attachment in the model.
type attachmentLife struct {
	UUID string    `db:"uuid"`
	Life life.Life `db:"life_id"`
}

// attachmentLives is a convenience type that facilitates transforming a slice
// of [attachmentLives] values to a map.
type attachmentLives []attachmentLife

// Iter provides a seq2 implementation for iterating the values of
// [attachmentLives].
func (l attachmentLives) Iter(yield func(string, life.Life) bool) {
	for _, v := range l {
		if !yield(v.UUID, v.Life) {
			return
		}
	}
}

type storageAttachmentLife struct {
	StorageInstanceID string    `db:"storage_id"`
	Life              life.Life `db:"life_id"`
}

type storageAttachmentLives []storageAttachmentLife

// Iter provides a seq2 implementation for iterating the values of
// [storageAttachmentLives].
func (l storageAttachmentLives) Iter(yield func(string, life.Life) bool) {
	for _, v := range l {
		if !yield(v.StorageInstanceID, v.Life) {
			return
		}
	}
}

// entityLife represents the current life value of a storage entity in the model.
type entityLife struct {
	LifeID int `db:"life_id"`
}

// entityUUID represents the UUID of a storage entity in the model.
type entityUUID struct {
	UUID string `db:"uuid"`
}

type entityName struct {
	Name string `db:"name"`
}

// filesystemAttachmentIDs represents the ids of attachment points to a
// filesystem attachment. This information includes the filesystem ID the
// attachment is for. As well as this either the machine or unit name the
// attachment for is included.
type filesystemAttachmentIDs struct {
	UUID         string         `db:"uuid"`
	FilesystemID string         `db:"filesystem_id"`
	MachineName  sql.NullString `db:"machine_name"`
	UnitName     sql.NullString `db:"unit_name"`
}

// filesystemAttachmentUUID represents the UUID of a record in the
// filesystem_attachment table.
type filesystemAttachmentUUID entityUUID

// filesystemAttachmentUUIDs represents a slice of filesystem attachment UUIDs.
// This type exists so that we can provide sqlair with a named type to process a
// slice of strings.
type filesystemAttachmentUUIDs []string

// filesystemID represents the filesystem id value for a storage filesystem
// instance.
type filesystemID struct {
	ID string `db:"filesystem_id"`
}

// filesystemLife represents the current life value and filesystem id for a
// storage filesystem instance in the model.
type filesystemLife struct {
	ID   string    `db:"filesystem_id"`
	Life life.Life `db:"life_id"`
}

// filesystemLives is a convenience type that facilitates transforming a slice
// of [filesystemLife] values to a map.
type filesystemLives []filesystemLife

// Iter provides a seq2 implementation for iterating the values of
// [filesystemLives].
func (l filesystemLives) Iter(yield func(string, life.Life) bool) {
	for _, v := range l {
		if !yield(v.ID, v.Life) {
			return
		}
	}
}

// filesystemAttachmentParams represents the attachment params for a filesystem
// attachment from the model database.
type filesystemAttachmentParams struct {
	CharmStorageCountMax int              `db:"charm_storage_count_max"`
	CharmStorageLocation sql.Null[string] `db:"charm_storage_location"`
	CharmStorageReadOnly sql.Null[bool]   `db:"charm_storage_read_only"`
	MachineInstanceID    sql.Null[string] `db:"machine_instance_id"`
	MountPoint           sql.Null[string] `db:"mount_point"`
	ProviderID           sql.Null[string] `db:"provider_id"`
	StoragePoolType      string           `db:"storage_pool_type"`
}

// filesystemProvisioningParams represents the provisioning params for a filesystem from the
// model database.
type filesystemProvisioningParams struct {
	FilesystemID string           `db:"filesystem_id"`
	Type         string           `db:"type"`
	SizeMiB      uint64           `db:"size_mib"`
	VolumeID     sql.Null[string] `db:"volume_id"`
}

// filesystemRemovalParams represents the removal params for a filesystem from
// the model database.
type filesystemRemovalParams struct {
	Type       string         `db:"type"`
	ProviderID string         `db:"provider_id"`
	Obliterate sql.Null[bool] `db:"obliterate_on_cleanup"`
}

// filesystemUUID represents the UUID of a record in the filesystem table.
type filesystemUUID entityUUID

// machineUUID represents the UUID of a record in the machine table.
type machineUUID entityUUID

// netNodeUUID represents the UUID of a record in the network node table.
type netNodeUUID entityUUID

// storageInstanceUUID represents the UUID of a record in the storage_instance table.
type storageInstanceUUID entityUUID

// storageAttachmentUUID represents the UUID of a record in the storage_attachment
// table.
type storageAttachmentUUID entityUUID

// netNodeUUIDRef represents a reference to a network node uuid in a storage
// entity table.
type netNodeUUIDRef struct {
	UUID string `db:"net_node_uuid"`
}

// unitUUID represents the UUID of a record in the unit table.
type unitUUID entityUUID

// volumeAttachmentIDs represents the ids of attachment points to a
// volume attachment. This information includes the volume ID the
// attachment is for. As well as this either the machine or unit name the
// attachment for is included.
type volumeAttachmentIDs struct {
	UUID        string         `db:"uuid"`
	VolumeID    string         `db:"volume_id"`
	MachineName sql.NullString `db:"machine_name"`
	UnitName    sql.NullString `db:"unit_name"`
}

// modelResourceTagInfo represents the information about model resource tag
// information for storage.
type modelResourceTagInfo struct {
	ResourceTags   string `db:"resource_tags"`
	ModelUUID      string `db:"uuid"`
	ControllerUUID string `db:"controller_uuid"`
}

// storagePoolAttribute represent a single attribute from the
// storage_pool_attribute table.
type storagePoolAttribute struct {
	Key   string `db:"key"`
	Value string `db:"value"`
}

// storagePoolAttributeWithUUID represents a single attribute from the
// storage_pool_attribute table including the storage pool UUID. This value
// is useful when expecting multiple storage pool parameters.
//
// If you only expect attributes for a single storage pool then use
// [storagePoolAtribute].
type storagePoolAttributeWithUUID struct {
	StoragePoolUUID string `db:"storage_pool_uuid"`
	Key             string `db:"key"`
	Value           string `db:"value"`
}

// volumeAttachmentPlanLife represents the life of a volume attachment plan in
// the model and the volume id for the volume the attachment plan is for.
type volumeAttachmentPlanLife struct {
	VolumeID string    `db:"volume_id"`
	Life     life.Life `db:"life_id"`
}

type volumeAttachmentPlanLives []volumeAttachmentPlanLife

// Iter provides a seq2 implementation for iterating the values of
// [volumeAttachmentPlanLife].
func (l volumeAttachmentPlanLives) Iter(yield func(string, life.Life) bool) {
	for _, v := range l {
		if !yield(v.VolumeID, v.Life) {
			return
		}
	}
}

// volumeAttachmentPlanUUID represents the UUID of a record in the
// volume_attachment_plan table.
type volumeAttachmentPlanUUID entityUUID

// volumeAttachmentUUID represents the UUID of a record in the volume_attachment
// table.
type volumeAttachmentUUID entityUUID

// volumeAttachmentUUIDs represents a slice of volume attachment UUIDs.
// This type exists so that we can provide sqlair with a named type to process a
// slice of strings.
type volumeAttachmentUUIDs []string

// volumeAttachment represents a volume attachment.
type volumeAttachment struct {
	VolumeID              string    `db:"volume_id"`
	Life                  life.Life `db:"life_id"`
	ReadOnly              bool      `db:"read_only"`
	BlockDeviceName       string    `db:"block_device_name"`
	BlockDeviceUUID       string    `db:"block_device_uuid"`
	BlockDeviceBusAddress string    `db:"block_device_bus_address"`
}

type volumeAttachmentProvisionedInfo struct {
	UUID            string           `db:"uuid"`
	ReadOnly        bool             `db:"read_only"`
	BlockDeviceUUID sql.Null[string] `db:"block_device_uuid"`
}

// volumeID represents the volume id value for a storage volume instance.
type volumeID struct {
	ID string `db:"volume_id"`
}

// volumeLife represents the current life value and volume id for a storage
// volume instance in the model.
type volumeLife struct {
	ID   string    `db:"volume_id"`
	Life life.Life `db:"life_id"`
}

// volumeLives is a convenience type that facilitates transforming a slice
// of [volumeLife] values to a map.
type volumeLives []volumeLife

// Iter provides a seq2 implementation for iterating the values of
// [volumeLives].
func (l volumeLives) Iter(yield func(string, life.Life) bool) {
	for _, v := range l {
		if !yield(v.ID, v.Life) {
			return
		}
	}
}

type volume struct {
	VolumeID   string `db:"volume_id"`
	ProviderID string `db:"provider_id"`
	HardwareID string `db:"hardware_id"`
	WWN        string `db:"wwn"`
	SizeMiB    uint64 `db:"size_mib"`
	Persistent bool   `db:"persistent"`
}

type volumeProvisionedInfo struct {
	UUID       string `db:"uuid"`
	ProviderID string `db:"provider_id"`
	HardwareID string `db:"hardware_id"`
	WWN        string `db:"wwn"`
	SizeMiB    uint64 `db:"size_mib"`
	Persistent bool   `db:"persistent"`
}

type filesystem struct {
	FilesystemID string           `db:"filesystem_id"`
	ProviderID   string           `db:"provider_id"`
	VolumeID     sql.Null[string] `db:"volume_id"`
	SizeMiB      uint64           `db:"size_mib"`
}

type filesystemAttachment struct {
	FilesystemID string `db:"filesystem_id"`
	MountPoint   string `db:"mount_point"`
	ReadOnly     bool   `db:"read_only"`
}

// volumeUUID represents the UUID of a record in the volume table.
type volumeUUID entityUUID

// filesystemTemplate represents the combination of storage directives, charm
// storage and provider type.
type filesystemTemplate struct {
	StorageName  string `db:"storage_name"`
	SizeMiB      uint64 `db:"size_mib"`
	Count        int    `db:"count"`
	MaxCount     int    `db:"count_max"`
	ProviderType string `db:"storage_type"`
	ReadOnly     bool   `db:"read_only"`
	Location     string `db:"location"`
}

// machineVolumeAttachmentProvisioningParams represents the provisioning params
// for a volume attachment onto a machine in the model.
type machineVolumeAttachmentProvisioningParams struct {
	BlockDeviceUUID sql.Null[string] `db:"block_device_uuid"`
	ProviderType    string           `db:"provider_type"`
	ReadOnly        sql.Null[bool]   `db:"read_only"`
	VolumeID        string           `db:"volume_id"`
}

// machineVolumeProvisioningParams represents the provisioning params for a
// volume that is to be attached or is attached to a machine in the model.
type machineVolumeProvisioningParams struct {
	ProviderType         string           `db:"provider_type"`
	RequestedSizeMiB     uint64           `db:"requested_size_mib"`
	SizeMiB              uint64           `db:"size_mib"`
	StorageID            string           `db:"storage_id"`
	StorageName          string           `db:"storage_name"`
	StoragePoolUUID      string           `db:"storage_pool_uuid"`
	StorageUnitOwnerName sql.Null[string] `db:"storage_unit_owner_name"`
	VolumeID             string           `db:"volume_id"`
}

// volumeProvisioningParams represents the provisioning params for a volume from the model
// database.
type volumeProvisioningParams struct {
	VolumeID             string `db:"volume_id"`
	Type                 string `db:"type"`
	RequestedSizeMiB     uint64 `db:"requested_size_mib"`
	VolumeAttachmentUUID string `db:"volume_attachment_uuid"`
}

// volumeRemovalParams represents the removal params for a volume from the model
// database.
type volumeRemovalParams struct {
	Type       string         `db:"type"`
	ProviderID string         `db:"provider_id"`
	Obliterate sql.Null[bool] `db:"obliterate_on_cleanup"`
}

// volumeAttachmentParams represents the attachment params for a volume
// attachment from the model database.
type volumeAttachmentParams struct {
	Type        string `db:"type"`
	MachineName string `db:"machine_name"`
	InstanceID  string `db:"instance_id"`
	ProviderID  string `db:"provider_id"`
	ReadOnly    bool   `db:"read_only"`
}

// storageNameAttributes represents each key/value attribute for a given storage
// derived from the provider/pool used to provisioner the storage.
type storageNameAttributes struct {
	StorageName string `db:"storage_name"`
	Key         string `db:"key"`
	Value       string `db:"value"`
}

// filesystemProvisionedInfo is used to set the provisioned info for a
// filesystem.
type filesystemProvisionedInfo struct {
	UUID       string `db:"uuid"`
	ProviderID string `db:"provider_id"`
	SizeMiB    uint64 `db:"size_mib"`
}

// filesystemAttachmentProvisionedInfo is used to set the provisioned info for
// a filesystem attachment.
type filesystemAttachmentProvisionedInfo struct {
	UUID       string `db:"uuid"`
	MountPoint string `db:"mount_point"`
	ReadOnly   bool   `db:"read_only"`
}

type storageID struct {
	ID string `db:"storage_id"`
}

type storageIDs []storageID

type unitUUIDRef struct {
	UUID string `db:"unit_uuid"`
}

type storageAttachmentIdentifier struct {
	StorageInstanceUUID string `db:"storage_instance_uuid"`
	UnitUUID            string `db:"unit_uuid"`
}

type volumeAttachmentInfo struct {
	UUID              string    `db:"uuid"`
	NetNodeUUID       string    `db:"net_node_uuid"`
	StorageVolumeUUID string    `db:"storage_volume_uuid"`
	Life              life.Life `db:"life_id"`
}

type volumeAttachmentPlan struct {
	UUID         string    `db:"uuid"`
	Life         life.Life `db:"life_id"`
	DeviceTypeID int       `db:"device_type_id"`
}

type volumeAttachmentPlanAttr struct {
	AttachmentPlanUUID string `db:"attachment_plan_uuid"`
	Key                string `db:"key"`
	Value              string `db:"value"`
}

type storageAttachmentInfo struct {
	StorageAttachmentUUID string    `db:"storage_attachment_uuid"`
	KindID                int       `db:"storage_kind_id"`
	Life                  life.Life `db:"life_id"`
	FilesystemMountPoint  string    `db:"mount_point"`
	BlockDeviceUUID       string    `db:"block_device_uuid"`
}
