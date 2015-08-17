// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"time"
)

// Status used to represent the status of an entity, but has recently become
// and applies to "workloads" as well, which we don't currently model, for no
// very clear reason.
//
// Status values currently apply to machine (agents), unit (agents), unit
// (workloads), service (workloads), and volumes.
type Status string

// StatusInfo holds a Status and associated information.
type StatusInfo struct {
	Status  Status
	Message string
	Data    map[string]interface{}
	Since   *time.Time
}

// StatusSetter represents a type whose status can be set.
type StatusSetter interface {
	SetStatus(status Status, info string, data map[string]interface{}) error
}

// StatusGetter represents a type whose status can be read.
type StatusGetter interface {
	Status() (StatusInfo, error)
}

// Status values common to machine and unit agents.
const (

	// The entity requires human intervention in order to operate
	// correctly.
	StatusError Status = "error"

	// The entity is actively participating in the environment.
	// For unit agents, this is a state we preserve for backwards
	// compatibility with scripts during the life of Juju 1.x.
	// In Juju 2.x, the agent-state will remain “active” and scripts
	// will watch the unit-state instead for signals of service readiness.
	StatusStarted Status = "started"
)

// Status values specific to machine agents.
const (

	// The machine is not yet participating in the environment.
	StatusPending Status = "pending"

	// The machine's agent will perform no further action, other than
	// to set the unit to Dead at a suitable moment.
	StatusStopped Status = "stopped"

	// The machine ought to be signalling activity, but it cannot be
	// detected.
	StatusDown Status = "down"
)

// Status values specific to unit agents.
const (

	// The machine on which a unit is to be hosted is still being
	// spun up in the cloud.
	StatusAllocating Status = "allocating"

	// The machine on which this agent is running is being rebooted.
	// The juju-agent should move from rebooting to idle when the reboot is complete.
	StatusRebooting Status = "rebooting"

	// The agent is running a hook or action. The human-readable message should reflect
	// which hook or action is being run.
	StatusExecuting Status = "executing"

	// Once the agent is installed and running it will notify the Juju server and its state
	// becomes "idle". It will stay "idle" until some action (e.g. it needs to run a hook) or
	// error (e.g it loses contact with the Juju server) moves it to a different state.
	StatusIdle Status = "idle"

	// The unit agent has failed in some way,eg the agent ought to be signalling
	// activity, but it cannot be detected. It might also be that the unit agent
	// detected an unrecoverable condition and managed to tell the Juju server about it.
	StatusFailed Status = "failed"

	// The juju agent has has not communicated with the juju server for an unexpectedly long time;
	// the unit agent ought to be signalling activity, but none has been detected.
	StatusLost Status = "lost"

	// ---- Outdated ----
	// The unit agent is downloading the charm and running the install hook.
	StatusInstalling Status = "installing"

	// The unit is being destroyed; the agent will soon mark the unit as “dead”.
	// In Juju 2.x this will describe the state of the agent rather than a unit.
	StatusStopping Status = "stopping"
)

// Status values specific to services and units, reflecting the
// state of the software itself.
const (

	// The unit is not yet providing services, but is actively doing stuff
	// in preparation for providing those services.
	// This is a "spinning" state, not an error state.
	// It reflects activity on the unit itself, not on peers or related units.
	StatusMaintenance Status = "maintenance"

	// This unit used to exist, we have a record of it (perhaps because of storage
	// allocated for it that was flagged to survive it). Nonetheless, it is now gone.
	StatusTerminated Status = "terminated"

	// A unit-agent has finished calling install, config-changed, and start,
	// but the charm has not called status-set yet.
	StatusUnknown Status = "unknown"

	// The unit is unable to progress to an active state because a service to
	// which it is related is not running.
	StatusWaiting Status = "waiting"

	// The unit needs manual intervention to get back to the Active state.
	StatusBlocked Status = "blocked"

	// The unit believes it is correctly offering all the services it has
	// been asked to offer.
	StatusActive Status = "active"
)

// Status values specific to storage.
const (
	// StatusAttaching indicates that the storage is being attached to a
	// machine.
	StatusAttaching Status = "attaching"

	// StatusDetaching indicates that the storage is attached to a machine.
	StatusAttached Status = "attached"

	// StatusDetaching indicates that the storage is being detached
	// from a machine.
	StatusDetaching Status = "detaching"

	// StatusDetached indicates that the storage is not attached to any
	// machine.
	StatusDetached Status = "detached"

	// StatusDestroying indicates that the storage is being destroyed.
	StatusDestroying Status = "destroying"
)

