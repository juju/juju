// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package multiwatcher

import (
	"time"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/status"
)

// The kind constants are there to stop typos when switching on kinds.
const (
	ActionKind            = "action"
	AnnotationKind        = "annotation" // the annotations should really be parts of the other entities
	ApplicationKind       = "application"
	ApplicationOfferKind  = "applicationOffer"
	BlockKind             = "block"
	BranchKind            = "branch"
	CharmKind             = "charm"
	MachineKind           = "machine"
	ModelKind             = "model"
	RelationKind          = "relation"
	RemoteApplicationKind = "remoteApplication"
	UnitKind              = "unit"
)

// Factory is used to create multiwatchers.
type Factory interface {
	// TODO: WatchUsersModels to filter just the user's models
	WatchModel(modelUUID string) Watcher
	WatchController() Watcher
}

// Watcher is the way a caller can find out what changes have happened
// on one or more models.
type Watcher interface {
	Stop() error
	Next() ([]Delta, error)
}

// EntityInfo is implemented by all entity Info types.
type EntityInfo interface {
	// EntityID returns an identifier that will uniquely
	// identify the entity within its kind
	EntityID() EntityID

	// Clone returns a clone of the EntityInfo.
	Clone() EntityInfo
}

// EntityID uniquely identifies an entity being tracked by the
// multiwatcherStore.
type EntityID struct {
	Kind      string
	ModelUUID string
	ID        string
}

// Delta holds details of a change to the model.
type Delta struct {
	// If Removed is true, the entity has been removed;
	// otherwise it has been created or changed.
	Removed bool
	// Entity holds data about the entity that has changed.
	Entity EntityInfo
}

// MachineInfo holds the information about a machine
// that is tracked by multiwatcherStore.
type MachineInfo struct {
	ModelUUID                string
	ID                       string
	InstanceID               string
	AgentStatus              StatusInfo
	InstanceStatus           StatusInfo
	Life                     life.Value
	Annotations              map[string]string
	Config                   map[string]interface{}
	Series                   string
	ContainerType            string
	SupportedContainers      []instance.ContainerType
	SupportedContainersKnown bool
	HardwareCharacteristics  *instance.HardwareCharacteristics
	CharmProfiles            []string
	Jobs                     []model.MachineJob
	Addresses                []network.ProviderAddress
	HasVote                  bool
	WantsVote                bool

	// The preferred public and preferred private address are
	// only stored on the machine info to populate the unit info.
	PreferredPublicAddress  network.SpaceAddress
	PreferredPrivateAddress network.SpaceAddress
}

// EntityID returns a unique identifier for a machine across
// models.
func (i *MachineInfo) EntityID() EntityID {
	return EntityID{
		Kind:      MachineKind,
		ModelUUID: i.ModelUUID,
		ID:        i.ID,
	}
}

// Clone returns a clone of the EntityInfo.
func (i *MachineInfo) Clone() EntityInfo {
	clone := *i
	clone.HardwareCharacteristics = i.HardwareCharacteristics.Clone()
	if len(i.Annotations) > 0 {
		clone.Annotations = map[string]string{}
		for k, v := range i.Annotations {
			clone.Annotations[k] = v
		}
	}
	if len(i.Config) > 0 {
		clone.Config = map[string]interface{}{}
		for k, v := range i.Config {
			clone.Config[k] = v
		}
	}
	if len(i.SupportedContainers) > 0 {
		clone.SupportedContainers = make([]instance.ContainerType, len(i.SupportedContainers))
		for i, c := range i.SupportedContainers {
			clone.SupportedContainers[i] = c
		}
	}
	if len(i.CharmProfiles) > 0 {
		clone.CharmProfiles = make([]string, len(i.CharmProfiles))
		for i, c := range i.CharmProfiles {
			clone.CharmProfiles[i] = c
		}
	}
	if len(i.Jobs) > 0 {
		clone.Jobs = make([]model.MachineJob, len(i.Jobs))
		for i, j := range i.Jobs {
			clone.Jobs[i] = j
		}
	}
	if len(i.Addresses) > 0 {
		clone.Addresses = make([]network.ProviderAddress, len(i.Addresses))
		for i, a := range i.Addresses {
			clone.Addresses[i] = a
		}
	}
	return &clone
}

// StatusInfo holds the unit and machine status information. It is
// used by ApplicationInfo and UnitInfo.
type StatusInfo struct {
	Err     error
	Current status.Status
	Message string
	Since   *time.Time
	Version string
	Data    map[string]interface{}
}

