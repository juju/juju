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
	// All represents any change to the namespace of interest.
	All = Create | Update | Delete
)

// ChangeEvent represents a new change set via the changestream.
type ChangeEvent interface {
	// Type returns the type of change (create, update, delete).
	Type() ChangeType
	// Namespace returns the namespace of the change. This is normally the
	// table name.
	Namespace() string
	// Changed returns the changed value of event. This logically can be
	// the primary key of the row that was changed or the field of the change
	// that was changed.
	Changed() string

	// Discriminator returns the discriminator value of event.
	// This is expected to be an immutable column which can be
	// used to filter to event.
	Discriminator() string
}

// Term represents a set of changes that are bounded by a coalesced set.
// The notion of a term are a set of changes that can be run one at a time
// asynchronously. Allowing changes within a given term to be signaled of a
// change independently of one another.
// Once a change within a term has been completed, only at that point
// is another change processed, until all changes are exhausted.
type Term interface {
	// Changes returns the changes that are part of the term.
	Changes() []ChangeEvent

	// Done signals that the term has been completed. Empty signals that
	// the term was empty and no changes were processed. This is useful to
	// help determine if more changes are available to be processed.
	// Abort is used to signal that setting the empty value should be aborted
	// and the term should be considered incomplete and done.
	Done(empty bool, abort <-chan struct{})
}
