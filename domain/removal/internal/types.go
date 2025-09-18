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