// NewStatusInfo return a new multiwatcher StatusInfo from a
// status StatusInfo.
func NewStatusInfo(s status.StatusInfo, err error) StatusInfo {
	return StatusInfo{
		Err:     err,
		Current: s.Status,
		Message: s.Message,
		Since:   s.Since,
		Data:    s.Data,
	}
}

// ApplicationInfo holds the information about an application that is tracked
// by multiwatcherStore.
type ApplicationInfo struct {
	ModelUUID       string
	Name            string
	Exposed         bool
	CharmURL        string
	OwnerTag        string
	Life            life.Value
	MinUnits        int
	Constraints     constraints.Value
	Annotations     map[string]string
	Config          map[string]interface{}
	Subordinate     bool
	Status          StatusInfo
	WorkloadVersion string
}

// EntityID returns a unique identifier for an application across
// models.
func (i *ApplicationInfo) EntityID() EntityID {
	return EntityID{
		Kind:      ApplicationKind,
		ModelUUID: i.ModelUUID,
		ID:        i.Name,
	}
}

// Clone returns a clone of the EntityInfo.
func (i *ApplicationInfo) Clone() EntityInfo {
	clone := *i
	if len(i.Annotations) > 0 {
		clone.Annotations = map[string]string{}
		for k, v := range i.Annotations {
			clone.Annotations[k] = v
		}
	}
	if len(i.Config) > 0 {
		clone.Config = map[string]interface{}{}
		for k, v := range i.Config {
			clone.Config[k] = v
		}
	}
	return &clone
}

// Profile is a representation of charm.v6 LXDProfile
type Profile struct {
	Config      map[string]string
	Description string
	Devices     map[string]map[string]string
}

func (p *Profile) Clone() *Profile {
	if p == nil {
		return nil
	}
	clone := *p
	if len(p.Config) > 0 {
		clone.Config = map[string]string{}
		for k, v := range p.Config {
			clone.Config[k] = v
		}
	}
	if len(p.Devices) > 0 {
		clone.Devices = map[string]map[string]string{}
		for k, cfg := range p.Devices {
			var cCfg map[string]string
			if len(cfg) > 0 {
				cCfg = map[string]string{}
				for k, v := range cfg {
					cCfg[k] = v
				}
			}
			clone.Devices[k] = cCfg
		}
	}
	return &clone
}

// CharmInfo holds the information about a charm that is tracked by the
// multiwatcher.
type CharmInfo struct {
	ModelUUID    string
	CharmURL     string
	CharmVersion string
	Life         life.Value
	LXDProfile   *Profile
	// DefaultConfig is derived from state-stored *charm.Config.
	DefaultConfig map[string]interface{}
}

// EntityID returns a unique identifier for an charm across
// models.
func (i *CharmInfo) EntityID() EntityID {
	return EntityID{
		Kind:      CharmKind,
		ModelUUID: i.ModelUUID,
		ID:        i.CharmURL,
	}
}

// Clone returns a clone of the EntityInfo.
func (i *CharmInfo) Clone() EntityInfo {
	clone := *i
	clone.LXDProfile = i.LXDProfile.Clone()
	if len(i.DefaultConfig) > 0 {
		clone.DefaultConfig = map[string]interface{}{}
		for k, v := range i.DefaultConfig {
			clone.DefaultConfig[k] = v
		}
	}
	return &clone
}

// RemoteApplicationUpdate holds the information about a remote application that is
// tracked by multiwatcherStore.
type RemoteApplicationUpdate struct {
	ModelUUID string
	Name      string
	OfferUUID string
	OfferURL  string
	Life      life.Value
	Status    StatusInfo
}

// EntityID returns a unique identifier for a remote application across models.
func (i *RemoteApplicationUpdate) EntityID() EntityID {
	return EntityID{
		Kind:      RemoteApplicationKind,
		ModelUUID: i.ModelUUID,
		ID:        i.Name,
	}
}

// Clone returns a clone of the EntityInfo.
func (i *RemoteApplicationUpdate) Clone() EntityInfo {
	clone := *i
	return &clone
}

// ApplicationOfferInfo holds the information about an application offer that is
// tracked by multiwatcherStore.
type ApplicationOfferInfo struct {
	ModelUUID            string
	OfferName            string
	OfferUUID            string
	ApplicationName      string
	CharmName            string
	TotalConnectedCount  int
	ActiveConnectedCount int
}

