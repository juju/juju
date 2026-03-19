// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"database/sql"
	"time"
)

// count represents the result of performing an aggregate count operation in
// sql.
type count struct {
	Count int `db:"count"`
}

// entityUUID represents the UUID of a storage entity in the model.
type entityUUID struct {
	UUID string `db:"uuid"`
}

// nameAndUUID is an agnostic container for a `name` and `uuid`
// column combination.
type nameAndUUID struct {
	Name string `db:"name"`
	UUID string `db:"uuid"`
}

// name is an agnostic container for a `name` value.
type name struct {
	Name string `db:"name"`
}

// idAndKind represents an agnostic container for `id` and `kind`
// column combination.
type idAndKind struct {
	ID   int    `db:"id"`
	Kind string `db:"kind"`
}

// machineUUIDs is a slice type of string representing machineUUIDs in the
// model.
type machineUUIDs []string

type uuids []string

// machineAndUnitNetNodeUUID represents names and net node uuid
// for machine or unit combinations where the data is gathered in
// a single query.
type machineAndUnitNetNodeUUID struct {
	MachineName        sql.NullString `db:"machine_name"`
	MachineNetNodeUUID sql.NullString `db:"machine_net_node_uuid"`
	UnitName           sql.NullString `db:"unit_name"`
	UnitNetNodeUUID    sql.NullString `db:"unit_net_node_uuid"`
}

// storageInstanceID represents the storage instance storage_id column for a
// row in the storage_instance table.
type storageInstanceID struct {
	ID string `db:"storage_id"`
}

type storageInstanceUUIDAndID struct {
	UUID string `db:"uuid"`
	ID   string `db:"storage_id"`
}

type storageInstanceIDs []string

// dbModelStoragePool represents a single row from the model_storage_pool table.
type dbModelStoragePool struct {
	StoragePoolUUID string `db:"storage_pool_uuid"`
	StorageKindID   int    `db:"storage_kind_id"`
}

// dbAggregateCount is a type to store the result for counting the number of
// rows returned by a select query.
type dbAggregateCount struct {
	Count int `db:"count"`
}

// storageInstanceUUID represents the UUID of a storage instance in the model.
type storageInstanceUUID entityUUID

// unitUUID represents the UUID of a unit in the model.
type unitUUID entityUUID

// importStorageInstance represents a storage_instance.
type importStorageInstance struct {
	UUID            string `db:"uuid"`
	CharmName       string `db:"charm_name"`
	StorageName     string `db:"storage_name"`
	StorageID       string `db:"storage_id"`
	StorageKindID   int    `db:"storage_kind_id"`
	LifeID          int    `db:"life_id"`
	StoragePoolUUID string `db:"storage_pool_uuid"`
	RequestedSize   uint64 `db:"requested_size_mib"`
}

// importStorageUnitOwner represents a storage_unit_owner.
type importStorageUnitOwner struct {
	StorageInstanceUUID string `db:"storage_instance_uuid"`
	UnitUUID            string `db:"unit_uuid"`
}

type importStorageAttachment struct {
	UUID                string `db:"uuid"`
	StorageInstanceUUID string `db:"storage_instance_uuid"`
	UnitUUID            string `db:"unit_uuid"`
	LifeID              int    `db:"life_id"`
}

type importStorageFilesystem struct {
	UUID       string `db:"uuid"`
	ID         string `db:"filesystem_id"`
	LifeID     int    `db:"life_id"`
	ScopeID    int    `db:"provision_scope_id"`
	ProviderID string `db:"provider_id"`
	SizeInMiB  uint64 `db:"size_mib"`
}

type importStorageInstanceFilesystem struct {
	StorageInstanceUUID string `db:"storage_instance_uuid"`
	FilesystemUUID      string `db:"storage_filesystem_uuid"`
}

type importStorageFilesystemAttachment struct {
	UUID           string `db:"uuid"`
	FilesystemUUID string `db:"storage_filesystem_uuid"`
	NetNodeUUID    string `db:"net_node_uuid"`
	ScopeID        int    `db:"provision_scope_id"`
	LifeID         int    `db:"life_id"`
	MountPoint     string `db:"mount_point"`
	ProviderID     string `db:"provider_id"`
	ReadOnly       bool   `db:"read_only"`
}

// importStorageVolume represents the data contained in a
// storage volume which may be inserted on model import.
type importStorageVolume struct {
	UUID             string `db:"uuid"`
	VolumeID         string `db:"volume_id"`
	LifeID           int    `db:"life_id"`
	ProvisionScopeID int    `db:"provision_scope_id"`
	ProviderID       string `db:"provider_id"`
	SizeMiB          uint64 `db:"size_mib"`
	HardwareID       string `db:"hardware_id"`
	WWN              string `db:"wwn"`
	Persistent       bool   `db:"persistent"`
}

// importStorageInstanceVolume represents a pairing of a storage
// instance and a volume on model import.
type importStorageInstanceVolume struct {
	StorageInstanceUUID string `db:"storage_instance_uuid"`
	VolumeUUID          string `db:"storage_volume_uuid"`
}

