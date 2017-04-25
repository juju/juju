// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package multiwatcher

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/status"
)

// Life describes the lifecycle state of an entity ("alive", "dying"
// or "dead").
type Life string

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
		d.Entity = new(ModelInfo)
	case "machine":
		d.Entity = new(MachineInfo)
	case "application":
		d.Entity = new(ApplicationInfo)
	case "remoteApplication":
		d.Entity = new(RemoteApplicationInfo)
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
	default:
		return errors.Errorf("Unexpected entity name %q", entityKind)
	}
	return json.Unmarshal(elements[2], &d.Entity)
}

// Address describes a network address.
type Address struct {
	Value           string `json:"value"`
	Type            string `json:"type"`
	Scope           string `json:"scope"`
	SpaceName       string `json:"space-name,omitempty"`
	SpaceProviderId string `json:"space-provider-id,omitempty"`
}

// MachineInfo holds the information about a machine
// that is tracked by multiwatcherStore.
type MachineInfo struct {
	ModelUUID                string                            `json:"model-uuid"`
	Id                       string                            `json:"id"`
	InstanceId               string                            `json:"instance-id"`
	AgentStatus              StatusInfo                        `json:"agent-status"`
	InstanceStatus           StatusInfo                        `json:"instance-status"`
	Life                     Life                              `json:"life"`
	Config                   map[string]interface{}            `json:"config,omitempty"`
	Series                   string                            `json:"series"`
	SupportedContainers      []instance.ContainerType          `json:"supported-containers"`
	SupportedContainersKnown bool                              `json:"supported-containers-known"`
	HardwareCharacteristics  *instance.HardwareCharacteristics `json:"hardware-characteristics,omitempty"`
	Jobs                     []MachineJob                      `json:"jobs"`
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
	ModelUUID   string                 `json:"model-uuid"`
	Name        string                 `json:"name"`
	Exposed     bool                   `json:"exposed"`
	CharmURL    string                 `json:"charm-url"`
	OwnerTag    string                 `json:"owner-tag"`
	Life        Life                   `json:"life"`
	MinUnits    int                    `json:"min-units"`
	Constraints constraints.Value      `json:"constraints"`
	Config      map[string]interface{} `json:"config,omitempty"`
	Subordinate bool                   `json:"subordinate"`
	Status      StatusInfo             `json:"status"`
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

// RemoteApplicationInfo holds the information about a remote application that is
// tracked by multiwatcherStore.
type RemoteApplicationInfo struct {
	ModelUUID      string     `json:"model-uuid"`
	Name           string     `json:"name"`
	ApplicationURL string     `json:"application-url"`
	Life           Life       `json:"life"`
	Status         StatusInfo `json:"status"`
}

// EntityId returns a unique identifier for a remote application across models.
func (i *RemoteApplicationInfo) EntityId() EntityId {
	return EntityId{
		Kind:      "remoteApplication",
		ModelUUID: i.ModelUUID,
		Id:        i.Name,
	}
}

// ApplicationOfferInfo holds the information about an application offer that is
// tracked by multiwatcherStore.
type ApplicationOfferInfo struct {
	ModelUUID       string `json:"model-uuid"`
	OfferName       string `json:"offer-name"`
	ApplicationName string `json:"application-name"`
	CharmName       string `json:"charm-name"`
	ConnectedCount  int    `json:"connected-count"`
}

// EntityId returns a unique identifier for an application offer across models.
func (i *ApplicationOfferInfo) EntityId() EntityId {
	return EntityId{
		Kind:      "applicationOffer",
		ModelUUID: i.ModelUUID,
		Id:        i.OfferName,
	}
}

// Port identifies a network port number for a particular protocol.
type Port struct {
	Protocol string `json:"protocol"`
	Number   int    `json:"number"`
}

// PortRange represents a single range of ports.
type PortRange struct {
	FromPort int    `json:"from-port"`
	ToPort   int    `json:"to-port"`
	Protocol string `json:"protocol"`
}

// UnitInfo holds the information about a unit
// that is tracked by multiwatcherStore.
type UnitInfo struct {
	ModelUUID      string      `json:"model-uuid"`
	Name           string      `json:"name"`
	Application    string      `json:"application"`
	Series         string      `json:"series"`
	CharmURL       string      `json:"charm-url"`
	PublicAddress  string      `json:"public-address"`
	PrivateAddress string      `json:"private-address"`
	MachineId      string      `json:"machine-id"`
	Ports          []Port      `json:"ports"`
	PortRanges     []PortRange `json:"port-ranges"`
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

// CharmRelation is a mirror struct for charm.Relation.
type CharmRelation struct {
	Name      string `json:"name"`
	Role      string `json:"role"`
	Interface string `json:"interface"`
	Optional  bool   `json:"optional"`
	Limit     int    `json:"limit"`
	Scope     string `json:"scope"`
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

// MachineJob values define responsibilities that machines may be
// expected to fulfil.
type MachineJob string

const (
	JobHostUnits   MachineJob = "JobHostUnits"
	JobManageModel MachineJob = "JobManageModel"
)

// NeedsState returns true if the job requires a state connection.
func (job MachineJob) NeedsState() bool {
	return job == JobManageModel
}

// AnyJobNeedsState returns true if any of the provided jobs
// require a state connection.
func AnyJobNeedsState(jobs ...MachineJob) bool {
	for _, j := range jobs {
		if j.NeedsState() {
			return true
		}
	}
	return false
}

// BlockInfo holds the information about a block that is tracked by
// multiwatcherStore.
type BlockInfo struct {
	ModelUUID string    `json:"model-uuid"`
	Id        string    `json:"id"`
	Type      BlockType `json:"type"`
	Message   string    `json:"message"`
	Tag       string    `json:"tag"`
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

// BlockType values define model block type.
type BlockType string

const (
	// BlockDestroy type identifies destroy blocks.
	BlockDestroy BlockType = "BlockDestroy"

	// BlockRemove type identifies remove blocks.
	BlockRemove BlockType = "BlockRemove"

	// BlockChange type identifies change blocks.
	BlockChange BlockType = "BlockChange"
)

// ModelInfo holds the information about an model that is
// tracked by multiwatcherStore.
type ModelInfo struct {
	ModelUUID      string                 `json:"model-uuid"`
	Name           string                 `json:"name"`
	Life           Life                   `json:"life"`
	Owner          string                 `json:"owner"`
	ControllerUUID string                 `json:"controller-uuid"`
	Config         map[string]interface{} `json:"config,omitempty"`
	Status         StatusInfo             `json:"status"`
	Constraints    constraints.Value      `json:"constraints"`
}

// EntityId returns a unique identifier for an model.
func (i *ModelInfo) EntityId() EntityId {
	return EntityId{
		Kind:      "model",
		ModelUUID: i.ModelUUID,
		Id:        i.ModelUUID,
	}
}
