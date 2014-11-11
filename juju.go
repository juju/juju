// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package juju

import (
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"gopkg.in/juju/charm.v4"
)

// temporary types to break the state <> apiserver dependency loop

// RelationUnitsChange holds notifications of units entering and leaving the
// scope of a RelationUnit, and changes to the settings of those units known
// to have entered.
//
// When remote units first enter scope and then when their settings
// change, the changes will be noted in the Changed field, which holds
// the unit settings for every such unit, indexed by the unit id.
//
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

// MachineInfo holds the information about a Machine
// that is watched by StateWatcher.
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
