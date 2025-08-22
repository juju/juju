// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

// permission represents a permission in the system.
type permission struct {
	// UUID is the unique identifier for the permission.
	UUID string `db:"uuid"`

	// GrantOn is the unique identifier of the permission target.
	// A name or UUID depending on the ObjectType.
	GrantOn string `db:"grant_on"`

	// GrantTo is the unique identifier of the user the permission
	// is granted to.
	GrantTo string `db:"grant_to"`

	// AccessType is a string version of core permission AccessType.
	AccessType string `db:"access_type"`

	// ObjectType is a string version of core permission ObjectType.
	ObjectType string `db:"object_type"`
}

// user represents the uuid and disabled status of a user.
type user struct {
	UUID     string `db:"uuid"`
	Disabled bool   `db:"disabled"`
}

// name is an agnostic container for a `name` column.
type name struct {
	Name string `db:"name"`
}

// entityUUID is an agnostic container for a `uuid` column.
type entityUUID struct {
	UUID string `db:"uuid"`
}

// offerUser represents a user and their access to a specific offer.
type offerUser struct {
	OfferUUID   string `db:"grant_on"`
	Name        string `db:"name"`
	DisplayName string `db:"display_name"`
	Access      string `db:"access_type"`
}
