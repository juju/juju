// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

// These structs represent the persistent cloud entity schema in the database.

// CloudType represents a single row from the cloud_type table.
type CloudType struct {
	ID   int    `db:"id"`
	Type string `db:"type"`
}

type AuthTypes []AuthType

// AuthTypeIds represents a list of ids from the database.
type AuthTypeIds []int

// AuthType represents a single row from the auth_type table.
type AuthType struct {
	ID   int    `db:"id"`
	Type string `db:"type"`
}

// Cloud represents a row from the cloud table joined with the cloud_type and
// auth_type tables along with a column built form various tables signalling if
// the cloud is a controller.
type Cloud struct {
	// ID holds the cloud document key.
	ID string `db:"uuid"`

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

// CloudWithAuthType represents a row from v_cloud_auth that can be decoded into
// this type.
type CloudWithAuthType struct {
	// ID holds the cloud document key.
	ID string `db:"uuid"`

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

// Attrs stores a list of attributes corresponding to keys in the cloud_defaults
// table.
type Attrs []string

// CloudDefaults represents a single row from the cloud__defaults table.
type CloudDefaults struct {
	// ID holds the cloud uuid.
	ID string `db:"cloud_uuid"`

	// Key is the key value.
	Key string `db:"key"`

	// Value is the value associated with key.
	Value string `db:"value"`
}

type CloudAuthType struct {
	// CloudUUID holds the cloud reference.
	CloudUUID string `db:"cloud_uuid"`

	// AuthTypeID holds the auth type reference.
	AuthTypeID int `db:"auth_type_id"`
}

// RegionNames stores a list of regions names corresponding to names the
// cloud_region table.
type RegionNames []string

// CloudRegion represents a row in the cloud_region table.
type CloudRegion struct {
	// ID holds the cloud region document key.
	ID string `db:"uuid"`

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

// CloudRegionDefaults represents a single row from the cloud_region_defaults
// table.
type CloudRegionDefaults struct {
	// ID holds the cloud region uuid.
	ID string `db:"region_uuid"`

	// Key is the key value.
	Key string `db:"key"`

	// Value is the value associated with key.
	Value string `db:"value"`
}

// CloudRegionDefaultValue represents a single row from cloud_region_defaults
// when joined with cloud_region on cloud_region_uuid. It is used when
// deserializing defaults for all regions of a cloud.
type CloudRegionDefaultValue struct {
	// Name is the name of the region.
	Name string `db:"name"`

	// Key is the key value.
	Key string `db:"key"`

	// Value is the value associated with key.
	Value string `db:"value"`
}

// CloudUUIDs is a slice of uuids from the database.
type CloudUUIDs []string

// CloudCACert represents a single row from the cloud_ca_cert table.
type CloudCACert struct {
	// CloudUUID holds the cloud reference.
	CloudUUID string `db:"cloud_uuid"`

	// CACert holds the ca cert.
	CACert string `db:"ca_cert"`
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

	// Access is the type of access for this user for the
	// GrantOn value.
	Access string `db:"access"`
}
