// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package multiwatcher

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"gopkg.in/juju/charm.v5-unstable"

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

type EntityId struct {
	Kind string
	Id   interface{}
}

// Delta holds details of a change to the environment.
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
	default:
		return fmt.Errorf("Unexpected entity name %q", entityKind)
	}
	return json.Unmarshal(elements[2], &d.Entity)
}

// When remote units leave scope, their ids will be noted in the
// Departed field, and no further events will be sent for those units.
type RelationUnitsChange struct {
	Changed  map[string]UnitSettings
	Departed []string
}

// UnitSettings holds information about a service unit's settings
// within a relation.
type UnitSettings struct {
	Version int64
}

// MachineInfo holds the information about a Machine
// that is watched by StateMultiwatcher.
type MachineInfo struct {
	Id                       string `bson:"_id"`
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

func (i *MachineInfo) EntityId() EntityId {
	return EntityId{
		Kind: "machine",
		Id:   i.Id,
	}
}

type ServiceInfo struct {
	Name        string `bson:"_id"`
	Exposed     bool
	CharmURL    string
	OwnerTag    string
	Life        Life
	MinUnits    int
	Constraints constraints.Value
	Config      map[string]interface{}
	Subordinate bool
}

func (i *ServiceInfo) EntityId() EntityId {
	return EntityId{
		Kind: "service",
		Id:   i.Name,
	}
}

type UnitInfo struct {
	Name           string `bson:"_id"`
	Service        string
	Series         string
	CharmURL       string
	PublicAddress  string
	PrivateAddress string
	MachineId      string
	Ports          []network.Port
	PortRanges     []network.PortRange
	Status         Status
	StatusInfo     string
	StatusData     map[string]interface{}
	Subordinate    bool
}

func (i *UnitInfo) EntityId() EntityId {
	return EntityId{
		Kind: "unit",
		Id:   i.Name,
	}
}

type ActionInfo struct {
	Id         string                 `bson:"_id"`
	Receiver   string                 `bson:"receiver"`
	Name       string                 `bson:"name"`
	Parameters map[string]interface{} `bson:"parameters"`
	Status     string                 `bson:"status"`
	Message    string                 `bson:"message"`
	Results    map[string]interface{} `bson:"results"`
	Enqueued   time.Time              `bson:"enqueued"`
	Started    time.Time              `bson:"started"`
	Completed  time.Time              `bson:"completed"`
}

func (i *ActionInfo) EntityId() EntityId {
	return EntityId{
		Kind: "action",
		Id:   i.Id,
	}
}

type RelationInfo struct {
	Key       string `bson:"_id"`
	Id        int
	Endpoints []Endpoint
}

func (i *RelationInfo) EntityId() EntityId {
	return EntityId{
		Kind: "relation",
		Id:   i.Key,
	}
}

type AnnotationInfo struct {
	Tag         string
	Annotations map[string]string
}

func (i *AnnotationInfo) EntityId() EntityId {
	return EntityId{
		Kind: "annotation",
		Id:   i.Tag,
	}
}

type Endpoint struct {
	ServiceName string
	Relation    charm.Relation
}

// MachineJob values define responsibilities that machines may be
// expected to fulfil.
type MachineJob string

const (
	JobHostUnits        MachineJob = "JobHostUnits"
	JobManageEnviron    MachineJob = "JobManageEnviron"
	JobManageNetworking MachineJob = "JobManageNetworking"

	// Deprecated in 1.18
	JobManageStateDeprecated MachineJob = "JobManageState"
)

// NeedsState returns true if the job requires a state connection.
func (job MachineJob) NeedsState() bool {
	return job == JobManageEnviron
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

// BlockInfo holds the information about blocks
// in this environment that are watched.
type BlockInfo struct {
	Id      string    `bson:"_id"`
	Type    BlockType `bson:"type"`
	Message string    `bson:"message,omitempty"`
	Tag     string    `bson:"tag"`
}

// EntityId returns block id.
func (i *BlockInfo) EntityId() EntityId {
	return EntityId{
		Kind: "block",
		Id:   i.Id,
	}
}

// BlockType values define environment block type.
type BlockType string

const (
	// BlockDestroy type identifies destroy blocks.
	BlockDestroy BlockType = "BlockDestroy"

	// BlockRemove type identifies remove blocks.
	BlockRemove BlockType = "BlockRemove"

	// BlockChange type identifies change blocks.
	BlockChange BlockType = "BlockChange"
)
