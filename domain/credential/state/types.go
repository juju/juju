// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

// These structs represent the persistent cloud credential entity schema in the database.

type CloudCredential struct {
	// ID holds the cloud credential document key.
	ID string `db:"uuid"`

	// CloudUUID holds the cloud reference.
	CloudUUID string `db:"cloud_uuid"`

	// AuthTypeID holds the auth type reference.
	AuthTypeID int `db:"auth_type_id"`

	// Name is the name of the credential.
	Name string `db:"name"`

	// Owner is the user who owns the credential.
	// TODO(wallyworld) - this will be a user reference when users are added.
	OwnerUUID string `db:"owner_uuid"`

	// Revoked is true if the credential has been revoked.
	Revoked bool `db:"revoked"`

	// Invalid stores flag that indicates if a credential is invalid.
	// Note that the credential is valid:
	//  * if the flag is explicitly set to 'false'; or
	//  * if the flag is not set at all, as will be the case for
	//    new inserts or credentials created with previous Juju versions. In
	//    this case, we'd still read it as 'false' and the credential validity
	//    will be interpreted correctly.
	// This flag will need to be explicitly set to 'true' for a credential
	// to be considered invalid.
	Invalid bool `db:"invalid"`

	// InvalidReason contains the reason why the credential was marked as invalid.
	// This can range from cloud messages such as an expired credential to
	// commercial reasons set via CLI or api calls.
	InvalidReason string `db:"invalid_reason"`
}
