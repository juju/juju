// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

// TODO(ericsnow) Eliminate the juju-related imports.

import (
	"time"

	"github.com/juju/juju/core/relation"
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
	Model               ModelStatusInfo                    `json:"model"`
	Machines            map[string]MachineStatus           `json:"machines"`
	Applications        map[string]ApplicationStatus       `json:"applications"`
	RemoteApplications  map[string]RemoteApplicationStatus `json:"remote-applications"`
	Offers              map[string]ApplicationOfferStatus  `json:"offers"`
	Relations           []RelationStatus                   `json:"relations"`
	ControllerTimestamp *time.Time                         `json:"controller-timestamp"`
}

// IsEmpty checks all collections on FullStatus to determine if the status is empty.
// Note that only the collections are checked here as Model information will always be populated.
func (fs *FullStatus) IsEmpty() bool {
	return len(fs.Applications) == 0 &&
		len(fs.Machines) == 0 &&
		len(fs.Offers) == 0 &&
		len(fs.RemoteApplications) == 0 &&
		len(fs.Relations) == 0
}

// ModelStatusInfo holds status information about the model itself.
type ModelStatusInfo struct {
	Name             string         `json:"name"`
	Type             string         `json:"type"`
	CloudTag         string         `json:"cloud-tag"`
	CloudRegion      string         `json:"region,omitempty"`
	Version          string         `json:"version"`
	AvailableVersion string         `json:"available-version"`
	ModelStatus      DetailedStatus `json:"model-status"`
	MeterStatus      MeterStatus    `json:"meter-status"`
	SLA              string         `json:"sla"`
}

// NetworkInterfaceStatus holds a /etc/network/interfaces-type data and the
// space name for any device with at least one associated IP address.
type NetworkInterface struct {
	// IPAddresses holds the IP addresses bound to this machine.
	IPAddresses    []string `json:"ip-addresses"`
	MACAddress     string   `json:"mac-address"`
	Gateway        string   `json:"gateway,omitempty"`
	DNSNameservers []string `json:"dns-nameservers,omitempty"`

	// Space holds the name of a space in which this devices IP addr's
	// subnet belongs.
	Space string `json:"space,omitempty"`

	// Is this interface up?
	IsUp bool `json:"is-up"`
}

// MachineStatus holds status info about a machine.
type MachineStatus struct {
	AgentStatus    DetailedStatus `json:"agent-status"`
	InstanceStatus DetailedStatus `json:"instance-status"`
	DNSName        string         `json:"dns-name"`

	// IPAddresses holds the IP addresses known for this machine. It is
	// here for backwards compatibility. It should be similar to its
	// namesakes in NetworkInterfaces, but may also include any
	// public/floating IP addresses not actually bound to the machine but
	// known to the provider.
	IPAddresses []string `json:"ip-addresses,omitempty"`

	// InstanceId holds the unique identifier for this machine, based on
	// what is supplied by the provider.
	InstanceId instance.Id `json:"instance-id"`

	// Series holds the name of the operating system release installed on
	// this machine.
	Series string `json:"series"`

	// Id is the Juju identifier for this machine in this model.
	Id string `json:"id"`

	// NetworkInterfaces holds a map of NetworkInterface for this machine.
	NetworkInterfaces map[string]NetworkInterface `json:"network-interfaces,omitempty"`

	// Containers holds the MachineStatus of any containers hosted on this
	// machine.
	Containers map[string]MachineStatus `json:"containers"`

	// Constraints holds a string of space-separated key=value pairs for
	// each constraint datum.
	Constraints string `json:"constraints"`

	// Hardware holds a string of space-separated key=value pairs for each
	// hardware specification datum.
	Hardware string `json:"hardware"`

	Jobs      []multiwatcher.MachineJob `json:"jobs"`
	HasVote   bool                      `json:"has-vote"`
	WantsVote bool                      `json:"wants-vote"`

	// LXDProfiles holds all the machines current LXD profiles that have
	// been applied to the machine
	LXDProfiles map[string]LXDProfile `json:"lxd-profiles,omitempty"`
}

// LXDProfile holds status info about a LXDProfile
type LXDProfile struct {
	Config      map[string]string            `json:"config"`
	Description string                       `json:"description"`
	Devices     map[string]map[string]string `json:"devices"`
}

