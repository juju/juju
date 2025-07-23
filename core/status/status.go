// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"time"
)

// Status used to represent the status of an entity, but has recently become
// and applies to "workloads" as well, which we don't currently model, for no
// very clear reason.
//
// Status values currently apply to machine (agents), unit (agents), unit
// (workloads), application (workloads), volumes, filesystems, and models.
type Status string

// String returns a string representation of the Status.
func (s Status) String() string {
	return string(s)
}

// StatusInfo holds a Status and associated information.
type StatusInfo struct {
	Status  Status
	Message string
	Data    map[string]interface{}
	Since   *time.Time
}

// StatusSetter represents a type whose status can be set.
type StatusSetter interface {
	SetStatus(StatusInfo) error
}

// StatusGetter represents a type whose status can be read.
type StatusGetter interface {
	Status() (StatusInfo, error)
}

// ModificationStatusGetter represents a type whose modification status can be
// read.
type ModificationStatusGetter interface {
	ModificationStatus() (StatusInfo, error)
}

const (
	// Status values common to machine and unit agents, and actions.

	// Error means the entity requires human intervention
	// in order to operate correctly.
	//
	// The action did not get run due to an error.
	Error Status = "error"
)

const (
	// Status values common to machine and unit agents.

	// Started is set when:
	// The entity is actively participating in the model.
	// For unit agents, this is a state we preserve for backwards
	// compatibility with scripts during the life of Juju 1.x.
	// In Juju 2.x, the agent-state will remain “active” and scripts
	// will watch the unit-state instead for signals of application readiness.
	Started Status = "started"
)

const (
	// Status values specific to machine agents and actions.

	// Pending is set when:
	//
	// The machine is not yet participating in the model.
	//
	// The action first is queued.
	Pending Status = "pending"
)

const (
	// Status values specific to machine agents.

	// Stopped is set when:
	// The machine's agent will perform no further action, other than
	// to set the unit to Dead at a suitable moment.
	Stopped Status = "stopped"

	// Down is set when:
	// The machine ought to be signalling activity, but it cannot be
	// detected.
	Down Status = "down"
)

const (
	// Status values specific to unit agents and actions.

	// Failed is set when:
	//
	// The unit agent has failed in some way,eg the agent ought to be signalling
	// activity, but it cannot be detected. It might also be that the unit agent
	// detected an unrecoverable condition and managed to tell the Juju server about it.
	//
	// The action did not complete successfully.
	Failed Status = "failed"
)

const (
	// Status values specific to unit agents.

	// Allocating is set when:
	// The machine on which a unit is to be hosted is still being
	// spun up in the cloud.
	Allocating Status = "allocating"

	// Rebooting is set when:
	// The machine on which this agent is running is being rebooted.
	// The juju-agent should move from rebooting to idle when the reboot is complete.
	Rebooting Status = "rebooting"

	// Executing is set when:
	// The agent is running a hook or action. The human-readable message should reflect
	// which hook or action is being run.
	Executing Status = "executing"

	// Idle is set when:
	// Once the agent is installed and running it will notify the Juju server and its state
	// becomes "idle". It will stay "idle" until some action (e.g. it needs to run a hook) or
	// error (e.g it loses contact with the Juju server) moves it to a different state.
	Idle Status = "idle"

	// Lost is set when:
	// The juju agent has not communicated with the juju server for an unexpectedly long time;
	// the unit agent ought to be signalling activity, but none has been detected.
	Lost Status = "lost"
)

const (
	// Status values specific to applications and units, reflecting the
	// state of the software itself.

	// Unset is only for applications, and is a placeholder status.
	// The core/cache package deals with aggregating the unit status
	// to the application level.
	Unset Status = "unset"

	// Maintenance is set when:
	// The unit is not yet providing services, but is actively doing stuff
	// in preparation for providing those services.
	// This is a "spinning" state, not an error state.
	// It reflects activity on the unit itself, not on peers or related units.
	Maintenance Status = "maintenance"

	// Terminated is set when:
	// This unit used to exist, we have a record of it (perhaps because of storage
	// allocated for it that was flagged to survive it). Nonetheless, it is now gone.
	Terminated Status = "terminated"

	// Unknown is set when:
	// A unit-agent has finished calling install, config-changed, and start,
	// but the charm has not called status-set yet.
	Unknown Status = "unknown"

	// Waiting is set when:
	// The unit is unable to progress to an active state because an application to
	// which it is related is not running.
	Waiting Status = "waiting"

	// Blocked is set when:
	// The unit needs manual intervention to get back to the Running state.
	Blocked Status = "blocked"

	// Active is set when:
	// The unit believes it is correctly offering all the services it has
	// been asked to offer.
	Active Status = "active"
)

const (
	// Status values specific to storage.

	// Attaching indicates that the storage is being attached
	// to a machine.
	Attaching Status = "attaching"

	// Attached indicates that the storage is attached to a
	// machine.
	Attached Status = "attached"

	// Detaching indicates that the storage is being detached
	// from a machine.
	Detaching Status = "detaching"

	// Detached indicates that the storage is not attached to
	// any machine.
	Detached Status = "detached"
)

