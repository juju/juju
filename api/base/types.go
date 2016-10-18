// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package base

import (
	"time"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/status"
)

// UserModel holds information about a model and the last
// time the model was accessed for a particular user. This is a client
// side structure that translates the owner tag into a user facing string.
type UserModel struct {
	Name           string
	UUID           string
	Owner          string
	LastConnection *time.Time
}

// ModelStatus holds information about the status of a juju model.
type ModelStatus struct {
	UUID               string
	Life               string
	Owner              string
	TotalMachineCount  int
	CoreCount          int
	HostedMachineCount int
	ServiceCount       int
	Machines           []Machine
}

// Machine holds information about a machine in a juju model.
type Machine struct {
	Id         string
	InstanceId string
	HasVote    bool
	WantsVote  bool
	Status     string
	Hardware   *instance.HardwareCharacteristics
}

// ModelInfo holds information about a model.
type ModelInfo struct {
	Name            string
	UUID            string
	ControllerUUID  string
	ProviderType    string
	DefaultSeries   string
	Cloud           string
	CloudRegion     string
	CloudCredential string
	Owner           string
	Life            string
	Status          Status
	Users           []UserInfo
	Machines        []Machine
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
