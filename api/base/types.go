// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package base

import (
	"time"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/status"
)

// UserModel holds information about a model and the last
// time the model was accessed for a particular user. This is a client
// side structure that translates the owner tag into a user facing string.
type UserModel struct {
	Name           string
	UUID           string
	Type           model.ModelType
	Owner          string
	LastConnection *time.Time
}

// ModelStatus holds information about the status of a juju model.
type ModelStatus struct {
	UUID               string
	Life               life.Value
	ModelType          model.ModelType
	Owner              string
	TotalMachineCount  int
	CoreCount          int
	HostedMachineCount int
	ApplicationCount   int
	UnitCount          int
	Applications       []Application
	Machines           []Machine
	Volumes            []Volume
	Filesystems        []Filesystem
	Error              error
}

// Machine holds information about a machine in a juju model.
type Machine struct {
	Id          string
	InstanceId  string
	DisplayName string
	Status      string
	Message     string
	Hardware    *instance.HardwareCharacteristics
}

// Application holds information about an application in a juju model.
type Application struct {
	Name string
}

// ModelInfo holds information about a model.
type ModelInfo struct {
	Name            string
	UUID            string
	Type            model.ModelType
	ControllerUUID  string
	IsController    bool
	ProviderType    string
	Cloud           string
	CloudRegion     string
	CloudCredential string
	Owner           string
	Life            life.Value
	Status          Status
	Users           []UserInfo
	Machines        []Machine
	AgentVersion    *semversion.Number
}

// Status represents the status of a machine, application, or unit.
type Status struct {
	Status status.Status
	Info   string
	Data   map[string]interface{}
	Since  *time.Time
}

// UserInfo holds information about a user in a juju model.
type UserInfo struct {
	UserName       string
	DisplayName    string
	LastConnection *time.Time
	Access         string
}

// Volume holds information about a volume in a juju model.
type Volume struct {
	Id         string
	ProviderId string
	Status     string
	Message    string
	Detachable bool
}

// Filesystem holds information about a filesystem in a juju model.
type Filesystem struct {
	Id         string
	ProviderId string
	Status     string
	Message    string
	Detachable bool
}

// UserModelSummary holds summary about a model for a user.
type UserModelSummary struct {
	Name               string
	UUID               string
	Type               model.ModelType
	ControllerUUID     string
	IsController       bool
	ProviderType       string
	Cloud              string
	CloudRegion        string
	CloudCredential    string
	Owner              string
	Life               life.Value
	Status             Status
	ModelUserAccess    string
	UserLastConnection *time.Time
	Counts             []EntityCount
	AgentVersion       *semversion.Number
	Error              error
	Migration          *MigrationSummary
}

// EntityCount holds a count for a particular entity, for example machines or core count.
type EntityCount struct {
	Entity string
	Count  int64
}

// MigrationSummary holds information about a current migration attempt
// if there is one on progress.
type MigrationSummary struct {
	Status    string
	StartTime *time.Time
	EndTime   *time.Time
}

// StoredCredential contains information about the cloud credential stored on the controller
// and used by models.
type StoredCredential struct {
	// CloudCredential is a cloud credential id that identifies cloud credential on the controller.
	// The value is what CloudCredentialTag.Id() returns.
	CloudCredential string

	// Valid is a flag that indicates whether the credential is valid.
	Valid bool
}
