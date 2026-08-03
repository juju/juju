// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import "database/sql"

// machineRow is the sqlair input/output type for the machine query.
type machineRow struct {
	UUID      string           `db:"uuid"`
	Name      string           `db:"name"`
	OSName    string           `db:"os_name"`
	Channel   string           `db:"channel"`
	Directive sql.Null[string] `db:"directive"`
}

// machineUUIDParam is used for parameterising queries that need a machine UUID.
type machineUUIDParam struct {
	UUID string `db:"uuid"`
}

// countResult is a row type for COUNT queries.
type countResult struct {
	Count int `db:"count"`
}

// constraintRow maps to v_machine_constraint view columns.
type constraintRow struct {
	Arch             sql.NullString  `db:"arch"`
	CPUCores         sql.Null[int64] `db:"cpu_cores"`
	CPUPower         sql.Null[int64] `db:"cpu_power"`
	Mem              sql.Null[int64] `db:"mem"`
	RootDisk         sql.Null[int64] `db:"root_disk"`
	RootDiskSource   sql.NullString  `db:"root_disk_source"`
	InstanceRole     sql.NullString  `db:"instance_role"`
	InstanceType     sql.NullString  `db:"instance_type"`
	ContainerType    sql.NullString  `db:"container_type"`
	VirtType         sql.NullString  `db:"virt_type"`
	AllocatePublicIP sql.NullBool    `db:"allocate_public_ip"`
	ImageID          sql.NullString  `db:"image_id"`
	IPFamily         sql.NullString  `db:"ip_family"`
	SpaceName        sql.NullString  `db:"space_name"`
	SpaceExclude     sql.NullBool    `db:"space_exclude"`
	Tag              sql.NullString  `db:"tag"`
	Zone             sql.NullString  `db:"zone"`
}

// unitRow maps to the unit query results.
type unitRow struct {
	Name          string         `db:"name"`
	PrincipalUUID sql.NullString `db:"principal_uuid"`
}

// unitUUIDName maps a unit UUID to its name for resolution.
type unitUUIDName struct {
	UUID string `db:"uuid"`
	Name string `db:"name"`
}

// endpointBindingRow maps to the application endpoint binding query.
type endpointBindingRow struct {
	Endpoint  string           `db:"endpoint"`
	SpaceUUID sql.Null[string] `db:"space_uuid"`
}

// appRow holds application identity data for endpoint lookups.
type appRow struct {
	UUID      string `db:"uuid"`
	Name      string `db:"name"`
	SpaceUUID string `db:"space_uuid"`
}

// appUUIDParam is used for parameterising queries by application UUID.
type appUUIDParam struct {
	UUID string `db:"uuid"`
}

// storagePoolRow holds the storage pool provider from a query.
type storagePoolRow struct {
	Provider string `db:"type"`
}

// storagePoolAttrRow holds a storage pool attribute key-value pair.
type storagePoolAttrRow struct {
	Key   string `db:"key"`
	Value string `db:"value"`
}

// storagePoolNameParam is a parameter type for storage pool name queries.
type storagePoolNameParam struct {
	Name string `db:"name"`
}

// spaceSubnetRow maps to v_space_subnet view columns.
type spaceSubnetRow struct {
	SpaceUUID        string `db:"uuid"`
	SpaceName        string `db:"name"`
	SpaceProviderID  string `db:"provider_id"`
	SubnetUUID       string `db:"subnet_uuid"`
	SubnetCIDR       string `db:"subnet_cidr"`
	SubnetProviderID string `db:"subnet_provider_id"`
	AvailabilityZone string `db:"subnet_az"`
}

// modelInfoRow maps to the model table for identity info.
type modelInfoRow struct {
	Name        string `db:"name"`
	CloudName   string `db:"cloud_name"`
	CloudType   string `db:"cloud_type"`
	CloudRegion string `db:"cloud_region"`
}

// modelConfigRow holds a model config key-value pair.
type modelConfigRow struct {
	Key   string `db:"key"`
	Value string `db:"value"`
}

// modelConfigKeys is a slice type for IN-clause parameters.
type modelConfigKeys []string

// volumeRow holds volume provisioning data from a query.
type volumeRow struct {
	UUID             string `db:"uuid"`
	VolumeID         string `db:"volume_id"`
	RequestedSizeMiB int64  `db:"requested_size_mib"`
	Provider         string `db:"provider"`
	StorageName      string `db:"storage_name"`
	StorageID        string `db:"storage_id"`
}

// volumeUUIDParam is used for parameterising queries by volume UUID.
type volumeUUIDParam struct {
	UUID string `db:"uuid"`
}

// volumeAttachmentRow holds volume attachment query results.
type volumeAttachmentRow struct {
	VolumeUUID string `db:"volume_uuid"`
	VolumeID   string `db:"volume_id"`
	Provider   string `db:"provider"`
	ReadOnly   bool   `db:"read_only"`
	ProviderID string `db:"provider_id"`
}

// volumeOwnerRow holds the unit name that owns a storage volume.
type volumeOwnerRow struct {
	UnitName string `db:"unit_name"`
}
