// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

// userPublicKeyInsert describes the data input needed for inserting new public
// keys for a user.
type userPublicKeyInsert struct {
	Comment                  string `db:"comment"`
	FingerprintHashAlgorithm string `db:"algorithm"`
	Fingerprint              string `db:"fingerprint"`
	PublicKey                string `db:"public_key"`
	UserId                   string `db:"user_id"`
}

// publicKey represents a single row the user public key table.
type publicKey struct {
	PublicKey string `db:"public_key"`
}

// userId represents a user id for associating public keys with.
type userId struct {
	UserId string `db:"user_id"`
}
