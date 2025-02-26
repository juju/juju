// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import "time"

// removal represents a record in the removal table
type removal struct {
	// UUID uniquely identifies this removal job.
	UUID string `db:"uuid"`
	// RemovalTypeID indicates the type of entity that this removal job is for.
	RemovalTypeID int `db:"removal_type_id"`
	// UUID uniquely identifies a domain entity.
	EntityUUID string `db:"entity_uuid"`
	// Force indicates whether this removal was qualified with the --force flag.
	Force bool `db:"force"`
	// ScheduledFor indicates the earliest date that this job should be executed.
	ScheduledFor time.Time `db:"scheduled_for"`
	// Arg is a JSON representing free-form job argumentation.
	Arg string `db:"arg"`
}

// entityUUID holds a UUID in string form.
type entityUUID struct {
	// UUID uniquely identifies a domain entity.
	UUID string `db:"uuid"`
}
