// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

// TODO(ericsnow) Eliminate the juju-related imports.

import (
	"time"

	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state/multiwatcher"
)

// StatusParams holds parameters for the Status call.
type StatusParams struct {
	Patterns []string
}

// TODO(ericsnow) Add FullStatusResult.

// FullStatus holds information about the status of a juju model.
type FullStatus struct {
	ModelName        string
	AvailableVersion string
	Machines         map[string]MachineStatus
	Services         map[string]ServiceStatus
	Networks         map[string]NetworkStatus
	Relations        []RelationStatus
}

// MachineStatus holds status info about a machine.
type MachineStatus struct {
	Agent AgentStatus

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
// TODO(dimitern): Drop this in a follow-up.
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

// LegacyStatus holds minimal information on the status of a juju model.
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

const (
	// DefaultMaxLogsPerEntity is the default value for logs for each entity
	// that should be kept at any given time.
	DefaultMaxLogsPerEntity = 100

	// DefaultPruneInterval is the default interval that should be waited
	// between prune calls.
	DefaultPruneInterval = 5 * time.Minute
)

// StatusHistoryPruneArgs holds arguments for status history
// prunning process.
type StatusHistoryPruneArgs struct {
	MaxLogsPerEntity int
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

// HistoryKind represents the possible types of
// status history entries.
type HistoryKind string

const (
	// KindCombined represents all possible kinds.
	KindCombined HistoryKind = "combined"
	// KindAgent represent a unit agent status history entry.
	KindAgent HistoryKind = "agent"
	// KindWorkload represents a charm workload status history entry.
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

	// StatusError means the entity requires human intervention
	// in order to operate correctly.
	StatusError Status = "error"

	// StatusStarted is set when:
	// The entity is actively participating in the model.
	// For unit agents, this is a state we preserve for backwards
	// compatibility with scripts during the life of Juju 1.x.
	// In Juju 2.x, the agent-state will remain “active” and scripts
	// will watch the unit-state instead for signals of service readiness.
	StatusStarted Status = "started"
)

const (
	// Status values specific to machine agents.

	// StatusPending is set when:
	// The machine is not yet participating in the model.
	StatusPending Status = "pending"

	// StatusStopped is set when:
	// The machine's agent will perform no further action, other than
	// to set the unit to Dead at a suitable moment.
	StatusStopped Status = "stopped"

	// StatusDown is set when:
	// The machine ought to be signalling activity, but it cannot be
	// detected.
	StatusDown Status = "down"
)

const (
	// Status values specific to unit agents.

	// StatusAllocating is set when:
	// The machine on which a unit is to be hosted is still being
	// spun up in the cloud.
	StatusAllocating Status = "allocating"

	// StatusRebooting is set when:
	// The machine on which this agent is running is being rebooted.
	// The juju-agent should move from rebooting to idle when the reboot is complete.
	StatusRebooting Status = "rebooting"

	// StatusExecuting is set when:
	// The agent is running a hook or action. The human-readable message should reflect
	// which hook or action is being run.
	StatusExecuting Status = "executing"

	// StatusIdle is set when:
	// Once the agent is installed and running it will notify the Juju server and its state
	// becomes "idle". It will stay "idle" until some action (e.g. it needs to run a hook) or
	// error (e.g it loses contact with the Juju server) moves it to a different state.
	StatusIdle Status = "idle"

	// StatusFailed is set when:
	// The unit agent has failed in some way,eg the agent ought to be signalling
	// activity, but it cannot be detected. It might also be that the unit agent
	// detected an unrecoverable condition and managed to tell the Juju server about it.
	StatusFailed Status = "failed"

	// StatusLost is set when:
	// The juju agent has has not communicated with the juju server for an unexpectedly long time;
	// the unit agent ought to be signalling activity, but none has been detected.
	StatusLost Status = "lost"
)

const (
	// Status values specific to services and units, reflecting the
	// state of the software itself.

	// StatusMaintenance is set when:
	// The unit is not yet providing services, but is actively doing stuff
	// in preparation for providing those services.
	// This is a "spinning" state, not an error state.
	// It reflects activity on the unit itself, not on peers or related units.
	StatusMaintenance Status = "maintenance"

	// StatusTerminated is set when:
	// This unit used to exist, we have a record of it (perhaps because of storage
	// allocated for it that was flagged to survive it). Nonetheless, it is now gone.
	StatusTerminated Status = "terminated"

	// StatusUnknown is set when:
	// A unit-agent has finished calling install, config-changed, and start,
	// but the charm has not called status-set yet.
	StatusUnknown Status = "unknown"

	// StatusWaiting is set when:
	// The unit is unable to progress to an active state because a service to
	// which it is related is not running.
	StatusWaiting Status = "waiting"

	// StatusBlocked is set when:
	// The unit needs manual intervention to get back to the Running state.
	StatusBlocked Status = "blocked"

	// StatusActive is set when:
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
