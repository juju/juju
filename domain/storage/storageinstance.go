package storage

import (
	coremachine "github.com/juju/juju/core/machine"
	coreunit "github.com/juju/juju/core/unit"
	domainlife "github.com/juju/juju/domain/life"
)

// StorageInstanceInfo describes basic information about a StorageInstance in
// the model.
type StorageInstanceInfo struct {
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

// StorageInstanceUnitAttachment describes a single attachment of a
// StorageInstance onto a Unit.
type StorageInstanceUnitAttachment struct {
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
