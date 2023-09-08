// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

// These structs represent the persistent cloud entity schema in the database.

type CloudType struct {
	ID   int    `db:"id"`
	Type string `db:"type"`
}

type AuthTypes []AuthType

type AuthType struct {
	ID   int    `db:"id"`
	Type string `db:"type"`
}

type Cloud struct {
	// ID holds the cloud document key.
	ID string `db:"uuid"`

	// Name holds the cloud name.
	Name string `db:"name"`

	// Type holds the cloud type reference.
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

type CloudAuthType struct {
	// ID holds the cloud auth type document key.
	ID string `db:"uuid"`

	// CloudUUID holds the cloud reference.
	CloudUUID string `db:"cloud_uuid"`

	// AuthTypeID holds the auth type reference.
	AuthTypeID string `db:"auth_type_id"`
}

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

type CloudCACert struct {
	// ID holds the cloud ca cert document key.
	ID string `db:"uuid"`

	// CloudUUID holds the cloud reference.
	CloudUUID string `db:"cloud_uuid"`

	// CACert holds the ca cert.
	CACert string `db:"ca_cert"`
}
