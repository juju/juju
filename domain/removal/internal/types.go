// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

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

	// VolumeAttachmentPlanUUID identifies the plan for actioning a volume
	// attachment that is run on the actual attached machine.
	VolumeAttachmentPlanUUID *string
}

// CascadedStorageLives identifies all storage entities that transitioned to
// "dying" as part of another entity removal.
type CascadedStorageLives struct {
	// StorageAttachmentUUIDs identify storage attachments for dying units
	// that were alive, but are now also dying.
	StorageAttachmentUUIDs []string

	// StorageInstanceUUIDs contain machine-scoped storage instances that have
	// had a life advancement along with the machine/app/unit.
	// Note that we do not invoke storage instance destruction along with unit
	// removals for a machine. These will be the machine's local storage
	// instances.
	StorageInstanceUUIDs []string

	// FileSystemUUIDs identify any file-systems that transitioned
	// to dying with the machine/app/unit.
	FileSystemUUIDs []string

	// FileSystemAttachmentUUIDs identify any file-system attachments that
	// transitioned to dying with the machine/app/unit.
	FileSystemAttachmentUUIDs []string

	// VolumeUUIDs identify any volumes that transitioned
	// to dying with the machine/app/unit.
	VolumeUUIDs []string

	// VolumeAttachmentUUIDs identify any volume attachments that transitioned
	// to dying with the machine/app/unit.
	VolumeAttachmentUUIDs []string

	// VolumeAttachmentUUIDs identify any volume attachments that transitioned
	// to dying with the machine/app/unit.
	VolumeAttachmentPlanUUIDs []string
}

// MergeInstance merges the result of cascading the destruction of a single
// storage instance into this value and returns the result.
func (c CascadedStorageLives) MergeInstance(i CascadedStorageInstanceLives) CascadedStorageLives {
	if i.FileSystemAttachmentUUID != nil {
		c.FileSystemAttachmentUUIDs = append(c.FileSystemAttachmentUUIDs, *i.FileSystemAttachmentUUID)
	}

	if i.VolumeAttachmentUUID != nil {
		c.VolumeAttachmentUUIDs = append(c.VolumeAttachmentUUIDs, *i.VolumeAttachmentUUID)
	}

	if i.VolumeAttachmentPlanUUID != nil {
		c.VolumeAttachmentPlanUUIDs = append(c.VolumeAttachmentPlanUUIDs, *i.VolumeAttachmentPlanUUID)
	}

	if i.FileSystemUUID != nil {
		c.FileSystemUUIDs = append(c.FileSystemUUIDs, *i.FileSystemUUID)
	}

	if i.VolumeUUID != nil {
		c.VolumeUUIDs = append(c.VolumeUUIDs, *i.VolumeUUID)
	}

	return c
}

// IsEmpty returns true if the struct value indicates that no associated
// storage entites were ensured to be "dying" along with an entity.
func (c CascadedStorageLives) IsEmpty() bool {
	return len(c.StorageAttachmentUUIDs) == 0 &&
		len(c.StorageInstanceUUIDs) == 0 &&
		len(c.FileSystemUUIDs) == 0 &&
		len(c.FileSystemAttachmentUUIDs) == 0 &&
		len(c.VolumeUUIDs) == 0 &&
		len(c.VolumeAttachmentUUIDs) == 0 &&
		len(c.VolumeAttachmentPlanUUIDs) == 0
}

// CascadedUnitLives contains identifiers for entities that were ensured to be
// "dying" along with a unit. It is intended to inform the service layer which
// entities should have removal jobs scheduled for them.
type CascadedUnitLives struct {
	CascadedStorageLives

	// MachineUUID if not nil indicates that the unit's
	// machine is no longer alive.
	MachineUUID *string
}

// IsEmpty returns true if the struct value indicates that no associated
// entites were ensured to be "dying" along with a unit.
func (c CascadedUnitLives) IsEmpty() bool {
	return c.MachineUUID == nil && c.CascadedStorageLives.IsEmpty()
}

// CascadedApplicationLives contains identifiers for entities that were ensured
// to be "dying" along with an application. It is intended to inform the service
// layer which entities should have removal jobs scheduled for them.
type CascadedApplicationLives struct {
	CascadedStorageLives

	// MachineUUIDs identify machines that advanced to dying as a result of
	// the application's units being the last on the machine.
	MachineUUIDs []string

	// UnitUUIDs identify the app's units that became dying along with it.
	UnitUUIDs []string

	// RelationUUIDs identify relations that this application was participating
	// in that advanced to dying along with it.
	RelationUUIDs []string
}

// IsEmpty returns true if the struct value indicates that no associated
// entites were ensured to be "dying" along with an application.
func (c CascadedApplicationLives) IsEmpty() bool {
	return c.CascadedStorageLives.IsEmpty() &&
		len(c.MachineUUIDs) == 0 &&
		len(c.UnitUUIDs) == 0 &&
		len(c.RelationUUIDs) == 0
}

// CascadedUnitLives contains identifiers for entities that were ensured to be
// "dying" along with a machine. It is intended to inform the service layer
// which entities should have removal jobs scheduled for them.
type CascadedMachineLives struct {
	CascadedStorageLives

	// MachineUUIDs identify containers on the machine,
	// who's life advanced to dying as well.
	MachineUUIDs []string

	// UnitUUIDs identify units on the machine or its conainers that were alive,
	// but are now dying as a result of the machine's pending removal
	UnitUUIDs []string
}

// IsEmpty returns true if the struct value indicates that no associated
// entites were ensured to be "dying" along with a machine.
func (c CascadedMachineLives) IsEmpty() bool {
	return c.CascadedStorageLives.IsEmpty() &&
		len(c.MachineUUIDs) == 0 &&
		len(c.UnitUUIDs) == 0
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