// EntityID returns a unique identifier for an application offer across models.
func (i *ApplicationOfferInfo) EntityID() EntityID {
	return EntityID{
		Kind:      ApplicationOfferKind,
		ModelUUID: i.ModelUUID,
		ID:        i.OfferName,
	}
}

// Clone returns a clone of the EntityInfo.
func (i *ApplicationOfferInfo) Clone() EntityInfo {
	clone := *i
	return &clone
}

// UnitInfo holds the information about a unit
// that is tracked by multiwatcherStore.
type UnitInfo struct {
	ModelUUID      string
	Name           string
	Application    string
	Series         string
	CharmURL       string
	Life           life.Value
	Annotations    map[string]string
	PublicAddress  string
	PrivateAddress string
	MachineID      string
	Ports          []network.Port
	PortRanges     []network.PortRange
	Principal      string
	Subordinate    bool
	// Workload and agent state are modelled separately.
	WorkloadStatus StatusInfo
	AgentStatus    StatusInfo
}

// EntityID returns a unique identifier for a unit across
// models.
func (i *UnitInfo) EntityID() EntityID {
	return EntityID{
		Kind:      UnitKind,
		ModelUUID: i.ModelUUID,
		ID:        i.Name,
	}
}

// Clone returns a clone of the EntityInfo.
func (i *UnitInfo) Clone() EntityInfo {
	clone := *i
	if len(i.Annotations) > 0 {
		clone.Annotations = map[string]string{}
		for k, v := range i.Annotations {
			clone.Annotations[k] = v
		}
	}
	if len(i.Ports) > 0 {
		clone.Ports = make([]network.Port, len(i.Ports))
		for i, p := range i.Ports {
			clone.Ports[i] = p
		}
	}
	if len(i.PortRanges) > 0 {
		clone.PortRanges = make([]network.PortRange, len(i.PortRanges))
		for i, p := range i.PortRanges {
			clone.PortRanges[i] = p
		}
	}
	return &clone
}

// ActionInfo holds the information about a action that is tracked by
// multiwatcherStore.
type ActionInfo struct {
	ModelUUID  string
	ID         string
	Receiver   string
	Name       string
	Parameters map[string]interface{}
	Status     string
	Message    string
	Results    map[string]interface{}
	Enqueued   time.Time
	Started    time.Time
	Completed  time.Time
}

// EntityID returns a unique identifier for an action across
// models.
func (i *ActionInfo) EntityID() EntityID {
	return EntityID{
		Kind:      ActionKind,
		ModelUUID: i.ModelUUID,
		ID:        i.ID,
	}
}

// Clone returns a clone of the EntityInfo.
func (i *ActionInfo) Clone() EntityInfo {
	clone := *i
	if len(i.Parameters) > 0 {
		clone.Parameters = map[string]interface{}{}
		for k, v := range i.Parameters {
			clone.Parameters[k] = v
		}
	}
	if len(i.Results) > 0 {
		clone.Results = map[string]interface{}{}
		for k, v := range i.Results {
			clone.Results[k] = v
		}
	}
	return &clone
}

// RelationInfo holds the information about a relation that is tracked
// by multiwatcherStore.
type RelationInfo struct {
	ModelUUID string
	Key       string
	ID        int
	Endpoints []Endpoint
}

// Endpoint holds an application-relation pair.
type Endpoint struct {
	ApplicationName string
	Relation        CharmRelation
}

// CharmRelation mirrors charm.Relation.
type CharmRelation struct {
	Name      string
	Role      string
	Interface string
	Optional  bool
	Limit     int
	Scope     string
}

// EntityID returns a unique identifier for a relation across
// models.
func (i *RelationInfo) EntityID() EntityID {
	return EntityID{
		Kind:      RelationKind,
		ModelUUID: i.ModelUUID,
		ID:        i.Key,
	}
}

// Clone returns a clone of the EntityInfo.
func (i *RelationInfo) Clone() EntityInfo {
	clone := *i
	if len(i.Endpoints) > 0 {
		clone.Endpoints = make([]Endpoint, len(i.Endpoints))
		for i, ep := range i.Endpoints {
			clone.Endpoints[i] = ep
		}
	}
	return &clone
}

// AnnotationInfo holds the information about an annotation that is
// tracked by multiwatcherStore.
type AnnotationInfo struct {
	ModelUUID   string
	Tag         string
	Annotations map[string]string
}

