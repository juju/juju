// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"database/sql/driver"

	"github.com/juju/juju/internal/database"
)

// publicKey represents a single row from the user public key table.
type publicKey struct {
	Fingerprint string `db:"fingerprint"`
	PublicKey   string `db:"public_key"`
}

// publicKeyData represents a single raw public key from the user public key
// table.
type publicKeyData struct {
	PublicKey string `db:"public_key"`
}

// modelAuthorizedKey represents a single row from the model_authorized_keys
// table.
type modelAuthorizedKey struct {
	UserPublicSSHKeyId int64  `db:"user_public_ssh_key_id"`
	ModelUUID          string `db:"model_uuid"`
}

// modelUUIDValue represents a model id for associating public keys with.
type modelUUIDValue struct {
	UUID string `db:"model_uuid"`
}

// userPublicKey represents a single user public key within the controller.
type userPublicKey struct {
	UserName  string `db:"name"`
	PublicKey string `db:"public_key"`
}

// userPublicKeyId represents a single raw user public key id from the database.
type userPublicKeyId struct {
	Id int64 `db:"id"`
}

// userPublicKeyIds represents an aggregate slice of [userPublicKeyId] for
// performing bulk in operations.
type userPublicKeyIds []userPublicKeyId

// userPublicKeyInsert describes the data input needed for inserting new public
// keys for a user.
type userPublicKeyInsert struct {
	Comment                  string            `db:"comment"`
	FingerprintHashAlgorithm string            `db:"algorithm"`
	Fingerprint              database.NullBlob `db:"fingerprint"`
	PublicKey                database.NullBlob `db:"public_key"`
	UserId                   string            `db:"user_uuid"`
}

// userUUIDValue represents a user id for associating public keys with.
type userUUIDValue struct {
	UUID string `db:"user_uuid"`
}

// Value returns the user id implementing the [driver.Valuer] interface.
func (u userPublicKeyId) Value() (driver.Value, error) {
	return u.Id, nil
}