// blockDevice represents a block device.
type blockDevice struct {
	UUID        string `db:"uuid"`
	NetNodeUUID string `db:"net_node_uuid"`

	Name string `db:"name"`

	HardwareId string `db:"hardware_id"`
	WWN        string `db:"wwn"`
	BusAddress string `db:"bus_address"`
	SerialId   string `db:"serial_id"`

	SizeMiB            uint64 `db:"size_mib"`
	FilesystemLabel    string `db:"filesystem_label"`
	HostFilesystemUUID string `db:"host_filesystem_uuid"`
	FilesystemType     string `db:"filesystem_type"`
	InUse              bool   `db:"in_use"`
	MountPoint         string `db:"mount_point"`
}

// deviceLink represents a block device's device link.
type deviceLink struct {
	BlockDeviceUUID string `db:"block_device_uuid"`
	NetNodeUUID     string `db:"net_node_uuid"`
	Name            string `db:"name"`
}

// importStorageVolumeAttachment represents a storage volume attachment
// on model import.
type importStorageVolumeAttachment struct {
	UUID              string `db:"uuid"`
	StorageVolumeUUID string `db:"storage_volume_uuid"`
	NetNodeUUID       string `db:"net_node_uuid"`
	LifeID            int    `db:"life_id"`
	ProvisionScopeID  int    `db:"provision_scope_id"`
	ProviderID        string `db:"provider_id"`
	BlockDeviceUUID   string `db:"block_device_uuid"`
	ReadOnly          bool   `db:"read_only"`
}

// importStorageVolumeAttachmentPlan represents a storage volume attachment
// plan on model import.
type importStorageVolumeAttachmentPlan struct {
	UUID              string        `db:"uuid"`
	DeviceTypeID      sql.NullInt64 `db:"device_type_id"`
	LifeID            int           `db:"life_id"`
	NetNodeUUID       string        `db:"net_node_uuid"`
	ProvisionScopeID  int           `db:"provision_scope_id"`
	StorageVolumeUUID string        `db:"storage_volume_uuid"`
}

// importStorageVolumePlanAttribute represents a storage volume attachment plan
// attribute on model import.
type importStorageVolumePlanAttribute struct {
	PlanUUID string `db:"attachment_plan_uuid"`
	Key      string `db:"key"`
	Value    string `db:"value"`
}

// modelResourceTagInfo represents the information about model resource tag
// information for storage.
type modelResourceTagInfo struct {
	ResourceTags   string `db:"resource_tags"`
	ModelUUID      string `db:"uuid"`
	ControllerUUID string `db:"controller_uuid"`
}

// insertStorageInstance represents the data needed to insert a new storage
// instance into the storage_instance table.
type insertStorageInstance struct {
	UUID             string `db:"uuid"`
	StorageName      string `db:"storage_name"`
	StorageKindID    int    `db:"storage_kind_id"`
	StorageID        string `db:"storage_id"`
	LifeID           int    `db:"life_id"`
	StoragePoolUUID  string `db:"storage_pool_uuid"`
	RequestedSizeMiB uint64 `db:"requested_size_mib"`
}

// insertStorageFilesystem represents the data needed to insert a new
// filesystem into the storage_filesystem table.
type insertStorageFilesystem struct {
	UUID             string `db:"uuid"`
	FilesystemID     string `db:"filesystem_id"`
	LifeID           int    `db:"life_id"`
	ProvisionScopeID int    `db:"provision_scope_id"`
	ProviderID       string `db:"provider_id"`
	SizeMiB          uint64 `db:"size_mib"`
}

// insertStorageInstanceFilesystem represents the data needed to link a storage
// instance to a filesystem in the storage_instance_filesystem table.
type insertStorageInstanceFilesystem struct {
	StorageInstanceUUID   string `db:"storage_instance_uuid"`
	StorageFilesystemUUID string `db:"storage_filesystem_uuid"`
}

// insertStorageFilesystemStatus represents data needed to set a filesystem's
// status in the storage_filesystem_status table.
type insertStorageFilesystemStatus struct {
	StorageFilesystemUUID string    `db:"filesystem_uuid"`
	StatusID              int       `db:"status_id"`
	Message               string    `db:"message"`
	UpdatedAt             time.Time `db:"updated_at"`
}

// insertStorageVolume represents the data needed to insert a new volume into
// the storage_volume table.
type insertStorageVolume struct {
	UUID             string `db:"uuid"`
	VolumeID         string `db:"volume_id"`
	LifeID           int    `db:"life_id"`
	ProvisionScopeID int    `db:"provision_scope_id"`
	ProviderID       string `db:"provider_id"`
	SizeMiB          uint64 `db:"size_mib"`
	HardwareID       string `db:"hardware_id"`
	WWN              string `db:"wwn"`
	Persistent       bool   `db:"persistent"`
}

// insertStorageInstanceVolume represents the data needed to link a storage
// instance to a volume in the storage_instance_volume table.
type insertStorageInstanceVolume struct {
	StorageInstanceUUID string `db:"storage_instance_uuid"`
	StorageVolumeUUID   string `db:"storage_volume_uuid"`
}

// insertStorageVolumeStatus represents the data needed to set a volume's status
// in the storage_volume_status table.
type insertStorageVolumeStatus struct {
	StorageVolumeUUID string    `db:"volume_uuid"`
	StatusID          int       `db:"status_id"`
	Message           string    `db:"message"`
	UpdatedAt         time.Time `db:"updated_at"`
}
