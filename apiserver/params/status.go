// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

// TODO(ericsnow) Eliminate the juju-related imports.

import (
	"time"

	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state/multiwatcher"
)

// StatusParams holds parameters for the Status call.
type StatusParams struct {
	Patterns []string
}

// TODO(ericsnow) Add FullStatusResult.

// Status holds information about the status of a juju environment.
type FullStatus struct {
	EnvironmentName string
	Machines        map[string]MachineStatus
	Services        map[string]ServiceStatus
	Networks        map[string]NetworkStatus
	Relations       []RelationStatus
}

// MachineStatus holds status info about a machine.
type MachineStatus struct {
	Agent AgentStatus

	// The following fields mirror fields in AgentStatus (introduced
	// in 1.19.x). The old fields below are being kept for
	// compatibility with old clients.
	// They can be removed once API versioning lands.
	AgentState     Status
	AgentStateInfo string
	AgentVersion   string
	Life           string
	Err            error

	DNSName       string
	InstanceId    instance.Id
	InstanceState string
	Series        string
	Id            string
	Containers    map[string]MachineStatus
	Hardware      string
	Jobs          []multiwatcher.MachineJob
	HasVote       bool
	WantsVote     bool
}

// ServiceStatus holds status info about a service.
type ServiceStatus struct {
	Err           error
	Charm         string
	Exposed       bool
	Life          string
	Relations     map[string][]string
	Networks      NetworksSpecification
	CanUpgradeTo  string
	SubordinateTo []string
	Units         map[string]UnitStatus
	MeterStatuses map[string]MeterStatus
	Status        AgentStatus
}

// MeterStatus represents the meter status of a unit.
type MeterStatus struct {
	Color   string
	Message string
}

// UnitStatus holds status info about a unit.
type UnitStatus struct {
	// UnitAgent holds the status for a unit's agent.
	UnitAgent AgentStatus

	// Workload holds the status for a unit's workload
	Workload AgentStatus

	// Until Juju 2.0, we need to continue to return legacy agent state values
	// as top level struct attributes when the "FullStatus" API is called.
	AgentState     Status
	AgentStateInfo string
	AgentVersion   string
	Life           string
	Err            error

	Machine       string
	OpenedPorts   []string
	PublicAddress string
	Charm         string
	Subordinates  map[string]UnitStatus
}

// TODO(ericsnow) Rename to ServiceNetworksSepcification.

// NetworksSpecification holds the enabled and disabled networks for a
// service.
type NetworksSpecification struct {
	Enabled  []string
	Disabled []string
}

// NetworkStatus holds status info about a network.
type NetworkStatus struct {
	Err        error
	ProviderId network.Id
	CIDR       string
	VLANTag    int
}

// RelationStatus holds status info about a relation.
type RelationStatus struct {
	Id        int
	Key       string
	Interface string
	Scope     charm.RelationScope
	Endpoints []EndpointStatus
}

// EndpointStatus holds status info about a single endpoint
type EndpointStatus struct {
	ServiceName string
	Name        string
	Role        charm.RelationRole
	Subordinate bool
}

// TODO(ericsnow) Eliminate the String method.

func (epStatus *EndpointStatus) String() string {
	return epStatus.ServiceName + ":" + epStatus.Name
}

// AgentStatus holds status info about a machine or unit agent.
type AgentStatus struct {
	Status  Status
	Info    string
	Data    map[string]interface{}
	Since   *time.Time
	Kind    HistoryKind
	Version string
	Life    string
	Err     error
}

// LegacyStatus holds minimal information on the status of a juju environment.
type LegacyStatus struct {
	Machines map[string]LegacyMachineStatus
}

// LegacyMachineStatus holds just the instance-id of a machine.
type LegacyMachineStatus struct {
	InstanceId string // Not type instance.Id just to match original api.
}

// TODO(ericsnow) Rename to StatusHistoryArgs.

// StatusHistory holds the parameters to filter a status history query.
type StatusHistory struct {
	Kind HistoryKind
	Size int
	Name string
}

// TODO(ericsnow) Rename to UnitStatusHistoryResult.

// UnitStatusHistory holds a slice of statuses.
type UnitStatusHistory struct {
	Statuses []AgentStatus
}

// StatusResult holds an entity status, extra information, or an
// error.
type StatusResult struct {
	Error  *Error
	Id     string
	Life   Life
	Status Status
	Info   string
	Data   map[string]interface{}
	Since  *time.Time
}

// StatusResults holds multiple status results.
type StatusResults struct {
	Results []StatusResult
}

// ServiceStatusResult holds results for a service Full Status
type ServiceStatusResult struct {
	Service StatusResult
	Units   map[string]StatusResult
	Error   *Error
}

// ServiceStatusResults holds multiple StatusResult.
type ServiceStatusResults struct {
	Results []ServiceStatusResult
}

type HistoryKind string

const (
	KindCombined HistoryKind = "combined"
	KindAgent    HistoryKind = "agent"
	KindWorkload HistoryKind = "workload"
)

// Life describes the lifecycle state of an entity ("alive", "dying" or "dead").
type Life multiwatcher.Life

const (
	Alive Life = "alive"
	Dying Life = "dying"
	Dead  Life = "dead"
)

// Status represents the status of an entity.
// It could be a unit, machine or its agent.
type Status multiwatcher.Status

const (
	// Status values common to machine and unit agents.

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

const (
	// Status values specific to machine agents.

	// The machine is not yet participating in the environment.
	StatusPending Status = "pending"

	// The machine's agent will perform no further action, other than
	// to set the unit to Dead at a suitable moment.
	StatusStopped Status = "stopped"

	// The machine ought to be signalling activity, but it cannot be
	// detected.
	StatusDown Status = "down"
)

const (
	// Status values specific to unit agents.

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

const (
	// Status values specific to services and units, reflecting the
	// state of the software itself.

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

	// The unit needs manual intervention to get back to the Running state.
	StatusBlocked Status = "blocked"

	// The unit believes it is correctly offering all the services it has
	// been asked to offer.
	StatusActive Status = "active"
)

const (
	// Status values specific to storage.

	// StatusAttaching indicates that the storage is being attached
	// to a machine.
	StatusAttaching Status = "attaching"

	// StatusAttached indicates that the storage is attached to a
	// machine.
	StatusAttached Status = "attached"

	// StatusDetaching indicates that the storage is being detached
	// from a machine.
	StatusDetaching Status = "detaching"

	// StatusDetached indicates that the storage is not attached to
	// any machine.
	StatusDetached Status = "detached"

	// StatusDestroying indicates that the storage is being destroyed.
	StatusDestroying Status = "destroying"
)
