// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

// These structs represent the persistent cloud credential entity schema in the database.

type credentialWithAttribute struct {
	ID             string `db:"uuid"`
	CloudName      string `db:"cloud_name"`
	AuthType       string `db:"auth_type"`
	AttributeKey   string `db:"attribute_key"`
	AttributeValue string `db:"attribute_value"`
}
