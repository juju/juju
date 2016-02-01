// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package multiwatcher

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
)

// Life describes the lifecycle state of an entity ("alive", "dying"
// or "dead").
type Life string

// Status represents the status of an entity.
// It could be a service, unit, machine or its agent.
type Status string

// EntityInfo is implemented by all entity Info types.
type EntityInfo interface {
	// EntityId returns an identifier that will uniquely
	// identify the entity within its kind
	EntityId() EntityId
}

// EntityId uniquely identifies an entity being tracked by the
// multiwatcherStore.
type EntityId struct {
	Kind      string
	ModelUUID string
	Id        string
}

// Delta holds details of a change to the model.
type Delta struct {
	// If Removed is true, the entity has been removed;
	// otherwise it has been created or changed.
	Removed bool
	// Entity holds data about the entity that has changed.
	Entity EntityInfo
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
	case "service":
		d.Entity = new(ServiceInfo)
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
		return fmt.Errorf("Unexpected entity name %q", entityKind)
	}
	return json.Unmarshal(elements[2], &d.Entity)
}

// MachineInfo holds the information about a machine
// that is tracked by multiwatcherStore.
type MachineInfo struct {
	ModelUUID                string
	Id                       string
	InstanceId               string
	Status                   Status
	StatusInfo               string
	StatusData               map[string]interface{}
	Life                     Life
	Series                   string
	SupportedContainers      []instance.ContainerType
	SupportedContainersKnown bool
	HardwareCharacteristics  *instance.HardwareCharacteristics `json:",omitempty"`
	Jobs                     []MachineJob
	Addresses                []network.Address
	HasVote                  bool
	WantsVote                bool
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
// used by ServiceInfo and UnitInfo.
type StatusInfo struct {
	Err     error
	Current Status
	Message string
	Since   *time.Time
	Version string
	Data    map[string]interface{}
}

// ServiceInfo holds the information about a service that is tracked
// by multiwatcherStore.
type ServiceInfo struct {
	ModelUUID   string
	Name        string
	Exposed     bool
	CharmURL    string
	OwnerTag    string
	Life        Life
	MinUnits    int
	Constraints constraints.Value
	Config      map[string]interface{}
	Subordinate bool
	Status      StatusInfo
}

// EntityId returns a unique identifier for a service across
// models.
func (i *ServiceInfo) EntityId() EntityId {
	return EntityId{
		Kind:      "service",
		ModelUUID: i.ModelUUID,
		Id:        i.Name,
	}
}

// UnitInfo holds the information about a unit
// that is tracked by multiwatcherStore.
type UnitInfo struct {
	ModelUUID      string
	Name           string
	Service        string
	Series         string
	CharmURL       string
	PublicAddress  string
	PrivateAddress string
	MachineId      string
	Ports          []network.Port
	PortRanges     []network.PortRange
	Subordinate    bool
	// The following 3 status values are deprecated.
	Status     Status
	StatusInfo string
	StatusData map[string]interface{}
	// Workload and agent state are modelled separately.
	WorkloadStatus StatusInfo
	AgentStatus    StatusInfo
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
	ModelUUID  string
	Id         string
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
	ModelUUID string
	Key       string
	Id        int
	Endpoints []Endpoint
}

// Endpoint holds a service-relation pair.
type Endpoint struct {
	ServiceName string
	Relation    charm.Relation
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
	ModelUUID   string
	Tag         string
	Annotations map[string]string
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
	JobHostUnits        MachineJob = "JobHostUnits"
	JobManageModel      MachineJob = "JobManageModel"
	JobManageNetworking MachineJob = "JobManageNetworking"
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
	ModelUUID string
	Id        string
	Type      BlockType
	Message   string
	Tag       string
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
	ModelUUID  string
	Name       string
	Life       Life
	Owner      string
	ServerUUID string
}

// EntityId returns a unique identifier for an model.
func (i *ModelInfo) EntityId() EntityId {
	return EntityId{
		Kind:      "model",
		ModelUUID: i.ModelUUID,
		Id:        i.ModelUUID,
	}
}