const (
	// Status values specific to models.

	// Available indicates that the model is available for use.
	Available Status = "available"

	// Busy indicates that the model is not available for use because it is
	// running a process that must take the model offline, such as a migration,
	// upgrade, or backup.  This is a spinning state, it is not an error state,
	// and it should be expected that the model will eventually go back to
	// available.
	Busy Status = "busy"
)

const (
	// Status values specific to relations.

	// Joining is used to signify that a relation should become joined soon.
	Joining Status = "joining"

	// Joined is the normal status for a healthy, alive relation.
	Joined Status = "joined"

	// Broken is the status for when a relation life goes to Dead.
	Broken Status = "broken"

	// Suspending is used to signify that a relation will be temporarily broken
	// pending action to resume it.
	Suspending Status = "suspending"

	// Suspended is used to signify that a relation is temporarily broken pending
	// action to resume it.
	Suspended Status = "suspended"
)

const (
	// Status values specific to actions.

	// Completed indicates that the action ran to completion as intended.
	Completed Status = "completed"

	// Cancelled means that the Action was cancelled before being run.
	Cancelled Status = "cancelled"

	// Aborting indicates that the Action is running but should be
	// aborted.
	Aborting Status = "aborting"

	// Aborted indicates the Action was aborted.
	Aborted Status = "aborted"
)

const (
	// Status values that are common to several entities.

	// Destroying indicates that the entity is being destroyed.
	//
	// This is valid for volumes, filesystems, and models.
	Destroying Status = "destroying"
)

const (
	// Status values that are common to instances and actions.

	// Running indicates that the entity is currently running.
	Running Status = "running"
)

// InstanceStatus
const (
	Empty             Status = ""
	Provisioning      Status = "allocating"
	ProvisioningError Status = "provisioning error"
)

// ModificationStatus
const (
	Applied Status = "applied"
)

const (
	MessageWaitForMachine    = "waiting for machine"
	MessageWaitForContainer  = "waiting for container"
	MessageInstallingAgent   = "installing agent"
	MessageInitializingAgent = "agent initialising"
	MessageInstallingCharm   = "installing charm software"
)

// KnownModificationStatus returns true if the status has a known value for
// a modification of an instance.
func (s Status) KnownModificationStatus() bool {
	switch s {
	case
		Idle,
		Applied,
		Error,
		Unknown:
		return true
	}
	return false
}

// KnownInstanceStatus returns true if status has a known value for a machine
// cloud  instance.
func (s Status) KnownInstanceStatus() bool {
	switch s {
	case
		Pending,
		ProvisioningError,
		Allocating,
		Running,
		Error,
		Unknown:
		return true
	}
	return false
}

// KnownMachineStatus returns true if status has a known value for a machine.
func (s Status) KnownMachineStatus() bool {
	switch s {
	case
		Error,
		Started,
		Pending,
		Stopped,
		Down:
		return true
	}
	return false
}

// KnownAgentStatus returns true if status has a known value for an agent.
// It includes every status that has ever been valid for a unit or machine agent.
// This is used by the apiserver client facade to filter out unknown values.
func (s Status) KnownAgentStatus() bool {
	switch s {
	case
		Allocating,
		Error,
		Failed,
		Rebooting,
		Executing,
		Idle:
		return true
	}
	return false
}

// KnownWorkloadStatus returns true if status has a known value for a workload.
// It includes every status that has ever been valid for a unit agent.
// This is used by the apiserver client facade to filter out unknown values.
func (s Status) KnownWorkloadStatus() bool {
	if ValidWorkloadStatus(s) {
		return true
	}
	switch s {
	case Error: // include error so that we can filter on what the spec says is valid
		return true
	default:
		return false
	}
}

// ValidWorkloadStatus returns true if status has a valid value (that is to say,
// a value that it's OK to set) for units or applications.
func ValidWorkloadStatus(status Status) bool {
	switch status {
	case
		Blocked,
		Maintenance,
		Waiting,
		Active,
		Unknown,
		Terminated:
		return true
	default:
		return false
	}
}

// WorkloadMatches returns true if the candidate matches status,
// taking into account that the candidate may be a legacy
// status value which has been deprecated.
func (s Status) WorkloadMatches(candidate Status) bool {
	return s == candidate
}

// ValidModelStatus returns true if status has a valid value (that is to say,
// a value that it's OK to set) for models.
func ValidModelStatus(status Status) bool {
	switch status {
	case
		Available,
		Busy,
		Destroying,
		Suspended, // For  model, this means that its cloud credential is invalid and model will not be doing any cloud calls.
		Error:
		return true
	default:
		return false
	}
}

// Matches returns true if the candidate matches status,
// taking into account that the candidate may be a legacy
// status value which has been deprecated.
func (s Status) Matches(candidate Status) bool {
	return s == candidate
}
