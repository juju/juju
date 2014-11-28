// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"time"

	"github.com/juju/utils/exec"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/tools"
	"github.com/juju/juju/version"
)

// MachineContainersParams holds the arguments for making a SetSupportedContainers
// API call.
type MachineContainersParams struct {
	Params []MachineContainers
}

// MachineContainers holds the arguments for making an SetSupportedContainers call
// on a given machine.
type MachineContainers struct {
	MachineTag     string
	ContainerTypes []instance.ContainerType
}

// WatchContainer identifies a single container type within a machine.
type WatchContainer struct {
	MachineTag    string
	ContainerType string
}

// WatchContainers holds the arguments for making a WatchContainers
// API call.
type WatchContainers struct {
	Params []WatchContainer
}

// CharmURL identifies a single charm URL.
type CharmURL struct {
	URL string
}

// CharmURLs identifies multiple charm URLs.
type CharmURLs struct {
	URLs []CharmURL
}

// StringsResult holds the result of an API call that returns a slice
// of strings or an error.
type StringsResult struct {
	Error  *Error
	Result []string
}

// PortsResults holds the bulk operation result of an API call
// that returns a slice of network.Port.
type PortsResults struct {
	Results []PortsResult
}

// PortsResult holds the result of an API call that returns a slice
// of network.Port or an error.
type PortsResult struct {
	Error *Error
	Ports []network.Port
}

// MachinePorts holds a machine and network tags. It's used when
// referring to opened ports on the machine for a network.
type MachinePorts struct {
	MachineTag string
	NetworkTag string
}

// MachinePortRange holds a single port range open on a machine for
// the given unit and relation tags.
type MachinePortRange struct {
	UnitTag     string
	RelationTag string
	PortRange   network.PortRange
}

// MachinePortsParams holds the arguments for making a
// FirewallerAPIV1.GetMachinePorts() API call.
type MachinePortsParams struct {
	Params []MachinePorts
}

// MachinePortsResult holds a single result of the
// FirewallerAPIV1.GetMachinePorts() and UniterAPI.AllMachinePorts()
// API calls.
type MachinePortsResult struct {
	Error *Error
	Ports []MachinePortRange
}

// MachinePortsResults holds all the results of the
// FirewallerAPIV1.GetMachinePorts() and UniterAPI.AllMachinePorts()
// API calls.
type MachinePortsResults struct {
	Results []MachinePortsResult
}

// StringsResults holds the bulk operation result of an API call
// that returns a slice of strings or an error.
type StringsResults struct {
	Results []StringsResult
}

// StringResult holds a string or an error.
type StringResult struct {
	Error  *Error
	Result string
}

// StringResults holds the bulk operation result of an API call
// that returns a string or an error.
type StringResults struct {
	Results []StringResult
}

// EnvironmentResult holds the result of an API call returning a name and UUID
// for an environment.
type EnvironmentResult struct {
	Error *Error
	Name  string
	UUID  string
}

// ResolvedModeResult holds a resolved mode or an error.
type ResolvedModeResult struct {
	Error *Error
	Mode  ResolvedMode
}

// ResolvedModeResults holds the bulk operation result of an API call
// that returns a resolved mode or an error.
type ResolvedModeResults struct {
	Results []ResolvedModeResult
}

// StringBoolResult holds the result of an API call that returns a
// string and a boolean.
type StringBoolResult struct {
	Error  *Error
	Result string
	Ok     bool
}

// StringBoolResults holds multiple results with a string and a bool
// each.
type StringBoolResults struct {
	Results []StringBoolResult
}

// BoolResult holds the result of an API call that returns a
// a boolean or an error.
type BoolResult struct {
	Error  *Error
	Result bool
}

// BoolResults holds multiple results with BoolResult each.
type BoolResults struct {
	Results []BoolResult
}

// RelationSettings holds relation settings names and values.
type RelationSettings map[string]string

// RelationSettingsResult holds a relation settings map or an error.
type RelationSettingsResult struct {
	Error    *Error
	Settings RelationSettings
}

// RelationSettingsResults holds the result of an API calls that
// returns settings for multiple relations.
type RelationSettingsResults struct {
	Results []RelationSettingsResult
}

// ConfigSettings holds unit, service or cham configuration settings
// with string keys and arbitrary values.
type ConfigSettings map[string]interface{}

