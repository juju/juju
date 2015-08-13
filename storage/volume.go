// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import "github.com/juju/names"

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
	Machine names.MachineTag

	VolumeAttachmentInfo
}

// VolumeAttachmentInfo describes machine-specific volume attachment
// information, including how the volume is exposed on the machine.
type VolumeAttachmentInfo struct {
	// DeviceName is the volume's OS-specific device name (e.g. "sdb").
	//
	// If the device name may change (e.g. on machine restart), then this
	// field must be left blank.
	DeviceName string

	// BusAddress is the bus address, where the volume is attached to
	// the machine.
	//
	// The format of this field must match the field of the same name
	// in BlockDevice.
	BusAddress string

	// ReadOnly signifies whether the volume is read only or writable.
	ReadOnly bool
}