// ApplicationStatus holds status info about an application.
type ApplicationStatus struct {
	Err              error                  `json:"err,omitempty"`
	Charm            string                 `json:"charm"`
	Series           string                 `json:"series"`
	Exposed          bool                   `json:"exposed"`
	Life             string                 `json:"life"`
	Relations        map[string][]string    `json:"relations"`
	CanUpgradeTo     string                 `json:"can-upgrade-to"`
	SubordinateTo    []string               `json:"subordinate-to"`
	Units            map[string]UnitStatus  `json:"units"`
	MeterStatuses    map[string]MeterStatus `json:"meter-statuses"`
	Status           DetailedStatus         `json:"status"`
	WorkloadVersion  string                 `json:"workload-version"`
	CharmVersion     string                 `json:"charm-verion"`
	EndpointBindings map[string]string      `json:"endpoint-bindings"`

	// The following are for CAAS models.
	Scale         *int   `json:"int,omitempty"`
	ProviderId    string `json:"provider-id,omitempty"`
	PublicAddress string `json:"public-address"`
}

// RemoteApplicationStatus holds status info about a remote application.
type RemoteApplicationStatus struct {
	Err       error               `json:"err,omitempty"`
	OfferURL  string              `json:"offer-url"`
	OfferName string              `json:"offer-name"`
	Endpoints []RemoteEndpoint    `json:"endpoints"`
	Life      string              `json:"life"`
	Relations map[string][]string `json:"relations"`
	Status    DetailedStatus      `json:"status"`
}

// ApplicationOfferStatus holds status info about an application offer.
type ApplicationOfferStatus struct {
	Err                  error                     `json:"err,omitempty"`
	OfferName            string                    `json:"offer-name"`
	ApplicationName      string                    `json:"application-name"`
	CharmURL             string                    `json:"charm"`
	Endpoints            map[string]RemoteEndpoint `json:"endpoints"`
	ActiveConnectedCount int                       `json:"active-connected-count"`
	TotalConnectedCount  int                       `json:"total-connected-count"`
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

	// WorkloadStatus holds the status for a unit's workload.
	WorkloadStatus  DetailedStatus `json:"workload-status"`
	WorkloadVersion string         `json:"workload-version"`

	Machine       string                `json:"machine"`
	OpenedPorts   []string              `json:"opened-ports"`
	PublicAddress string                `json:"public-address"`
	Charm         string                `json:"charm"`
	Subordinates  map[string]UnitStatus `json:"subordinates"`
	Leader        bool                  `json:"leader,omitempty"`

	// The following are for CAAS models.
	ProviderId string `json:"provider-id,omitempty"`
	Address    string `json:"address,omitempty"`
}

// RelationStatus holds status info about a relation.
type RelationStatus struct {
	Id        int              `json:"id"`
	Key       string           `json:"key"`
	Interface string           `json:"interface"`
	Scope     string           `json:"scope"`
	Endpoints []EndpointStatus `json:"endpoints"`
	Status    DetailedStatus   `json:"status"`
}

// EndpointStatus holds status info about a single endpoint.
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

// History holds many DetailedStatus.
type History struct {
	Statuses []DetailedStatus `json:"statuses"`
	Error    *Error           `json:"error,omitempty"`
}

// StatusHistoryFilter holds arguments that can be use to filter a status history backlog.
type StatusHistoryFilter struct {
	Size    int            `json:"size"`
	Date    *time.Time     `json:"date"`
	Delta   *time.Duration `json:"delta"`
	Exclude []string       `json:"exclude"`
}

// StatusHistoryRequest holds the parameters to filter a status history query.
type StatusHistoryRequest struct {
	Kind   string              `json:"historyKind"`
	Size   int                 `json:"size"`
	Filter StatusHistoryFilter `json:"filter"`
	Tag    string              `json:"tag"`
}

// StatusHistoryRequests holds a slice of StatusHistoryArgs.
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

// ApplicationStatusResult holds results for an application Full Status.
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

// RelationStatusValue describes the status of a relation.
type RelationStatusValue relation.Status

const (
	Joined    RelationStatusValue = "joined"
	Suspended RelationStatusValue = "suspended"
	Broken    RelationStatusValue = "broken"
)
