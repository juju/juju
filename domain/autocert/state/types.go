// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

// dbAutocert is a named certificate to be stored in the database.
type dbAutocert struct {
	// UUID is the uuid of the autocert.
	UUID string `db:"uuid"`

	// Name is the autocert name. It uniquely identifies the certificate.
	Name string `db:"name"`

	// Data represents the binary (encoded) contents of the autocert.
	Data string `db:"data"`

	// Encoding is the autocert cache encoding id from the
	// autocert_cache_encoding table.
	Encoding int `db:"encoding"`
}