const (
	MessageInstalling = "installing charm software"

	// StorageReadyMessage is the message set to the agent status when all storage
	// attachments are properly done.
	StorageReadyMessage = "storage ready"

	// PreparingStorageMessage is the message set to the agent status before trying
	// to attach storages.
	PreparingStorageMessage = "preparing storage"
)

// KnownAgentStatus returns true if status has a known value for an agent.
// It includes every status that has ever been valid for a unit or machine agent.
// This is used by the apiserver client facade to filter out unknown values.
func (status Status) KnownAgentStatus() bool {
	switch status {
	case
		StatusAllocating,
		StatusError,
		StatusFailed,
		StatusRebooting,
		StatusExecuting,
		StatusIdle:
		return true
	case //Deprecated status vales
		StatusPending,
		StatusStarted,
		StatusStopped,
		StatusInstalling,
		StatusActive,
		StatusStopping,
		StatusDown:
		return true
	default:
		return false
	}
}

// KnownWorkloadStatus returns true if status has a known value for a workload.
// It includes every status that has ever been valid for a unit agent.
// This is used by the apiserver client facade to filter out unknown values.
func (status Status) KnownWorkloadStatus() bool {
	if ValidWorkloadStatus(status) {
		return true
	}
	switch status {
	case StatusError: // include error so that we can filter on what the spec says is valid
		return true
	case // Deprecated statuses
		StatusPending,
		StatusInstalling,
		StatusStarted,
		StatusStopped,
		StatusDown:
		return true
	default:
		return false
	}
}

// ValidWorkloadStatus returns true if status has a valid value (that is to say,
// a value that it's OK to set) for units or services.
func ValidWorkloadStatus(status Status) bool {
	switch status {
	case
		StatusBlocked,
		StatusMaintenance,
		StatusWaiting,
		StatusActive,
		StatusUnknown,
		StatusTerminated:
		return true
	default:
		return false
	}
}

// WorkloadMatches returns true if the candidate matches status,
// taking into account that the candidate may be a legacy
// status value which has been deprecated.
func (status Status) WorkloadMatches(candidate Status) bool {
	switch candidate {
	case status: // We could be holding an old status ourselves
		return true
	case StatusDown, StatusStopped:
		candidate = StatusTerminated
	case StatusInstalling:
		candidate = StatusMaintenance
	case StatusStarted:
		candidate = StatusActive
	}
	return status == candidate
}

// Matches returns true if the candidate matches status,
// taking into account that the candidate may be a legacy
// status value which has been deprecated.
func (status Status) Matches(candidate Status) bool {
	switch candidate {
	case StatusDown:
		candidate = StatusLost
	case StatusStarted:
		candidate = StatusActive
	case StatusStopped:
		candidate = StatusStopping
	}
	return status == candidate
}

// TranslateLegacyAgentStatus returns the status value clients expect to see for
// agent-state in versions prior to 1.24
func TranslateToLegacyAgentState(agentStatus, workloadStatus Status, workloadMessage string) (Status, bool) {
	// Originally AgentState (a member of api.UnitStatus) could hold one of:
	// StatusPending
	// StatusInstalled
	// StatusStarted
	// StatusStopped
	// StatusError
	// StatusDown
	// For compatibility reasons we convert modern states (from V2 uniter) into
	// four of the old ones: StatusPending, StatusStarted, StatusStopped, or StatusError.

	// For the purposes of deriving the legacy status, there's currently no better
	// way to determine if a unit is installed.
	// TODO(wallyworld) - use status history to see if start hook has run.
	isInstalled := workloadStatus != StatusMaintenance || workloadMessage != MessageInstalling

	switch agentStatus {
	case StatusAllocating:
		return StatusPending, true
	case StatusError:
		return StatusError, true
	case StatusRebooting, StatusExecuting, StatusIdle, StatusLost, StatusFailed:
		switch workloadStatus {
		case StatusError:
			return StatusError, true
		case StatusTerminated:
			return StatusStopped, true
		case StatusMaintenance:
			if isInstalled {
				return StatusStarted, true
			} else {
				return StatusPending, true
			}
		default:
			return StatusStarted, true
		}
	}
	return "", false
}
