// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation

// Status describes the status of a relation.
type Status string

const (
	// Joined is the normal status for a healthy, alive relation.
	Joined Status = "joined"

	// Broken is the status for when a relation life goes to Dead.
	Broken Status = "broken"

	// Suspended is used to signify that a relation is temporarily broken pending
	// action to resume it.
	Suspended Status = "suspended"

	// Error is used to signify that the relation is in an error state.
	Error Status = "error"
)
