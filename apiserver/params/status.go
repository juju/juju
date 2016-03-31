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
	"github.com/juju/juju/status"
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
	AgentStatus    DetailedStatus
	InstanceStatus DetailedStatus

	DNSName    string
	InstanceId instance.Id
	Series     string
	Id         string
	Containers map[string]MachineStatus
	Hardware   string
	Jobs       []multiwatcher.MachineJob
	HasVote    bool
	WantsVote  bool
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
	Status        DetailedStatus
}

// MeterStatus represents the meter status of a unit.
type MeterStatus struct {
	Color   string
	Message string
}

// UnitStatus holds status info about a unit.
type UnitStatus struct {
	// AgentStatus holds the status for a unit's agent.
	AgentStatus DetailedStatus

	// WorkloadStatus holds the status for a unit's workload
	WorkloadStatus DetailedStatus

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

// DetailedStatus holds status info about a machine or unit agent.
type DetailedStatus struct {
	Status  status.Status
	Info    string
	Data    map[string]interface{}
	Since   *time.Time
	Kind    HistoryKind
	Version string
	Life    string
	Err     error
}

// StatusHistoryArgs holds the parameters to filter a status history query.
type StatusHistoryArgs struct {
	Kind HistoryKind
	Size int
	Name string
}

// StatusHistoryResults holds a slice of statuses.
type StatusHistoryResults struct {
	Statuses []DetailedStatus
}

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
	Status status.Status
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
	// KindUnit represents agent and workload combined.
	KindUnit HistoryKind = "unit"
	// KindUnitAgent represent a unit agent status history entry.
	KindUnitAgent HistoryKind = "juju-unit"
	// KindWorkload represents a charm workload status history entry.
	KindWorkload HistoryKind = "workload"
	// KindMachineInstance represents an entry for a machine instance.
	KindMachineInstance = "machine"
	// KindMachine represents an entry for a machine agent.
	KindMachine = "juju-machine"
	// KindContainerInstance represents an entry for a container instance.
	KindContainerInstance = "container"
	// KindContainer represents an entry for a container agent.
	KindContainer = "juju-container"
)

// Life describes the lifecycle state of an entity ("alive", "dying" or "dead").
type Life multiwatcher.Life

const (
	Alive Life = "alive"
	Dying Life = "dying"
	Dead  Life = "dead"
)
