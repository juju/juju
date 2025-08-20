// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package removal

import (
	"strconv"
	"time"
)

// JobType indicates the type of entity that a removal job is for.
type JobType uint64

const (
	// RelationJob indicates a job to remove a relation.
	RelationJob JobType = iota
	// UnitJob indicates a job to remove a unit.
	UnitJob
	// ApplicationJob indicates a job to remove an application.
	ApplicationJob
	// MachineJob indicates a job to remove a machine.
	MachineJob
	// ModelJob indicates a job to remove a model.
	ModelJob
)

func (t JobType) String() string {
	switch t {
	case RelationJob:
		return "relation"
	case UnitJob:
		return "unit"
	case ApplicationJob:
		return "application"
	case MachineJob:
		return "machine"
	case ModelJob:
		return "model"
	default:
		return strconv.FormatInt(int64(t), 10)
	}
}

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
	// ScheduledFor indicates the earliest date that this job should be
	// executed.
	ScheduledFor time.Time
	// Arg is free form job configuration.
	Arg map[string]any
}

// ModelArtifacts holds the artifacts associated with a model that is being
// removed.
type ModelArtifacts struct {
	// MachineUUIDs is a list of machine UUIDs that are associated with the
	// model.
	MachineUUIDs []string
	// ApplicationUUIDs is a list of application UUIDs that are associated with
	// the model.
	ApplicationUUIDs []string
	// UnitUUIDs is a list of unit UUIDs that are associated with the model.
	UnitUUIDs []string
	// RelationUUIDs is a list of relation UUIDs that are associated with the
	// model.
	RelationUUIDs []string
}

// Empty returns true if there are no artifacts associated with the model.
func (a ModelArtifacts) Empty() bool {
	return len(a.MachineUUIDs) == 0 &&
		len(a.ApplicationUUIDs) == 0 &&
		len(a.UnitUUIDs) == 0 &&
		len(a.RelationUUIDs) == 0
}

// ApplicationArtifacts holds the artifacts associated with an application that
// is being removed.
type ApplicationArtifacts struct {
	// MachineUUIDs is a list of machine UUIDs that are associated with the
	// application.
	MachineUUIDs []string
	// UnitUUIDs is a list of unit UUIDs that are associated with the application.
	UnitUUIDs []string
	// RelationUUIDs is a list of relation UUIDs that are associated with the
	// application.
	RelationUUIDs []string
}

// Empty returns true if there are no artifacts associated with the application.
func (a ApplicationArtifacts) Empty() bool {
	return len(a.MachineUUIDs) == 0 &&
		len(a.UnitUUIDs) == 0 &&
		len(a.RelationUUIDs) == 0
}
