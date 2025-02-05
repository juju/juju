// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import "github.com/juju/juju/core/changestream"

const (
	// Create represents a new row in the database.
	Create changestream.ChangeType = 1 << iota
	// Update represents an update to an existing row in the database.
	Update
	// Delete represents a row that has been deleted from the database.
	Delete
	// All represents all the types of changes that can be represented.
	All = Create | Update | Delete
)
