// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

// TODO(ericsnow) Eliminate the juju-related imports.

import (
	"time"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/state/multiwatcher"
)

// StatusParams holds parameters for the Status call.
type StatusParams struct {
	Patterns []string `json:"patterns"`
}

// TODO(ericsnow) Add FullStatusResult.

// FullStatus holds information about the status of a juju model.
type FullStatus struct {
	Model        ModelStatusInfo              `json:"model"`
	Machines     map[string]MachineStatus     `json:"machines"`
	Applications map[string]ApplicationStatus `json:"applications"`
	Relations    []RelationStatus             `json:"relations"`
}

// ModelStatusInfo holds status information about the model itself.
type ModelStatusInfo struct {
	Name             string `json:"name"`
	CloudTag         string `json:"cloud-tag"`
	CloudRegion      string `json:"region,omitempty"`
	Version          string `json:"version"`
	AvailableVersion string `json:"available-version"`
	Migration        string `json:"migration,omitempty"`
}

// MachineStatus holds status info about a machine.
type MachineStatus struct {
	AgentStatus    DetailedStatus `json:"agent-status"`
	InstanceStatus DetailedStatus `json:"instance-status"`

	DNSName    string                    `json:"dns-name"`
	InstanceId instance.Id               `json:"instance-id"`
	Series     string                    `json:"series"`
	Id         string                    `json:"id"`
	Containers map[string]MachineStatus  `json:"containers"`
	Hardware   string                    `json:"hardware"`
	Jobs       []multiwatcher.MachineJob `json:"jobs"`
	HasVote    bool                      `json:"has-vote"`
	WantsVote  bool                      `json:"wants-vote"`
}

// ApplicationStatus holds status info about an application.
type ApplicationStatus struct {
	Err             error                  `json:"err,omitempty"`
	Charm           string                 `json:"charm"`
	Series          string                 `json:"series"`
	Exposed         bool                   `json:"exposed"`
	Life            string                 `json:"life"`
	Relations       map[string][]string    `json:"relations"`
	CanUpgradeTo    string                 `json:"can-upgrade-to"`
	SubordinateTo   []string               `json:"subordinate-to"`
	Units           map[string]UnitStatus  `json:"units"`
	MeterStatuses   map[string]MeterStatus `json:"meter-statuses"`
	Status          DetailedStatus         `json:"status"`
	WorkloadVersion string                 `json:"workload-version"`
}

// MeterStatus represents the meter status of a unit.
type MeterStatus struct {
	Color   string `json:"color"`
	Message string `json:"message"`
}

// UnitStatus holds status info about a unit.
type UnitStatus struct {
	// AgentStatus holds the status for a unit's agent.
	AgentStatus DetailedStatus `json:"agent-status"`

	// WorkloadStatus holds the status for a unit's workload
	WorkloadStatus  DetailedStatus `json:"workload-status"`
	WorkloadVersion string         `json:"workload-version"`

	Machine       string                `json:"machine"`
	OpenedPorts   []string              `json:"opened-ports"`
	PublicAddress string                `json:"public-address"`
	Charm         string                `json:"charm"`
	Subordinates  map[string]UnitStatus `json:"subordinates"`
}

// RelationStatus holds status info about a relation.
type RelationStatus struct {
	Id        int              `json:"id"`
	Key       string           `json:"key"`
	Interface string           `json:"interface"`
	Scope     string           `json:"scope"`
	Endpoints []EndpointStatus `json:"endpoints"`
}

// EndpointStatus holds status info about a single endpoint
type EndpointStatus struct {
	ApplicationName string `json:"application"`
	Name            string `json:"name"`
	Role            string `json:"role"`
	Subordinate     bool   `json:"subordinate"`
}

// TODO(ericsnow) Eliminate the String method.

func (epStatus *EndpointStatus) String() string {
	return epStatus.ApplicationName + ":" + epStatus.Name
}

// DetailedStatus holds status info about a machine or unit agent.
type DetailedStatus struct {
	Status  string                 `json:"status"`
	Info    string                 `json:"info"`
	Data    map[string]interface{} `json:"data"`
	Since   *time.Time             `json:"since"`
	Kind    string                 `json:"kind"`
	Version string                 `json:"version"`
	Life    string                 `json:"life"`
	Err     error                  `json:"err,omitempty"`
}

// History holds many DetailedStatus,
type History struct {
	Statuses []DetailedStatus `json:"statuses"`
	Error    *Error           `json:"error,omitempty"`
}

// StatusHistoryFilter holds arguments that can be use to filter a status history backlog.
type StatusHistoryFilter struct {
	Size  int            `json:"size"`
	Date  *time.Time     `json:"date"`
	Delta *time.Duration `json:"delta"`
}

// StatusHistoryRequest holds the parameters to filter a status history query.
type StatusHistoryRequest struct {
	Kind   string              `json:"historyKind"`
	Size   int                 `json:"size"`
	Filter StatusHistoryFilter `json:"filter"`
	Tag    string              `json:"tag"`
}

// StatusHistoryRequests holds a slice of StatusHistoryArgs
type StatusHistoryRequests struct {
	Requests []StatusHistoryRequest `json:"requests"`
}

// StatusHistoryResult holds a slice of statuses.
type StatusHistoryResult struct {
	History History `json:"history"`
	Error   *Error  `json:"error,omitempty"`
}

// StatusHistoryResults holds a slice of StatusHistoryResult.
type StatusHistoryResults struct {
	Results []StatusHistoryResult `json:"results"`
}

// StatusHistoryPruneArgs holds arguments for status history
// prunning process.
type StatusHistoryPruneArgs struct {
	MaxHistoryTime time.Duration `json:"max-history-time"`
	MaxHistoryMB   int           `json:"max-history-mb"`
}

// StatusResult holds an entity status, extra information, or an
// error.
type StatusResult struct {
	Error  *Error                 `json:"error,omitempty"`
	Id     string                 `json:"id"`
	Life   Life                   `json:"life"`
	Status string                 `json:"status"`
	Info   string                 `json:"info"`
	Data   map[string]interface{} `json:"data"`
	Since  *time.Time             `json:"since"`
}

// StatusResults holds multiple status results.
type StatusResults struct {
	Results []StatusResult `json:"results"`
}

// ApplicationStatusResult holds results for an application Full Status
type ApplicationStatusResult struct {
	Application StatusResult            `json:"application"`
	Units       map[string]StatusResult `json:"units"`
	Error       *Error                  `json:"error,omitempty"`
}

// ApplicationStatusResults holds multiple StatusResult.
type ApplicationStatusResults struct {
	Results []ApplicationStatusResult `json:"results"`
}

// Life describes the lifecycle state of an entity ("alive", "dying" or "dead").
type Life multiwatcher.Life

const (
	Alive Life = "alive"
	Dying Life = "dying"
	Dead  Life = "dead"
)
