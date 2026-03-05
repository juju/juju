// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"time"

	coremachine "github.com/juju/juju/core/machine"
	corestatus "github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	domainlife "github.com/juju/juju/domain/life"
)

// StorageInstanceFilesystemStatus represents the status of a filesystem that
// underpins a StorageInstance in the model. This type exists to represent
// status as there is cyclic imports between the status domain and storage.
//
// TODO (tlm): Remove cyclic imports from status domain. This will be done by
// brining the storage types back into the storage domain.
type StorageInstanceFilesystemStatus struct {
	// Data is the raw JSON information for the status.
	Data map[string]any

	// Message is a human friendly message describing the status of the
	// Filesystem.
	Message string

	// Since is the time at which this status became current.
	Since *time.Time

	// Status is the current status of the Filesystem.
	Status corestatus.Status

	// UUID is the unique identifier for the Filesystem.
	UUID FilesystemUUID
}

// StorageInstanceInfo describes basic information about a StorageInstance in
// the model.
type StorageInstanceInfo struct {
	// FilesystemStatus when not nil represents the status information for the
	// filesystem that underpings this StorageInstance.
	FilesystemStatus *StorageInstanceFilesystemStatus

	// ID is the storage identifier given to the StorageInstance.
	ID string

	// Life is the current life value of the StorageInstance.
	Life domainlife.Life

	// Kind is the type of storage being offered by the StorageInstnace.
	Kind StorageKind

	// Persistent indicates if the StorageInstance life cycle outlives the the
	// unit and machines that it is attached to. Some storage is provisioned
	// directly within a machine. In this case the StorageInstance lifecycle is
	// directly tied to that of the Machine.
	Persistent bool

	// UnitAttachments describes all of the attachments the StorageInstance has
	// onto Units in the model.
	UnitAttachments []StorageInstanceUnitAttachment

	// UnitOwner when set represents the Unit within the model that owns the
	// StorageInstance.
	UnitOwner *StorageInstanceUnitOwner

	// UUID is the unique identifier given to the StorageInstance.
	UUID StorageInstanceUUID

	// VolumeStatus when not nil represents the status information for the
	// volume that underpings this StorageInstance.
	VolumeStatus *StorageInstanceVolumeStatus
}

// StorageInstanceMachineAttachment describes an attachment of a StorageInstance
// onto a Machine in the model. StorageInstances are not directly attached to
// machines. It is via their realised composition of Volumes and Filesystems
// that they become available on a Machine in the model.
type StorageInstanceMachineAttachment struct {
	// MachineName is the name of the Machine in the model that the
	// StorageInstance has been made available on.
	MachineName string

	// MachineUUID is the unique identifier of the Machine in the model that the
	// StorageInstance has been made available on.
	MachineUUID coremachine.UUID
}

// StorageInstanceStatus represents the status information of a StorageInstance.
// This type exists over the status domain type due to cyclic imports. It would
// be expected that this is fixed over time.
type StorageInstanceStatus struct {
}

// StorageInstanceUnitAttachment describes a single attachment of a
// StorageInstance onto a Unit.
type StorageInstanceUnitAttachment struct {
	// Life describes the current lifecycle of the StorageInstanceAttachment on
	// to the Unit.
	Life domainlife.Life

	// Location is the expected location where the StorageInstance can be found
	// for the Charm in the Unit.
	Location string

	// MachineAttachment further describes the attachment of a StorageInstance
	// onto a Unit by also describing the Machine that it is transitively
	// attached through. When this value is nil no Machine attachment exists.
	MachineAttachment *StorageInstanceMachineAttachment

	// UnitName is the name of the Unit in the model that the StorageInstance
	// is attached to.
	UnitName string

	// UnitUUID is the unique identifier of the Unit in the model that the
	// StorageInstance is attached to.
	UnitUUID coreunit.UUID

	// UUID is the unique identifier of the StorageInstanceAttachment.
	UUID StorageAttachmentUUID
}

// StorageInstanceUnitOwner describes the Unit that is considered to own a
// StorageInstance.
type StorageInstanceUnitOwner struct {
	// Name is the name of the Unit in the model that owns the StorageInstance.
	Name string

	// UUID is the unique identifier of the Unit in the model that owns the
	// StorageInstance.
	UUID coreunit.UUID
}

// StorageInstanceVolumeStatus represents the status of a volume that
// underpins a StorageInstance in the model. This type exists to represent
// status as there is cyclic imports between the status domain and storage.
//
// TODO (tlm): Remove cyclic imports from status domain. This will be done by
// bringing the storage types back into the storage domain.
type StorageInstanceVolumeStatus struct {
	// Data is the raw JSON information for the status.
	Data map[string]any

	// Message is a human friendly message describing the status of the
	// Volume.
	Message string

	// Since is the time at which this status became current.
	Since *time.Time

	// Status is the current status of the Volume.
	Status corestatus.Status

	// UUID is the unique identifier for the Volume.
	UUID VolumeUUID
}