// EntityID returns a unique identifier for an annotation across
// models.
func (i *AnnotationInfo) EntityID() EntityID {
	return EntityID{
		Kind:      AnnotationKind,
		ModelUUID: i.ModelUUID,
		ID:        i.Tag,
	}
}

// Clone returns a clone of the EntityInfo.
func (i *AnnotationInfo) Clone() EntityInfo {
	clone := *i
	if len(i.Annotations) > 0 {
		clone.Annotations = map[string]string{}
		for k, v := range i.Annotations {
			clone.Annotations[k] = v
		}
	}
	return &clone
}

// BlockInfo holds the information about a block that is tracked by
// multiwatcherStore.
type BlockInfo struct {
	ModelUUID string
	ID        string
	Type      model.BlockType
	Message   string
	Tag       string
}

// EntityID returns a unique identifier for a block across
// models.
func (i *BlockInfo) EntityID() EntityID {
	return EntityID{
		Kind:      BlockKind,
		ModelUUID: i.ModelUUID,
		ID:        i.ID,
	}
}

// Clone returns a clone of the EntityInfo.
func (i *BlockInfo) Clone() EntityInfo {
	clone := *i
	return &clone
}

// ModelInfo holds the information about a model that is
// tracked by multiwatcherStore.
type ModelInfo struct {
	ModelUUID       string
	Type            model.ModelType
	Name            string
	Life            life.Value
	Owner           string
	ControllerUUID  string
	IsController    bool
	Cloud           string
	CloudRegion     string
	CloudCredential string
	Annotations     map[string]string
	Config          map[string]interface{}
	Status          StatusInfo
	Constraints     constraints.Value
	SLA             ModelSLAInfo

	UserPermissions map[string]permission.Access
}

// ModelSLAInfo describes the SLA info for a model.
type ModelSLAInfo struct {
	Level string
	Owner string
}

// EntityID returns a unique identifier for a model.
func (i *ModelInfo) EntityID() EntityID {
	return EntityID{
		Kind:      ModelKind,
		ModelUUID: i.ModelUUID,
		ID:        i.ModelUUID,
	}
}

// Clone returns a clone of the EntityInfo.
func (i *ModelInfo) Clone() EntityInfo {
	clone := *i
	if len(i.Annotations) > 0 {
		clone.Annotations = map[string]string{}
		for k, v := range i.Annotations {
			clone.Annotations[k] = v
		}
	}
	if len(i.Config) > 0 {
		clone.Config = map[string]interface{}{}
		for k, v := range i.Config {
			clone.Config[k] = v
		}
	}
	if len(i.UserPermissions) > 0 {
		clone.UserPermissions = map[string]permission.Access{}
		for k, v := range i.UserPermissions {
			clone.UserPermissions[k] = v
		}
	}
	return &clone
}

// ItemChange is the multiwatcher representation of a core settings ItemChange.
type ItemChange struct {
	Type     int
	Key      string
	OldValue interface{}
	NewValue interface{}
}

// BranchInfo holds data about a model branch
// that is tracked by multiwatcherStore.
type BranchInfo struct {
	ModelUUID     string
	ID            string
	Name          string
	AssignedUnits map[string][]string
	Config        map[string][]ItemChange
	Created       int64
	CreatedBy     string
	Completed     int64
	CompletedBy   string
	GenerationID  int
}

// EntityID returns a unique identifier for a generation.
func (i *BranchInfo) EntityID() EntityID {
	return EntityID{
		Kind:      BranchKind,
		ModelUUID: i.ModelUUID,
		ID:        i.ID,
	}
}

// Clone returns a clone of the EntityInfo.
func (i *BranchInfo) Clone() EntityInfo {
	clone := *i
	if len(i.AssignedUnits) > 0 {
		clone.AssignedUnits = map[string][]string{}
		for k, units := range i.AssignedUnits {
			cUnits := make([]string, len(units))
			for i, u := range units {
				cUnits[i] = u
			}
			clone.AssignedUnits[k] = cUnits
		}
	}
	if len(i.Config) > 0 {
		clone.Config = map[string][]ItemChange{}
		for k, itemChanges := range i.Config {
			cItems := make([]ItemChange, len(itemChanges))
			for i, c := range itemChanges {
				cItems[i] = c
			}
			clone.Config[k] = cItems
		}
	}
	return &clone
}
