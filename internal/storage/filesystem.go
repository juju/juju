// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import "github.com/juju/names/v6"

// Filesystem identifies and describes a filesystem, either local or remote
// (NFS, Ceph etc).
type Filesystem struct {
	// Tag is a unique name assigned by Juju to the filesystem.
	Tag names.FilesystemTag

	// Volume is the tag of the volume that backs the filesystem, if any.
	Volume names.VolumeTag

	FilesystemInfo
}

// Filesystem describes a filesystem, either local or remote (NFS, Ceph etc).
type FilesystemInfo struct {
	// ProviderId is a unique provider-supplied ID for the filesystem.
	// ProviderId is required to be unique for the lifetime of the
	// filesystem, but may be reused.
	ProviderId string

	// Size is the size of the filesystem, in MiB.
	Size uint64
}

// FilesystemAttachment describes machine-specific filesystem attachment information,
// including how the filesystem is exposed on the machine.
type FilesystemAttachment struct {
	// Filesystem is the unique tag assigned by Juju for the filesystem
	// that this attachment corresponds to.
	Filesystem names.FilesystemTag

	// Machine is the unique tag assigned by Juju for the machine that
	// this attachment corresponds to.
	Machine names.Tag

	FilesystemAttachmentInfo
}

// FilesystemAttachmentInfo describes machine-specific filesystem attachment
// information, including how the filesystem is exposed on the machine.
type FilesystemAttachmentInfo struct {
	// Path is the path at which the filesystem is mounted on the machine
	// that this attachment corresponds to.
	Path string

	// ReadOnly indicates that the filesystem is mounted read-only.
	ReadOnly bool
}
