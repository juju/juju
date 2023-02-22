// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package changestream

// ChangeType represents the type of change.
// The changes are bit flags so that they can be combined.
type ChangeType int

const (
	// Create represents a new row in the database.
	Create ChangeType = 1 << iota
	// Update represents an update to an existing row in the database.
	Update
	// Delete represents a row that has been deleted from the database.
	Delete
)

// ChangeEvent represents a new change set via the changestream.
type ChangeEvent interface {
	// Type returns the type of change (create, update, delete).
	Type() ChangeType
	// Namespace returns the namespace of the change. This is normally the
	// table name.
	Namespace() string
	// ChangedUUID returns the entity UUID of the change.
	ChangedUUID() string
}
