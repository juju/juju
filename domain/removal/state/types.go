// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"database/sql"
	"time"

	"github.com/juju/juju/domain/life"
)

// removalJob represents a record in the removal table
type removalJob struct {
	// UUID uniquely identifies this removal job.
	UUID string `db:"uuid"`
	// RemovalTypeID indicates the type of entity that this removal job is for.
	RemovalTypeID uint64 `db:"removal_type_id"`
	// UUID uniquely identifies the domain entity being removed.
	EntityUUID string `db:"entity_uuid"`
	// Force indicates whether this removal was qualified with the --force flag.
	Force bool `db:"force"`
	// ScheduledFor indicates the earliest date that this job should be executed.
	ScheduledFor time.Time `db:"scheduled_for"`
	// Arg is a JSON string representing free-form job argumentation.
	// It must represent a map[string]any.
	Arg sql.NullString `db:"arg"`
}

// entityUUID holds a UUID in string form.
type entityUUID struct {
	// UUID uniquely identifies a domain entity.
	UUID string `db:"uuid"`
}

// entityAssoicationCount holds a Count in int form and the UUID in string form
// for the associated entity.
type entityAssoicationCount struct {
	// UUID uniquely identifies a associated domain entity.
	UUID string `db:"uuid"`
	// Count counts the number of entities.
	Count int `db:"count"`
}

// entityLife holds an entity's life in integer
type entityLife struct {
	Life life.Life `db:"life_id"`
}
