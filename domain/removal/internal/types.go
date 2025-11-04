// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

// CascadedStorageInstanceLifeChildren contains the filesystem or volume
// indentifiers for a single storage instance that were ensured to be "dying".
type CascadedStorageInstanceLifeChildren struct {
	// FileSystemUUID identifes a file-system that the storage instance was
	// attached to. They are removed if provisioned with "machine" scope
	// and have no other storage instance attachments.
	FileSystemUUID *string

	// VolumeUUID identifes a volume that the storage instance was attached to.
	// They are removed if provisioned with "machine" scope and have no
	// other storage instance attachments.
	VolumeUUID *string
}

// CascadedStorageAttachmentLifeChildren contains the filesystem attachment or
// volume attachment or volume attachment plan indentifiers for a single storage
// attachment that need a removal job scheduled.
type CascadedStorageAttachmentLifeChildren struct {
	// FilesystemAttachmentUUID identifes a filesystem attachment that the
	// storage attachment owns that needs a removal job scheduled.
	FilesystemAttachmentUUID *string

	// VolumeAttachmentUUID identifes a volume attachment that the storage
	// attachment owns that needs a removal job scheduled.
	VolumeAttachmentUUID *string

	// VolumeAttachmentPlanUUID identifes a volume attachment plan that the
	// storage attachment owns that needs a removal job scheduled.
	VolumeAttachmentPlanUUID *string
}

// CascadedStorageInstanceLives contains identifiers for entities that were
// ensured to be "dying" along with storage instances. It is intended to
// inform the service layer which entities should have removal jobs scheduled
// for them.
// Note that there is no IsEmpty method for this struct; an instance always
// has an attachment to something.
type CascadedStorageInstanceLives struct {
	// StorageInstanceUUIDs contain machine-scoped storage instances that have
	// had a life advancement along with the machine/app/unit.
	// Note that we do not invoke storage instance destruction along with unit
	// removals for a machine. These will be the machine's local storage
	// instances.
	StorageInstanceUUIDs []string

	// FileSystemUUIDs identify any file-systems that transitioned
	// to dying with the machine/app/unit.
	FileSystemUUIDs []string

	// VolumeUUIDs identify any volumes that transitioned
	// to dying with the machine/app/unit.
	VolumeUUIDs []string
}

// Merge appends all EntityUUIDs from one CascadedStorageInstanceLives into
// another and returns the result
func (c CascadedStorageInstanceLives) Merge(i CascadedStorageInstanceLives) CascadedStorageInstanceLives {
	c.StorageInstanceUUIDs = append(c.StorageInstanceUUIDs, i.StorageInstanceUUIDs...)
	c.FileSystemUUIDs = append(c.FileSystemUUIDs, i.FileSystemUUIDs...)
	c.VolumeUUIDs = append(c.VolumeUUIDs, i.VolumeUUIDs...)

	return c
}

// CascadedStorageAttachmentLives contains identifiers for entities that will
// need to die along with storage attachments, but are not yet dying. It is
// intended to inform the service layer which entities should have force removal
// jobs scheduled for them.
// Note that there is no IsEmpty method for this struct; an instance always
// has an attachment to something.
type CascadedStorageAttachmentLives struct {
	// StorageAttachmentUUIDs identify storage attachments for dying units
	// that were alive, but are now also dying.
	StorageAttachmentUUIDs []string

	// FileSystemAttachmentUUIDs identify any file-system attachments that
	// will eventually die, but need force removal jobs created.
	FileSystemAttachmentUUIDs []string

	// VolumeAttachmentUUIDs identify any volume attachments that will
	// eventually die, but need force removal jobs created.
	VolumeAttachmentUUIDs []string

	// VolumeAttachmentUUIDs identify any volume attachments that will
	// eventually die, but need force removal jobs created.
	VolumeAttachmentPlanUUIDs []string
}

// CascadedStorageProvisionedAttachmentLives contains identifiers for entities
// that were ensured to be "dying" along with death of a storage instance.
// It is intended to inform the service layer which entities should have removal
// jobs scheduled for them.
// Note that there is no IsEmpty method for this struct; an instance always
// has an attachment to something.
type CascadedStorageProvisionedAttachmentLives struct {
	// FileSystemAttachmentUUIDs identify any file-system attachments that
	// transitioned to dying with the storage attachment.
	FileSystemAttachmentUUIDs []string

	// VolumeAttachmentUUIDs identify any volume attachments that transitioned
	// to dying with the storage attachment.
	VolumeAttachmentUUIDs []string

	// VolumeAttachmentUUIDs identify any volume attachments that transitioned
	// to dying with the storage attachment.
	VolumeAttachmentPlanUUIDs []string
}

// Merge appends all EntityUUIDs from one CascadedStorageAttachmentLives into
// another and returns the result
func (c CascadedStorageAttachmentLives) Merge(i CascadedStorageAttachmentLives) CascadedStorageAttachmentLives {
	c.StorageAttachmentUUIDs = append(c.StorageAttachmentUUIDs, i.StorageAttachmentUUIDs...)
	c.FileSystemAttachmentUUIDs = append(c.FileSystemAttachmentUUIDs, i.FileSystemAttachmentUUIDs...)
	c.VolumeAttachmentUUIDs = append(c.VolumeAttachmentUUIDs, i.VolumeAttachmentUUIDs...)
	c.VolumeAttachmentPlanUUIDs = append(c.VolumeAttachmentPlanUUIDs, i.VolumeAttachmentPlanUUIDs...)

	return c
}

// CascadedStorageLives identifies all storage entities that transitioned to
// "dying" as part of another entity removal.
type CascadedStorageLives struct {
	CascadedStorageInstanceLives
	CascadedStorageAttachmentLives
}

// Merge appends all EntityUUIDs from one CascadedStorageLives into another and
// returns the result
func (c CascadedStorageLives) Merge(i CascadedStorageLives) CascadedStorageLives {
	c.CascadedStorageAttachmentLives = c.CascadedStorageAttachmentLives.Merge(
		i.CascadedStorageAttachmentLives)
	c.CascadedStorageInstanceLives = c.CascadedStorageInstanceLives.Merge(
		i.CascadedStorageInstanceLives)

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

// CascadedMachineLives contains identifiers for entities that were ensured to be
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

// CascadedRemoteRelationLives contains identifiers for entities that need to
// be removed along with the relations. Remote relations is somewhat of a
// special case, since there exist synthetic units (i.e. without a uniter)
// that need to be departed manually.
type CascadedRemoteRelationLives struct {
	// SyntheticRelationUnitUUIDs identify the relation units that need to be
	// departed to remove the relation.
	SyntheticRelationUnitUUIDs []string
}

// StorageAttachmentDetachInfo contains the information required to establish
// if a storage attachment in the model can be detached.
type StorageAttachmentDetachInfo struct {
	// CharmStorageName is the unique name given by the charm for for the
	// storage.
	CharmStorageName string

	// CountFulfilment indicates how many alive storage attachments are
	// currently in play to satisfy the requirements of
	// [StorageAttachmentDetachInfo.RequiredCountMin].
	CountFulfilment int

	// RequiredCountMin indicates the minimum number storage instances that must
	// be attached to this unit to satisfy the charm's requirements.
	RequiredCountMin int

	// Life is the current life value of the storage attachment.
	Life int

	// UnitLife is the current life value of the unit for with the storage
	// is attached to.
	UnitLife int

	// UnitUUID is the UUID of the unit the storage is attached to.
	UnitUUID string
}