// ConfigSettingsResult holds a configuration map or an error.
type ConfigSettingsResult struct {
	Error    *Error
	Settings ConfigSettings
}

// ConfigSettingsResults holds multiple configuration maps or errors.
type ConfigSettingsResults struct {
	Results []ConfigSettingsResult
}

// EnvironConfig holds an environment configuration.
type EnvironConfig map[string]interface{}

// EnvironConfigResult holds environment configuration or an error.
type EnvironConfigResult struct {
	Config EnvironConfig
}

// RelationUnit holds a relation and a unit tag.
type RelationUnit struct {
	Relation string
	Unit     string
}

// RelationUnits holds the parameters for API calls expecting a pair
// of relation and unit tags.
type RelationUnits struct {
	RelationUnits []RelationUnit
}

// RelationIds holds multiple relation ids.
type RelationIds struct {
	RelationIds []int
}

// RelationUnitPair holds a relation tag, a local and remote unit tags.
type RelationUnitPair struct {
	Relation   string
	LocalUnit  string
	RemoteUnit string
}

// RelationUnitPairs holds the parameters for API calls expecting
// multiple sets of a relation tag, a local and remote unit tags.
type RelationUnitPairs struct {
	RelationUnitPairs []RelationUnitPair
}

// RelationUnitSettings holds a relation tag, a unit tag and local
// unit settings.
type RelationUnitSettings struct {
	Relation string
	Unit     string
	Settings RelationSettings
}

// RelationUnitsSettings holds the arguments for making a EnterScope
// or WriteSettings API calls.
type RelationUnitsSettings struct {
	RelationUnits []RelationUnitSettings
}

// RelationResult returns information about a single relation,
// or an error.
type RelationResult struct {
	Error    *Error
	Life     Life
	Id       int
	Key      string
	Endpoint multiwatcher.Endpoint
}

// RelationResults holds the result of an API call that returns
// information about multiple relations.
type RelationResults struct {
	Results []RelationResult
}

// EntityPort holds an entity's tag, a protocol and a port.
type EntityPort struct {
	Tag      string
	Protocol string
	Port     int
}

// EntitiesPorts holds the parameters for making an OpenPort or
// ClosePort on some entities.
type EntitiesPorts struct {
	Entities []EntityPort
}

// EntityPortRange holds an entity's tag, a protocol and a port range.
type EntityPortRange struct {
	Tag      string
	Protocol string
	FromPort int
	ToPort   int
}

// EntitiesPortRanges holds the parameters for making an OpenPorts or
// ClosePorts on some entities.
type EntitiesPortRanges struct {
	Entities []EntityPortRange
}

// EntityCharmURL holds an entity's tag and a charm URL.
type EntityCharmURL struct {
	Tag      string
	CharmURL string
}

// EntitiesCharmURL holds the parameters for making a SetCharmURL API
// call.
type EntitiesCharmURL struct {
	Entities []EntityCharmURL
}

// BytesResult holds the result of an API call that returns a slice
// of bytes.
type BytesResult struct {
	Result []byte
}

// LifeResult holds the life status of a single entity, or an error
// indicating why it is not available.
type LifeResult struct {
	Life  Life
	Error *Error
}

// LifeResults holds the life or error status of multiple entities.
type LifeResults struct {
	Results []LifeResult
}

// MachineSetProvisioned holds a machine tag, provider-specific
// instance id, a nonce, or an error.
//
// NOTE: This is deprecated since 1.19.0 and not used by the
// provisioner, it's just retained for backwards-compatibility and
// should be removed.
type MachineSetProvisioned struct {
	Tag             string
	InstanceId      instance.Id
	Nonce           string
	Characteristics *instance.HardwareCharacteristics
}

// SetProvisioned holds the parameters for making a SetProvisioned
// call for a machine.
//
// NOTE: This is deprecated since 1.19.0 and not used by the
// provisioner, it's just retained for backwards-compatibility and
// should be removed.
type SetProvisioned struct {
	Machines []MachineSetProvisioned
}

// Network describes a single network available on an instance.
type Network struct {
	// Tag is the network's tag.
	Tag string

	// ProviderId is the provider-specific network id.
	ProviderId network.Id

	// CIDR of the network, in "123.45.67.89/12" format.
	CIDR string

	// VLANTag needs to be between 1 and 4094 for VLANs and 0 for
	// normal networks. It's defined by IEEE 802.1Q standard.
	VLANTag int
}

