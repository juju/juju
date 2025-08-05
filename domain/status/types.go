// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/constraints"
	"github.com/juju/juju/domain/deployment"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/storage"
)

// Application represents the status of an application.
type Application struct {
	ID              application.ID
	Life            life.Life
	Status          StatusInfo[WorkloadStatusType]
	Units           map[unit.Name]Unit
	Relations       []relation.UUID
	Subordinate     bool
	CharmLocator    charm.CharmLocator
	CharmVersion    string
	LXDProfile      []byte
	Platform        deployment.Platform
	Channel         *deployment.Channel
	Exposed         bool
	Scale           *int
	WorkloadVersion *string
	K8sProviderID   *string
}

// Unit represents the status of a unit.
type Unit struct {
	ApplicationName  string
	CharmLocator     charm.CharmLocator
	MachineName      *machine.Name
	AgentStatus      StatusInfo[UnitAgentStatusType]
	WorkloadStatus   StatusInfo[WorkloadStatusType]
	K8sPodStatus     StatusInfo[K8sPodStatusType]
	Life             life.Life
	Subordinate      bool
	PrincipalName    *unit.Name
	SubordinateNames map[unit.Name]struct{}
	Present          bool
	AgentVersion     string
	WorkloadVersion  *string
	K8sProviderID    *string
}

// Machine represents the status of a machine.
type Machine struct {
	UUID                    machine.UUID
	Hostname                string
	DisplayName             string
	DNSName                 string
	IPAddresses             []string
	InstanceID              instance.Id
	Life                    life.Life
	MachineStatus           StatusInfo[MachineStatusType]
	InstanceStatus          StatusInfo[InstanceStatusType]
	Platform                deployment.Platform
	Constraints             constraints.Constraints
	HardwareCharacteristics instance.HardwareCharacteristics
	LXDProfiles             []string
}

type StorageInstance struct {
	ID          string
	Owner       *unit.Name
	Kind        storage.StorageKind
	Life        life.Life
	Attachments map[unit.Name]StorageAttachment
}

type StorageAttachment struct {
	Life     life.Life
	Unit     *unit.Name
	Machine  *machine.Name
	Location string
}

type Filesystem struct {
	ID         string
	Life       life.Life
	Status     StatusInfo[StorageFilesystemStatusType]
	StorageID  string
	VolumeID   *string
	ProviderID string
	SizeMiB    uint64
}

type Volume struct {
	ID         string
	Life       life.Life
	Status     StatusInfo[StorageVolumeStatusType]
	StorageID  string
	ProviderID string
	HardwareID string
	WWN        string
	SizeMiB    uint64
	Persistent bool
}

type FilesystemAttachment struct {
	Life       life.Life
	MountPoint string
	ReadOnly   bool
}

type VolumeAttachment struct {
	Life                 life.Life
	DeviceName           string
	DeviceLink           string
	BusAddress           string
	ReadOnly             bool
	VolumeAttachmentPlan *VolumeAttachmentPlan
}

type VolumeAttachmentPlan struct {
	DeviceType       string
	DeviceAttributes map[string]string
}
