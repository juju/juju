// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

// TODO(ericsnow) Eliminate the juju-related imports.

import (
	"time"

	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/instance"
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
	Applications     map[string]ApplicationStatus
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

// ApplicationStatus holds status info about a application.
type ApplicationStatus struct {
	Err           error
	Charm         string
	Exposed       bool
	Life          string
	Relations     map[string][]string
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
	ApplicationName string
	Name            string
	Role            charm.RelationRole
	Subordinate     bool
}

// TODO(ericsnow) Eliminate the String method.

func (epStatus *EndpointStatus) String() string {
	return epStatus.ApplicationName + ":" + epStatus.Name
}

// DetailedStatus holds status info about a machine or unit agent.
type DetailedStatus struct {
	Status  string                 `json:"Status"`
	Info    string                 `json:"Info"`
	Data    map[string]interface{} `json:"Data"`
	Since   *time.Time             `json:"Since"`
	Kind    string                 `json:"Kind"`
	Version string                 `json:"Version"`
	Life    string                 `json:"Life"`
	Err     error                  `json:"Err"`
}

// History holds many DetailedStatus,
type History struct {
	Statuses []DetailedStatus `json:"Statuses"`
	Error    *Error           `json:"Error,omitempty"`
}

// StatusHistoryFilter holds arguments that can be use to filter a status history backlog.
type StatusHistoryFilter struct {
	Size  int            `json:"Size"`
	Date  *time.Time     `json:"Date"`
	Delta *time.Duration `json:"Delta"`
}

// StatusHistoryRequest holds the parameters to filter a status history query.
type StatusHistoryRequest struct {
	Kind   string              `json:"HistoryKind"`
	Size   int                 `json:"Size"`
	Filter StatusHistoryFilter `json:"Filter"`
	Tag    string              `json:"Tag"`
}

// StatusHistoryRequests holds a slice of StatusHistoryArgs
type StatusHistoryRequests struct {
	Requests []StatusHistoryRequest `json:"Requests"`
}

// StatusHistoryResult holds a slice of statuses.
type StatusHistoryResult struct {
	History History `json:"History"`
	Error   *Error  `json:"Error,omitempty"`
}

// StatusHistoryResults holds a slice of StatusHistoryResult.
type StatusHistoryResults struct {
	Results []StatusHistoryResult `json:"Results"`
}

// StatusHistoryPruneArgs holds arguments for status history
// prunning process.
type StatusHistoryPruneArgs struct {
	MaxHistoryTime time.Duration `json:"MaxHistoryTime"`
	MaxHistoryMB   int           `json:"MaxHistoryMB"`
}

// StatusResult holds an entity status, extra information, or an
// error.
type StatusResult struct {
	Error  *Error
	Id     string
	Life   Life
	Status string
	Info   string
	Data   map[string]interface{}
	Since  *time.Time
}

// StatusResults holds multiple status results.
type StatusResults struct {
	Results []StatusResult
}

// ApplicationStatusResult holds results for a application Full Status
type ApplicationStatusResult struct {
	Application StatusResult
	Units       map[string]StatusResult
	Error       *Error
}

// ApplicationStatusResults holds multiple StatusResult.
type ApplicationStatusResults struct {
	Results []ApplicationStatusResult
}

// Life describes the lifecycle state of an entity ("alive", "dying" or "dead").
type Life multiwatcher.Life

const (
	Alive Life = "alive"
	Dying Life = "dying"
	Dead  Life = "dead"
)