// NetworkInterface describes a single network interface available on
// an instance.
type NetworkInterface struct {
	// MACAddress is the network interface's hardware MAC address
	// (e.g. "aa:bb:cc:dd:ee:ff").
	MACAddress string

	// InterfaceName is the OS-specific network device name (e.g.
	// "eth1", even for for a VLAN eth1.42 virtual interface).
	InterfaceName string

	// NetworkTag is this interface's network tag.
	NetworkTag string

	// IsVirtual is true when the interface is a virtual device, as
	// opposed to a physical device.
	IsVirtual bool

	// Disabled returns whether the interface is disabled.
	Disabled bool
}

// InstanceInfo holds a machine tag, provider-specific instance id, a
// nonce, a list of networks and interfaces to set up.
type InstanceInfo struct {
	Tag             string
	InstanceId      instance.Id
	Nonce           string
	Characteristics *instance.HardwareCharacteristics
	Networks        []Network
	Interfaces      []NetworkInterface
}

// InstancesInfo holds the parameters for making a SetInstanceInfo
// call for multiple machines.
type InstancesInfo struct {
	Machines []InstanceInfo
}

// RequestedNetworkResult holds requested networks or an error.
type RequestedNetworkResult struct {
	Error    *Error
	Networks []string
}

// RequestedNetworksResults holds multiple requested networks results.
type RequestedNetworksResults struct {
	Results []RequestedNetworkResult
}

// MachineNetworkInfoResult holds network info for a single machine.
type MachineNetworkInfoResult struct {
	Error *Error
	Info  []network.Info
}

// MachineNetworkInfoResults holds network info for multiple machines.
type MachineNetworkInfoResults struct {
	Results []MachineNetworkInfoResult
}

// EntityStatus holds an entity tag, status and extra info.
type EntityStatus struct {
	Tag    string
	Status Status
	Info   string
	Data   map[string]interface{}
}

// SetStatus holds the parameters for making a SetStatus/UpdateStatus call.
type SetStatus struct {
	Entities []EntityStatus
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
}

// StatusResults holds multiple status results.
type StatusResults struct {
	Results []StatusResult
}

// MachineAddresses holds an machine tag and addresses.
type MachineAddresses struct {
	Tag       string
	Addresses []network.Address
}

// SetMachinesAddresses holds the parameters for making a SetMachineAddresses call.
type SetMachinesAddresses struct {
	MachineAddresses []MachineAddresses
}

// ConstraintsResult holds machine constraints or an error.
type ConstraintsResult struct {
	Error       *Error
	Constraints constraints.Value
}

// ConstraintsResults holds multiple constraints results.
type ConstraintsResults struct {
	Results []ConstraintsResult
}

// AgentGetEntitiesResults holds the results of a
// agent.API.GetEntities call.
type AgentGetEntitiesResults struct {
	Entities []AgentGetEntitiesResult
}

// AgentGetEntitiesResult holds the results of a
// machineagent.API.GetEntities call for a single entity.
type AgentGetEntitiesResult struct {
	Life          Life
	Jobs          []multiwatcher.MachineJob
	ContainerType instance.ContainerType
	Error         *Error
}

// VersionResult holds the version and possibly error for a given
// DesiredVersion() API call.
type VersionResult struct {
	Version *version.Number
	Error   *Error
}

// VersionResults is a list of versions for the requested entities.
type VersionResults struct {
	Results []VersionResult
}

// ToolsResult holds the tools and possibly error for a given
// Tools() API call.
type ToolsResult struct {
	Tools                          *tools.Tools
	DisableSSLHostnameVerification bool
	Error                          *Error
}

// ToolsResults is a list of tools for various requested agents.
type ToolsResults struct {
	Results []ToolsResult
}

// Version holds a specific binary version.
type Version struct {
	Version version.Binary
}

// EntityVersion specifies the tools version to be set for an entity
// with the given tag.
// version.Binary directly.
type EntityVersion struct {
	Tag   string
	Tools *Version
}

// EntitiesVersion specifies what tools are being run for
// multiple entities.
type EntitiesVersion struct {
	AgentTools []EntityVersion
}

