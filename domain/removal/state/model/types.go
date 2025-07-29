// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

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

// objectStoreUUID holds the UUID of an object store item.
type objectStoreUUID struct {
	UUID sql.Null[string] `db:"uuid"`
}

// entityUUIDs is a slice of entityUUID, used to hold multiple UUIDs.
type uuids []string

// entityUUID holds a UUID in string form.
type entityUUID struct {
	// UUID uniquely identifies a domain entity.
	UUID string `db:"uuid"`
}

// entityAssociationCount holds a Count in int form and the UUID in string form
// for the associated entity.
type entityAssociationCount struct {
	// UUID uniquely identifies a associated domain entity.
	UUID string `db:"uuid"`
	// Count counts the number of entities.
	Count int `db:"count"`
}

type count struct {
	Count int `db:"count"`
}

// unitMachineLifeSummary holds the counts of alive, not alive, and machine parent
// entities associated with a unit identified by the UUID. It is used to
// summarize the state of a unit in terms of its associated entities.
type unitMachineLifeSummary struct {
	// UUID uniquely identifies a associated domain entity.
	UUID string `db:"uuid"`
	// AliveCount counts the number of entities alive.
	AliveCount int `db:"alive_count"`
	// NotAliveCount counts the number of entities not alive.
	NotAliveCount int `db:"not_alive_count"`
	// MachineParentCount counts the number of entities associated with the
	// entity identified by the UUID.
	MachineParentCount int `db:"machine_parent_count"`
}

// entityLife holds an entity's life in integer
type entityLife struct {
	Life life.Life `db:"life_id"`
}

// unitUUID holds a unit UUID in string form.
type unitUUID struct {
	// UUID uniquely identifies a unit.
	UUID string `db:"unit_uuid"`
}

// applicationUnitName holds the application and unit names.
type applicationUnitName struct {
	// ApplicationName is the name of the application.
	ApplicationName string `db:"application_name"`
	// UnitName is the name of the unit.
	UnitName string `db:"unit_name"`
}

type linkLayerDevice struct {
	HardwareAddress string `db:"hardware_address"`
	Count           int    `db:"count"`
	UUID            string `db:"uuid"`
}
