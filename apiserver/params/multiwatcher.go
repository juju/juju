// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"github.com/juju/charm/v7"
	"github.com/juju/errors"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/status"
)

// EntityInfo is implemented by all entity Info types.
type EntityInfo interface {
	// EntityId returns an identifier that will uniquely
	// identify the entity within its kind
	EntityId() EntityId
}

// EntityId uniquely identifies an entity being tracked by the
// multiwatcherStore.
type EntityId struct {
	Kind      string `json:"kind"`
	ModelUUID string `json:"model-uuid"`
	Id        string `json:"id"`
}

// Delta holds details of a change to the model.
type Delta struct {
	// If Removed is true, the entity has been removed;
	// otherwise it has been created or changed.
	Removed bool `json:"removed"`
	// Entity holds data about the entity that has changed.
	Entity EntityInfo `json:"entity"`
}

// MarshalJSON implements json.Marshaler.
func (d *Delta) MarshalJSON() ([]byte, error) {
	b, err := json.Marshal(d.Entity)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	buf.WriteByte('[')
	c := "change"
	if d.Removed {
		c = "remove"
	}
	fmt.Fprintf(&buf, "%q,%q,", d.Entity.EntityId().Kind, c)
	buf.Write(b)
	buf.WriteByte(']')
	return buf.Bytes(), nil
}

// UnmarshalJSON implements json.Unmarshaler.
func (d *Delta) UnmarshalJSON(data []byte) error {
	var elements []json.RawMessage
	if err := json.Unmarshal(data, &elements); err != nil {
		return err
	}
	if len(elements) != 3 {
		return fmt.Errorf(
			"Expected 3 elements in top-level of JSON but got %d",
			len(elements))
	}
	var entityKind, operation string
	if err := json.Unmarshal(elements[0], &entityKind); err != nil {
		return err
	}
	if err := json.Unmarshal(elements[1], &operation); err != nil {
		return err
	}
	if operation == "remove" {
		d.Removed = true
	} else if operation != "change" {
		return fmt.Errorf("Unexpected operation %q", operation)
	}
	switch entityKind {
	case "model":
		d.Entity = new(ModelUpdate)
	case "machine":
		d.Entity = new(MachineInfo)
	case "application":
		d.Entity = new(ApplicationInfo)
	case "remoteApplication":
		d.Entity = new(RemoteApplicationUpdate)
	case "unit":
		d.Entity = new(UnitInfo)
	case "relation":
		d.Entity = new(RelationInfo)
	case "annotation":
		d.Entity = new(AnnotationInfo)
	case "block":
		d.Entity = new(BlockInfo)
	case "action":
		d.Entity = new(ActionInfo)
	case "charm":
		d.Entity = new(CharmInfo)
	case "branch":
		d.Entity = new(BranchInfo)
	default:
		return errors.Errorf("Unexpected entity name %q", entityKind)
	}
	return json.Unmarshal(elements[2], &d.Entity)
}

// MachineInfo holds the information about a machine
// that is tracked by multiwatcherStore.
type MachineInfo struct {
	ModelUUID                string                            `json:"model-uuid"`
	Id                       string                            `json:"id"`
	InstanceId               string                            `json:"instance-id"`
	AgentStatus              StatusInfo                        `json:"agent-status"`
	InstanceStatus           StatusInfo                        `json:"instance-status"`
	Life                     life.Value                        `json:"life"`
	Config                   map[string]interface{}            `json:"config,omitempty"`
	Series                   string                            `json:"series"`
	ContainerType            string                            `json:"container-type"`
	SupportedContainers      []instance.ContainerType          `json:"supported-containers"`
	SupportedContainersKnown bool                              `json:"supported-containers-known"`
	HardwareCharacteristics  *instance.HardwareCharacteristics `json:"hardware-characteristics,omitempty"`
	CharmProfiles            []string                          `json:"charm-profiles,omitempty"`
	Jobs                     []model.MachineJob                `json:"jobs"`
	Addresses                []Address                         `json:"addresses"`
	HasVote                  bool                              `json:"has-vote"`
	WantsVote                bool                              `json:"wants-vote"`
}

// EntityId returns a unique identifier for a machine across
// models.
func (i *MachineInfo) EntityId() EntityId {
	return EntityId{
		Kind:      "machine",
		ModelUUID: i.ModelUUID,
		Id:        i.Id,
	}
}

// StatusInfo holds the unit and machine status information. It is
// used by ApplicationInfo and UnitInfo.
type StatusInfo struct {
	Err     error                  `json:"err,omitempty"`
	Current status.Status          `json:"current"`
	Message string                 `json:"message"`
	Since   *time.Time             `json:"since,omitempty"`
	Version string                 `json:"version"`
	Data    map[string]interface{} `json:"data,omitempty"`
}

