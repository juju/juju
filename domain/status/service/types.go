// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"time"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/deployment"
	internalcharm "github.com/juju/juju/internal/charm"
	internalstorage "github.com/juju/juju/internal/storage"
)

// Application represents the status of an application.
type Application struct {
	Life            life.Value
	Status          status.StatusInfo
	Relations       []relation.UUID
	Subordinate     bool
	CharmLocator    charm.CharmLocator
	CharmVersion    string
	Platform        deployment.Platform
	Channel         *deployment.Channel
	Exposed         bool
	LXDProfile      *internalcharm.LXDProfile
	Scale           *int
	WorkloadVersion *string
	K8sProviderID   *string
	Units           map[unit.Name]Unit
}

// Unit represents the status of a unit.
type Unit struct {
	Life             life.Value
	ApplicationName  string
	MachineName      *machine.Name
	AgentStatus      status.StatusInfo
	WorkloadStatus   status.StatusInfo
	K8sPodStatus     status.StatusInfo
	Present          bool
	Subordinate      bool
	PrincipalName    *unit.Name
	SubordinateNames []unit.Name
	CharmLocator     charm.CharmLocator
	AgentVersion     string
	WorkloadVersion  *string
	K8sProviderID    *string
}

// Machine represents the status of a machine.
type Machine struct {
	Name                    machine.Name
	Hostname                string
	DisplayName             string
	DNSName                 string
	IPAddresses             []string
	InstanceID              instance.Id
	Life                    life.Value
	MachineStatus           status.StatusInfo
	InstanceStatus          status.StatusInfo
	Platform                deployment.Platform
	Constraints             constraints.Value
	HardwareCharacteristics instance.HardwareCharacteristics
	LXDProfiles             []string
}

// StatusHistoryFilter holds the parameters to filter a status history query.
type StatusHistoryFilter struct {
	Size  int
	Date  *time.Time
	Delta *time.Duration
}

// StatusHistoryRequest holds the parameters to filter a status history query.
type StatusHistoryRequest struct {
	Kind   status.HistoryKind
	Filter StatusHistoryFilter
	Tag    string
}

type StorageInstance struct {
	ID          string
	Owner       *unit.Name
	Kind        internalstorage.StorageKind
	Life        life.Value
	Attachments map[unit.Name]StorageAttachment
}

type StorageAttachment struct {
	Life     life.Value
	Unit     *unit.Name
	Machine  *machine.Name
	Location string
}

type Filesystem struct {
	ID                 string
	Life               life.Value
	Status             status.StatusInfo
	StorageID          string
	VolumeID           *string
	ProviderID         string
	SizeMiB            uint64
	MachineAttachments map[machine.Name]FilesystemAttachment
	UnitAttachments    map[unit.Name]FilesystemAttachment
}

type Volume struct {
	ID                 string
	Life               life.Value
	Status             status.StatusInfo
	StorageID          string
	ProviderID         string
	HardwareID         string
	WWN                string
	SizeMiB            uint64
	Persistent         bool
	MachineAttachments map[machine.Name]VolumeAttachment
	UnitAttachments    map[unit.Name]VolumeAttachment
}

type FilesystemAttachment struct {
	Life       life.Value
	MountPoint string
	ReadOnly   bool
}

type VolumeAttachment struct {
	Life                 life.Value
	DeviceName           string
	DeviceLink           string
	BusAddress           string
	ReadOnly             bool
	VolumeAttachmentPlan *VolumeAttachmentPlan
}

type VolumeAttachmentPlan struct {
	DeviceType       internalstorage.DeviceType
	DeviceAttributes map[string]string
}
