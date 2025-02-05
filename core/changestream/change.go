// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package changestream

// ChangeType represents the type of change.
// The changes are bit flags so that they can be combined.
type ChangeType int

// Change types represent the internal type of change that has occurred.
// These are not exposed publicly, this is by design, to prevent watchers
// expecting a unique create event over a update event. This can lead to
// faulty assumptions about the underlying data.
//
// Although, create and update are two separate types, they are combined
// to represent a change in the underlying type. A update will always supersede
// an update, and it is indicative of a change in the underlying type. A delete
// is a separate type as it represents a deletion of the underlying type and
// will supersede a create or update.
const (
	// create represents a new row in the database.
	create ChangeType = 1 << iota
	// update represents an update to an existing row in the database.
	update
	// delete represents a row that has been deleted from the database.
	delete
)

// Changed returns if the underlying type has changed. This will encompass
// if a row has been created or updated. There is no distinction between the
// two types of changes, only that the underlying type has changed.
const Changed = create | update

// Deleted returns if the underlying type has been deleted. This will encompass
// if a row has been deleted from the database.
const Deleted = delete

// All returns all the types of changes that can be represented.
const All = create | update | delete

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
