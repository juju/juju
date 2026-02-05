// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

// VolumeAttachmentPlanUUID represents the unique id for a storage
// VolumeAttachmentPlan.
type VolumeAttachmentPlanUUID baseUUID

// VolumeAttachmentUUID represents the unique id for a storage VolumeAttachment.
type VolumeAttachmentUUID baseUUID

// VolumeDeviceType defines what device a volume is indicating the method by
// which it is attached to an entity.
type VolumeDeviceType int

// VolumeUUID represents the unique id for a storage volume instance.
type VolumeUUID baseUUID

const (
	// VolumeDeviceTypeLocal indicates a local attachment.
	VolumeDeviceTypeLocal VolumeDeviceType = iota

	// VolumeDeviceTypeISCSI indicates an ISCSI attachment.
	VolumeDeviceTypeISCSI
)

// NewVolumeAttachmentPlanUUID creates a new, valid storage VolumeAttachmentPlan
// identifier.
func NewVolumeAttachmentPlanUUID() (VolumeAttachmentPlanUUID, error) {
	u, err := newUUID()
	return VolumeAttachmentPlanUUID(u), err
}

// NewVolumeAttachmentUUID creates a new, valid storage VolumeAttachment
// identifier.
func NewVolumeAttachmentUUID() (VolumeAttachmentUUID, error) {
	u, err := newUUID()
	return VolumeAttachmentUUID(u), err
}

// NewVolumeUUID creates a new, valid storage volume identifier.
func NewVolumeUUID() (VolumeUUID, error) {
	u, err := newUUID()
	return VolumeUUID(u), err
}

// String returns the string representation of this [VolumeAttachmentPlanUUID].
// This function satisfies the [fmt.Stringer] interface.
func (u VolumeAttachmentPlanUUID) String() string {
	return baseUUID(u).String()
}

// String returns the string representation of this [VolumeAttachmentUUID].
// This function satisfies the [fmt.Stringer] interface.
func (u VolumeAttachmentUUID) String() string {
	return baseUUID(u).String()
}

// String returns the string representation of [VolumeDeviceType].
// This value is guaranteed to line up with the constants defined for
// [github.com/juju/juju/internal/storage.DeviceType].
//
// If the value of [VolumeDeviceType] is not known a zero value
// string will be returned.
func (v VolumeDeviceType) String() string {
	switch v {
	case VolumeDeviceTypeLocal:
		return "local"
	case VolumeDeviceTypeISCSI:
		return "iscsi"
	default:
		return ""
	}
}

// String returns the string representation of this [VolumeUUID]. This function
// satisfies the [fmt.Stringer] interface.
func (u VolumeUUID) String() string {
	return baseUUID(u).String()
}

// Validate returns an error if the [VolumeAttachmentPlanUUID] is not valid.
func (u VolumeAttachmentPlanUUID) Validate() error {
	return baseUUID(u).validate()
}

// Validate returns an error if the [VolumeAttachmentUUID] is not valid.
func (u VolumeAttachmentUUID) Validate() error {
	return baseUUID(u).validate()
}

// Validate returns an error if the [VolumeUUID] is not valid.
func (u VolumeUUID) Validate() error {
	return baseUUID(u).validate()
}
