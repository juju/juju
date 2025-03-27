// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	corecloud "github.com/juju/juju/core/cloud"
	coremodel "github.com/juju/juju/core/model"
)

// These structs represent the persistent cloud entity schema in the database.

type modelUUID struct {
	UUID coremodel.UUID `db:"uuid"`
}

type modelCloudRegion struct {
	CloudUUID       corecloud.UUID `db:"cloud_uuid"`
	CloudRegionName string         `db:"cloud_region_name"`
}

// cloudType represents a single row from the cloud_type table.
type cloudType struct {
	ID   int    `db:"id"`
	Type string `db:"type"`
}

// authTypeIds represents a list of ids from the database.
type authTypeIds []int

// authType represents a single row from the auth_type table.
type authType struct {
	ID   int    `db:"id"`
	Type string `db:"type"`
}

// dbCloud represents a row from the cloud table joined with the cloud_type and
// auth_type tables along with a column built form various tables signalling if
// the cloud is a controller.
type dbCloud struct {
	// UUID holds the cloud's unique identifier.
	UUID string `db:"uuid"`

	// Name holds the cloud name.
	Name string `db:"name"`

	// Type holds the cloud type.
	Type string `db:"cloud_type"`

	// TypeID holds the unique cloud type id.
	TypeID int `db:"cloud_type_id"`

	// Endpoint holds the cloud's primary endpoint URL.
	Endpoint string `db:"endpoint"`

	// IdentityEndpoint holds the cloud's identity endpoint URK.
	IdentityEndpoint string `db:"identity_endpoint"`

	// StorageEndpoint holds the cloud's storage endpoint URL.
	StorageEndpoint string `db:"storage_endpoint"`

	// SkipTLSVerify indicates if the client should skip cert validation.
	SkipTLSVerify bool `db:"skip_tls_verify"`

	// IsControllerCloud indicates if the cloud is hosting the controller model.
	IsControllerCloud bool `db:"is_controller_cloud"`
}

// cloudWithAuthType represents a row from v_cloud_auth that can be decoded into
// this type.
type cloudWithAuthType struct {
	// ID holds the cloud document key.
	UUID string `db:"uuid"`

	// Name holds the cloud name.
	Name string `db:"name"`

	// Type holds the cloud type.
	Type string `db:"cloud_type"`

	// TypeID holds the unique cloud type id.
	TypeID int `db:"cloud_type_id"`

	// Endpoint holds the cloud's primary endpoint URL.
	Endpoint string `db:"endpoint"`

	// IdentityEndpoint holds the cloud's identity endpoint URK.
	IdentityEndpoint string `db:"identity_endpoint"`

	// StorageEndpoint holds the cloud's storage endpoint URL.
	StorageEndpoint string `db:"storage_endpoint"`

	// SkipTLSVerify indicates if the client should skip cert validation.
	SkipTLSVerify bool `db:"skip_tls_verify"`

	// IsControllerCloud indicates if the cloud is hosting the controller model.
	IsControllerCloud bool `db:"is_controller_cloud"`

	// AuthType describes one of the auth types supported by this cloud.
	AuthType string `db:"auth_type"`
}

type cloudAuthType struct {
	// CloudUUID holds the cloud reference.
	CloudUUID string `db:"cloud_uuid"`

	// AuthTypeID holds the auth type reference.
	AuthTypeID int `db:"auth_type_id"`
}

// regionNames stores a list of regions names corresponding to names the
// cloud_region table.
type regionNames []string

// cloudRegion represents a row in the cloud_region table.
type cloudRegion struct {
	// UUID holds the cloud region's unique identifier.
	UUID string `db:"uuid"`

	// CloudUUID holds the cloud reference.
	CloudUUID string `db:"cloud_uuid"`

	// Name is the name of the region.
	Name string `db:"name"`

	// Endpoint is the region's primary endpoint URL.
	Endpoint string `db:"endpoint"`

	// IdentityEndpoint is the region's identity endpoint URL.
	IdentityEndpoint string `db:"identity_endpoint"`

	// StorageEndpoint is the region's storage endpoint URL.
	StorageEndpoint string `db:"storage_endpoint"`
}

// uuids is a slice of uuids from the database.
type uuids []string

// cloudCACert represents a single row from the cloud_ca_cert table.
type cloudCACert struct {
	// CloudUUID holds the cloud reference.
	CloudUUID string `db:"cloud_uuid"`

	// CACert holds the ca cert.
	CACert string `db:"ca_cert"`
}

// cloudID represents only the name and UUID of a cloud
type cloudID struct {
	// UUID holds the cloud's unique identifier.
	UUID string `db:"uuid"`
	// Name holds the cloud name.
	Name string `db:"name"`
}

// cloudName represents only the name of a cloud
type dbCloudName struct {
	// Name holds the cloud name.
	Name string `db:"name"`
}

// dbAddUserPermission represents a permission in the system where the values
// overlap with corepermission.Permission.
type dbAddUserPermission struct {
	// UUID is the unique identifier for the permission.
	UUID string `db:"uuid"`

	// GrantOn is the unique identifier of the permission target.
	// A name or UUID depending on the ObjectType.
	GrantOn string `db:"grant_on"`

	// Name is the name of the user that the permission is granted to.
	Name string `db:"name"`

	// AccessType is the type of access for this user for the
	// GrantOn value.
	AccessType string `db:"access_type"`

	// ObjectType is the type of the object for this user for the
	// GrantOn value.
	ObjectType string `db:"object_type"`
}
