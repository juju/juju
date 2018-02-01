// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import "gopkg.in/juju/names.v2"

type DeviceType string

var (
	DeviceTypeLocal DeviceType = "local"
	DeviceTypeISCSI DeviceType = "iscsi"
)

// Volume identifies and describes a volume (disk, logical volume, etc.)
type Volume struct {
	// Name is a unique name assigned by Juju to the volume.
	Tag names.VolumeTag

	VolumeInfo
}

// VolumeInfo describes a volume (disk, logical volume etc.)
type VolumeInfo struct {
	// VolumeId is a unique provider-supplied ID for the volume.
	// VolumeId is required to be unique for the lifetime of the
	// volume, but may be reused.
	VolumeId string

	// HardwareId is the volume's hardware ID. Not all volumes have
	// a hardware ID, so this may be left blank.
	HardwareId string

	// WWN is the volume's World Wide Name (WWN). Not all volumes
	// have a WWN, so this may be left blank.
	WWN string

	// Size is the size of the volume, in MiB.
	Size uint64

	// Persistent reflects whether the volume is destroyed with the
	// machine to which it is attached.
	Persistent bool
}

// VolumeAttachment identifies and describes machine-specific volume
// attachment information, including how the volume is exposed on the
// machine.
type VolumeAttachment struct {
	// Volume is the unique tag assigned by Juju for the volume
	// that this attachment corresponds to.
	Volume names.VolumeTag

	// Machine is the unique tag assigned by Juju for the machine that
	// this attachment corresponds to.
	Machine names.Tag

	VolumeAttachmentInfo
}

type VolumeAttachmentPlan struct {
	// Volume is the unique tag assigned by Juju for the volume
	// that this attachment corresponds to.
	Volume names.VolumeTag

	// Machine is the unique tag assigned by Juju for the machine that
	// this attachment corresponds to.
	Machine names.MachineTag

	VolumeAttachmentPlanInfo
}

type VolumeAttachmentPlanInfo struct {
	// DeviceType describes what type of volume we are dealing with
	// possible options are:
	// * local - a block device that is directly attached to this instance
	// * iscsi - an iSCSI disk. This type of disk will require the machine agent
	// to execute additional steps before the device is available
	DeviceType DeviceType
	// DeviceAttributes is a map that contains DeviceType specific initialization
	// values. For example, in the case of iscsi, it may contain server address:port,
	// target, chap secrets, etc.
	DeviceAttributes map[string]string
}

// VolumeAttachmentInfo describes machine-specific volume attachment
// information, including how the volume is exposed on the machine.
type VolumeAttachmentInfo struct {
	// DeviceName is the volume's OS-specific device name (e.g. "sdb").
	//
	// If the device name may change (e.g. on machine restart), then this
	// field must be left blank.
	DeviceName string

	// DeviceLink is an OS-specific device link that must exactly match
	// one of the block device's links when attached.
	//
	// If no device link is known, or it may change (e.g. on machine
	// restart), then this field must be left blank.
	DeviceLink string

	// BusAddress is the bus address, where the volume is attached to
	// the machine.
	//
	// The format of this field must match the field of the same name
	// in BlockDevice.
	BusAddress string

	// ReadOnly signifies whether the volume is read only or writable.
	ReadOnly bool

	// PlanInfo holds information that the machine agent might use to
	// initialize the block device that has been attached to it.
	PlanInfo *VolumeAttachmentPlanInfo
}
