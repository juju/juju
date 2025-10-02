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

// CascadedUnitLives contains identifiers for entities that were ensured to be
// "dying" along with a machine. It is intended to inform the service layer
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
}

// IsEmpty returns true if the struct value indicates that no associated
// entites were ensured to be "dying" along with a unit.
func (c CascadedMachineLives) IsEmpty() bool {
	return len(c.MachineUUIDs) == 0 &&
		len(c.UnitUUIDs) == 0 &&
		len(c.StorageAttachmentUUIDs) == 0 &&
		len(c.StorageInstanceUUIDs) == 0
}