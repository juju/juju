// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

// CascadedUnitLives contains identifiers for entities that were ensured to be
// "dying" along with a unit. It is intended to inform the service layer which
// entities should have removal jobs scheduled for them.
type CascadedUnitLives struct {
	// MachineUUID if not nil indicates that the unit's
	// machine is no longer alive.
	MachineUUID *string

	// StorageAttachmentUUIDs identify any of the unit's storage
	// attachments that are not longer alive.
	StorageAttachmentUUIDs []string

	// StorageInstanceUUIDs identify any of the unit's storage
	// instances that are not longer alive.
	StorageInstanceUUIDs []string
}

// IsEmpty returns true if the struct value indicates that no associated
// entites were ensured to be "dying" along with a unit.
func (c CascadedUnitLives) IsEmpty() bool {
	return c.MachineUUID == nil &&
		len(c.StorageAttachmentUUIDs) == 0 &&
		len(c.StorageInstanceUUIDs) == 0
}

// CascadedMachineLives contains identifiers for entities that were ensured to
// be "dying" along with a machine. It is intended to inform the service layer
// which entities should have removal jobs scheduled for them.
type CascadedMachineLives struct {
	// MachineUUIDs identify containers on the machine,
	// who's life advanced to dying as well.
	MachineUUIDs []string

	// UnitUUIDs identify units on the machine or its conainers that were alive,
	// but are now dying as a result of the machine's pending removal
	UnitUUIDs []string

	// StorageAttachmentUUIDs identify storage attachments for units dying along
	// with the machine, that are were alive, but are now dying.
	StorageAttachmentUUIDs []string

	// StorageInstanceUUIDs contain machine-scoped storage instances that have
	// had a life advancement along with the machine.
	// Note that we do not invoke storage instance destruction along with unit
	// removals for a machine. These will be the machine's local storage
	// instances.
	StorageInstanceUUIDs []string

	// FileSystemUUIDs identify any file-systems that transitioned
	// to dying with the machine.
	FileSystemUUIDs []string

	// FileSystemAttachmentUUIDs identify any file-system attachments that
	// transitioned to dying with the machine.
	FileSystemAttachmentUUIDs []string

	// VolumeUUIDs identify any volumes that transitioned
	// to dying with the machine.
	VolumeUUIDs []string

	// VolumeAttachmentUUIDs identify any volume attachments that transitioned
	// to dying with the machine.
	VolumeAttachmentUUIDs []string
}

// IsEmpty returns true if the struct value indicates that no associated
// entites were ensured to be "dying" along with a machine.
func (c CascadedMachineLives) IsEmpty() bool {
	return len(c.MachineUUIDs) == 0 &&
		len(c.UnitUUIDs) == 0 &&
		len(c.StorageAttachmentUUIDs) == 0 &&
		len(c.StorageInstanceUUIDs) == 0 &&
		len(c.FileSystemUUIDs) == 0 &&
		len(c.FileSystemAttachmentUUIDs) == 0 &&
		len(c.VolumeUUIDs) == 0 &&
		len(c.VolumeAttachmentUUIDs) == 0
}

// CascadedApplicationLives contains identifiers for entities that were ensured
// to be "dying" along with an application. It is intended to inform the service
// layer which entities should have removal jobs scheduled for them.
type CascadedApplicationLives struct {
	// MachineUUIDs identify machines that advanced to dying as a result of
	// the application's units being the last on the machine.
	MachineUUIDs []string

	// UnitUUIDs identify the app's units that became dying along with it.
	UnitUUIDs []string

	// RelationUUIDs identify relations that this application was participating
	// in that advanced to dying along with it.
	RelationUUIDs []string

	// StorageAttachmentUUIDs identify storage attachments for units dying along
	// with the application, that are were alive, but are now dying.
	StorageAttachmentUUIDs []string
}

// IsEmpty returns true if the struct value indicates that no associated
// entites were ensured to be "dying" along with an application.
func (c CascadedApplicationLives) IsEmpty() bool {
	return len(c.MachineUUIDs) == 0 &&
		len(c.UnitUUIDs) == 0 &&
		len(c.RelationUUIDs) == 0 &&
		len(c.StorageAttachmentUUIDs) == 0
}

// CascadedRemoteApplicationOffererLives contains identifiers for entities that
// were ensured to be "dying" along with a remote application offerer. It is
// intended to inform the service layer which entities should have removal jobs
// scheduled for them
type CascadedRemoteApplicationOffererLives struct {
	// RelationUUIDs identify relations that this application was participating
	// in that advanced to dying along with it.
	RelationUUIDs []string
}

func (c CascadedRemoteApplicationOffererLives) IsEmpty() bool {
	return len(c.RelationUUIDs) == 0
}

// CascadedStorageInstanceLives contains identifiers for entities that were
// ensured to be "dying" along with a storage instance. It is intended to
// inform the service layer which entities should have removal jobs scheduled
// for them.
// Note that there is no IsEmpty method for this struct; an instance always
// has an attachment to something.
type CascadedStorageInstanceLives struct {
	// FileSystemUUID identifes a file-system that the storage instance was
	// attached to. They are removed if provisioned with "machine" scope
	// and have no other storage instance attachments.
	FileSystemUUID *string

	// FileSystemAttachmentUUID identifies the actual attachment of the storage
	// instance to a file-system. It can be removed even when the attached
	// file-system is not.
	FileSystemAttachmentUUID *string

	// VolumeUUID identifes a volume that the storage instance was attached to.
	// They are removed if provisioned with "machine" scope and have no
	// other storage instance attachments.
	VolumeUUID *string

	// VolumeAttachmentUUID identifies the actual attachment of the storage
	// instance to a volume. It can be removed even when the attached
	// volume is not.
	VolumeAttachmentUUID *string
}