// ApplicationInfo holds the information about an application that is tracked
// by multiwatcherStore.
type ApplicationInfo struct {
	ModelUUID       string                 `json:"model-uuid"`
	Name            string                 `json:"name"`
	Exposed         bool                   `json:"exposed"`
	CharmURL        string                 `json:"charm-url"`
	OwnerTag        string                 `json:"owner-tag"`
	Life            life.Value             `json:"life"`
	MinUnits        int                    `json:"min-units"`
	Constraints     constraints.Value      `json:"constraints"`
	Config          map[string]interface{} `json:"config,omitempty"`
	Subordinate     bool                   `json:"subordinate"`
	Status          StatusInfo             `json:"status"`
	WorkloadVersion string                 `json:"workload-version"`
}

// EntityId returns a unique identifier for an application across
// models.
func (i *ApplicationInfo) EntityId() EntityId {
	return EntityId{
		Kind:      "application",
		ModelUUID: i.ModelUUID,
		Id:        i.Name,
	}
}

// Profile is a representation of charm.v6 LXDProfile
type Profile struct {
	Config      map[string]string            `json:"config,omitempty"`
	Description string                       `json:"description,omitempty"`
	Devices     map[string]map[string]string `json:"devices,omitempty"`
}

// CharmInfo holds the information about a charm that is tracked by the
// multiwatcher.
type CharmInfo struct {
	ModelUUID    string     `json:"model-uuid"`
	CharmURL     string     `json:"charm-url"`
	CharmVersion string     `json:"charm-version"`
	Life         life.Value `json:"life"`
	LXDProfile   *Profile   `json:"profile"`
	// DefaultConfig is derived from state-stored *charm.Config.
	DefaultConfig map[string]interface{} `json:"config,omitempty"`
}

// EntityId returns a unique identifier for an charm across
// models.
func (i *CharmInfo) EntityId() EntityId {
	return EntityId{
		Kind:      "charm",
		ModelUUID: i.ModelUUID,
		Id:        i.CharmURL,
	}
}

// RemoteApplicationUpdate holds the information about a remote application that is
// tracked by multiwatcherStore.
type RemoteApplicationUpdate struct {
	ModelUUID string     `json:"model-uuid"`
	Name      string     `json:"name"`
	OfferUUID string     `json:"offer-uuid"`
	OfferURL  string     `json:"offer-url"`
	Life      life.Value `json:"life"`
	Status    StatusInfo `json:"status"`
}

// EntityId returns a unique identifier for a remote application across models.
func (i *RemoteApplicationUpdate) EntityId() EntityId {
	return EntityId{
		Kind:      "remoteApplication",
		ModelUUID: i.ModelUUID,
		Id:        i.Name,
	}
}

// ApplicationOfferInfo holds the information about an application offer that is
// tracked by multiwatcherStore.
type ApplicationOfferInfo struct {
	ModelUUID            string `json:"model-uuid"`
	OfferName            string `json:"offer-name"`
	OfferUUID            string `json:"offer-uuid"`
	ApplicationName      string `json:"application-name"`
	CharmName            string `json:"charm-name"`
	TotalConnectedCount  int    `json:"total-connected-count"`
	ActiveConnectedCount int    `json:"active-connected-count"`
}

// EntityId returns a unique identifier for an application offer across models.
func (i *ApplicationOfferInfo) EntityId() EntityId {
	return EntityId{
		Kind:      "applicationOffer",
		ModelUUID: i.ModelUUID,
		Id:        i.OfferName,
	}
}

// UnitInfo holds the information about a unit
// that is tracked by multiwatcherStore.
type UnitInfo struct {
	ModelUUID      string      `json:"model-uuid"`
	Name           string      `json:"name"`
	Application    string      `json:"application"`
	Series         string      `json:"series"`
	CharmURL       string      `json:"charm-url"`
	Life           life.Value  `json:"life"`
	PublicAddress  string      `json:"public-address"`
	PrivateAddress string      `json:"private-address"`
	MachineId      string      `json:"machine-id"`
	Ports          []Port      `json:"ports"`
	PortRanges     []PortRange `json:"port-ranges"`
	Principal      string      `json:"principal"`
	Subordinate    bool        `json:"subordinate"`
	// Workload and agent state are modelled separately.
	WorkloadStatus StatusInfo `json:"workload-status"`
	AgentStatus    StatusInfo `json:"agent-status"`
}

// EntityId returns a unique identifier for a unit across
// models.
func (i *UnitInfo) EntityId() EntityId {
	return EntityId{
		Kind:      "unit",
		ModelUUID: i.ModelUUID,
		Id:        i.Name,
	}
}

// ActionInfo holds the information about a action that is tracked by
// multiwatcherStore.
type ActionInfo struct {
	ModelUUID  string                 `json:"model-uuid"`
	Id         string                 `json:"id"`
	Receiver   string                 `json:"receiver"`
	Name       string                 `json:"name"`
	Parameters map[string]interface{} `json:"parameters,omitempty"`
	Status     string                 `json:"status"`
	Message    string                 `json:"message"`
	Results    map[string]interface{} `json:"results,omitempty"`
	Enqueued   time.Time              `json:"enqueued"`
	Started    time.Time              `json:"started"`
	Completed  time.Time              `json:"completed"`
}

