// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package removal

import "time"

// JobType indicates the type of entity that a removal job is for.
type JobType uint64

const (
	// RelationJob indicates a job to remove a relation.
	RelationJob JobType = iota
	// UnitJob indicates a job to remove a unit.
	UnitJob
	// ApplicationJob indicates a job to remove an application.
	ApplicationJob
)

// Job is a removal job for a single entity.
type Job struct {
	// UUID uniquely identifies this removal job.
	UUID UUID
	// RemovalType indicates the type of entity that this removal job is for.
	RemovalType JobType
	// UUID uniquely identifies the domain entity to be removed.
	EntityUUID string
	// Force indicates whether this removal was qualified with the --force flag.
	Force bool
	// ScheduledFor indicates the earliest date that this job should be executed.
	ScheduledFor time.Time
	// Arg is free form job configuration.
	Arg map[string]any
}
