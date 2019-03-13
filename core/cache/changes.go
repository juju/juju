// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
)

// ModelChange represents either a new model, or a change
// to an existing model.
type ModelChange struct {
	ModelUUID string
	Name      string
	Life      life.Value
	Owner     string // tag maybe?
	Config    map[string]interface{}
	Status    status.StatusInfo
}

// RemoveModel represents the situation when a model is removed
// from the database.
type RemoveModel struct {
	ModelUUID string
}

// ApplicationChange represents either a new application, or a change
// to an existing application in a model.
type ApplicationChange struct {
	ModelUUID       string
	Name            string
	Exposed         bool
	CharmURL        string
	Life            life.Value
	MinUnits        int
	Constraints     constraints.Value
	Config          map[string]interface{}
	Subordinate     bool
	Status          status.StatusInfo
	WorkloadVersion string
}

// RemoveApplication represents the situation when an application
// is removed from a model in the database.
type RemoveApplication struct {
	ModelUUID string
	Name      string
}

// CharmChange represents either a new charm, or a change
// to an existing charm in a model.
type CharmChange struct {
	ModelUUID    string
	CharmURL     string
	CharmVersion string
	LXDProfiler  lxdprofile.LXDProfiler
}

// RemoveCharm represents the situation when an charm
// is removed from a model in the database.
type RemoveCharm struct {
	ModelUUID string
	CharmURL  string
}

// UnitChange represents either a new unit, or a change
// to an existing unit in a model.
type UnitChange struct {
	ModelUUID      string
	Name           string
	Application    string
	Series         string
	CharmURL       string
	PublicAddress  string
	PrivateAddress string
	MachineId      string
	Ports          []network.Port
	PortRanges     []network.PortRange
	Principal      string
	Subordinate    bool
	WorkloadStatus status.StatusInfo
	AgentStatus    status.StatusInfo
}

// RemoveUnit represents the situation when a unit
// is removed from a model in the database.
type RemoveUnit struct {
	ModelUUID string
	Name      string
}

// MachineChange represents either a new machine, or a change
// to an existing machine in a model.
type MachineChange struct {
	ModelUUID                string
	Id                       string
	InstanceId               string
	AgentStatus              status.StatusInfo
	InstanceStatus           status.StatusInfo
	Life                     life.Value
	Config                   map[string]interface{}
	Series                   string
	SupportedContainers      []instance.ContainerType
	SupportedContainersKnown bool
	HardwareCharacteristics  *instance.HardwareCharacteristics
	Addresses                []network.Address
	HasVote                  bool
	WantsVote                bool
}

// RemoveMachine represents the situation when a machine
// is removed from a model in the database.
type RemoveMachine struct {
	ModelUUID string
	Id        string
}