// EntityId returns a unique identifier for an action across
// models.
func (i *ActionInfo) EntityId() EntityId {
	return EntityId{
		Kind:      "action",
		ModelUUID: i.ModelUUID,
		Id:        i.Id,
	}
}

// RelationInfo holds the information about a relation that is tracked
// by multiwatcherStore.
type RelationInfo struct {
	ModelUUID string     `json:"model-uuid"`
	Key       string     `json:"key"`
	Id        int        `json:"id"`
	Endpoints []Endpoint `json:"endpoints"`
}

// NewCharmRelation creates a new local CharmRelation structure from  the
// charm.Relation structure. NOTE: when we update the database to not store a
// charm.Relation directly in the database, this method should take the state
// structure type.
func NewCharmRelation(cr charm.Relation) CharmRelation {
	return CharmRelation{
		Name:      cr.Name,
		Role:      string(cr.Role),
		Interface: cr.Interface,
		Optional:  cr.Optional,
		Limit:     cr.Limit,
		Scope:     string(cr.Scope),
	}
}

// Endpoint holds an application-relation pair.
type Endpoint struct {
	ApplicationName string        `json:"application-name"`
	Relation        CharmRelation `json:"relation"`
}

// EntityId returns a unique identifier for a relation across
// models.
func (i *RelationInfo) EntityId() EntityId {
	return EntityId{
		Kind:      "relation",
		ModelUUID: i.ModelUUID,
		Id:        i.Key,
	}
}

// AnnotationInfo holds the information about an annotation that is
// tracked by multiwatcherStore.
type AnnotationInfo struct {
	ModelUUID   string            `json:"model-uuid"`
	Tag         string            `json:"tag"`
	Annotations map[string]string `json:"annotations"`
}

// EntityId returns a unique identifier for an annotation across
// models.
func (i *AnnotationInfo) EntityId() EntityId {
	return EntityId{
		Kind:      "annotation",
		ModelUUID: i.ModelUUID,
		Id:        i.Tag,
	}
}

// BlockInfo holds the information about a block that is tracked by
// multiwatcherStore.
type BlockInfo struct {
	ModelUUID string          `json:"model-uuid"`
	Id        string          `json:"id"`
	Type      model.BlockType `json:"type"`
	Message   string          `json:"message"`
	Tag       string          `json:"tag"`
}

// EntityId returns a unique identifier for a block across
// models.
func (i *BlockInfo) EntityId() EntityId {
	return EntityId{
		Kind:      "block",
		ModelUUID: i.ModelUUID,
		Id:        i.Id,
	}
}

// ModelUpdate holds the information about a model that is
// tracked by multiwatcherStore.
type ModelUpdate struct {
	ModelUUID      string                 `json:"model-uuid"`
	Name           string                 `json:"name"`
	Life           life.Value             `json:"life"`
	Owner          string                 `json:"owner"`
	ControllerUUID string                 `json:"controller-uuid"`
	IsController   bool                   `json:"is-controller"`
	Config         map[string]interface{} `json:"config,omitempty"`
	Status         StatusInfo             `json:"status"`
	Constraints    constraints.Value      `json:"constraints"`
	SLA            ModelSLAInfo           `json:"sla"`
}

// EntityId returns a unique identifier for a model.
func (i *ModelUpdate) EntityId() EntityId {
	return EntityId{
		Kind:      "model",
		ModelUUID: i.ModelUUID,
		Id:        i.ModelUUID,
	}
}

// ItemChange is the multiwatcher representation of a core settings ItemChange.
type ItemChange struct {
	Type     int         `json:"type"`
	Key      string      `json:"key"`
	OldValue interface{} `json:"old,omitempty"`
	NewValue interface{} `json:"new,omitempty"`
}

// BranchInfo holds data about a model generation (branch)
// that is tracked by multiwatcherStore.
type BranchInfo struct {
	ModelUUID     string                  `json:"model-uuid"`
	Id            string                  `json:"id"`
	Name          string                  `json:"name"`
	AssignedUnits map[string][]string     `json:"assigned-units"`
	Config        map[string][]ItemChange `json:"charm-config"`
	Created       int64                   `json:"created"`
	CreatedBy     string                  `json:"created-by"`
	Completed     int64                   `json:"completed"`
	CompletedBy   string                  `json:"completed-by"`
	GenerationId  int                     `json:"generation-id"`
}

// EntityId returns a unique identifier for a generation.
func (i *BranchInfo) EntityId() EntityId {
	return EntityId{
		Kind:      "branch",
		ModelUUID: i.ModelUUID,
		Id:        i.Id,
	}
}
