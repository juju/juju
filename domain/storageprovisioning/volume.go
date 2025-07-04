// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioning

import (
	coremachine "github.com/juju/juju/core/machine"
	coreunit "github.com/juju/juju/core/unit"
)

// VolumeAttachmentID is a struct that provides the IDs and names associated
// with a volume attachment. In this case the id refers to the volume
// resource the attachment is for. As well as this the name of the machine and
// or the unit the attachment is for is also provided.
//
// As it is unclear if a volume attachment is for a unit or a machine either
// one of the name values will be set but not both.
type VolumeAttachmentID struct {
	// VolumeID is the ID of the volume resource that the attachment is
	// for.
	VolumeID string

	// MachineName is the name of the machine the volume attachment is
	// against. Only one of [VolumeAttachmentID.MachineName] or
	// [VolumeAttachmentID.UnitName] will be set. It is reasonable to expect
	// one of these values to be set.
	MachineName *coremachine.Name

	// UnitName is the name of the unit the volume attachment is against.
	// Only one of [VolumeAttachmentID.MachineName] or
	// [VolumeAttachmentID.UnitName] will be set. It is reasonable to expect
	// one of these values to be set.
	UnitName *coreunit.Name
}