// NotifyWatchResult holds a NotifyWatcher id and an error (if any).
type NotifyWatchResult struct {
	NotifyWatcherId string
	Error           *Error
}

// NotifyWatchResults holds the results for any API call which ends up
// returning a list of NotifyWatchers
type NotifyWatchResults struct {
	Results []NotifyWatchResult
}

// StringsWatchResult holds a StringsWatcher id, changes and an error
// (if any).
type StringsWatchResult struct {
	StringsWatcherId string
	Changes          []string
	Error            *Error
}

// StringsWatchResults holds the results for any API call which ends up
// returning a list of StringsWatchers.
type StringsWatchResults struct {
	Results []StringsWatchResult
}

// RelationUnitsWatchResult holds a RelationUnitsWatcher id, changes
// and an error (if any).
type RelationUnitsWatchResult struct {
	RelationUnitsWatcherId string
	Changes                multiwatcher.RelationUnitsChange
	Error                  *Error
}

// RelationUnitsWatchResults holds the results for any API call which ends up
// returning a list of RelationUnitsWatchers.
type RelationUnitsWatchResults struct {
	Results []RelationUnitsWatchResult
}

// CharmsResponse is the server response to charm upload or GET requests.
type CharmsResponse struct {
	Error    string   `json:",omitempty"`
	CharmURL string   `json:",omitempty"`
	Files    []string `json:",omitempty"`
}

// RunParams is used to provide the parameters to the Run method.
// Commands and Timeout are expected to have values, and one or more
// values should be in the Machines, Services, or Units slices.
type RunParams struct {
	Commands string
	Timeout  time.Duration
	Machines []string
	Services []string
	Units    []string
}

// RunResult contains the result from an individual run call on a machine.
// UnitId is populated if the command was run inside the unit context.
type RunResult struct {
	exec.ExecResponse
	MachineId string
	UnitId    string
	Error     string
}

// RunResults is used to return the slice of results.  API server side calls
// need to return single structure values.
type RunResults struct {
	Results []RunResult
}

// AgentVersionResult is used to return the current version number of the
// agent running the API server.
type AgentVersionResult struct {
	Version version.Number
}

// ProvisioningInfo holds machine provisioning info.
type ProvisioningInfo struct {
	Constraints constraints.Value
	Series      string
	Placement   string
	Networks    []string
	Jobs        []multiwatcher.MachineJob
}

// ProvisioningInfoResult holds machine provisioning info or an error.
type ProvisioningInfoResult struct {
	Error  *Error
	Result *ProvisioningInfo
}

// ProvisioningInfoResults holds multiple machine provisioning info results.
type ProvisioningInfoResults struct {
	Results []ProvisioningInfoResult
}

// Metric holds a single metric.
type Metric struct {
	Key   string
	Value string
	Time  time.Time
}

// MetricsParam contains the metrics for a single unit.
type MetricsParam struct {
	Tag     string
	Metrics []Metric
}

// MetricsParams contains the metrics for multiple units.
type MetricsParams struct {
	Metrics []MetricsParam
}

// MeterStatusResult holds unit meter status or error.
type MeterStatusResult struct {
	Code  string
	Info  string
	Error *Error
}

// MeterStatusResults holds meter status results for multiple units.
type MeterStatusResults struct {
	Results []MeterStatusResult
}

// MachineBlockDevices holds a machine tag and the block devices present
// on that machine.
type MachineBlockDevices struct {
	Machine      string
	BlockDevices []storage.BlockDevice
}

// SetMachineBlockDevices holds the arguments for recording the block
// devices present on a set of machines.
type SetMachineBlockDevices struct {
	MachineBlockDevices []MachineBlockDevices
}

// BlockDeviceResult holds the result of an API call to retrieve details
// of a block device.
type BlockDeviceResult struct {
	Result storage.BlockDevice `json:"result"`
	Error  *Error              `json:"error,omitempty"`
}

// BlockDeviceResults holds the result of an API call to retrieve details
// of multiple block devices.
type BlockDeviceResults struct {
	Results []BlockDeviceResult `json:"results,omitempty"`
}

// DatastoreFilesystem holds the parameters for recording information about
// the filesystem corresponding to the specified datastore.
type DatastoreFilesystem struct {
	DatastoreId storage.DatastoreId `json:"datastoreid"`
	Filesystem  storage.Filesystem  `json:"filesystem"`
}
