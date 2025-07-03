// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioning

// FilesystemAttachmentID is a struct that provides the IDs and names associated
// with a filesystem attachment. In this case the ID refers to the filesystem
// resource the attachment is for. As well as this the name of the machine and
// or the unit the attachment is for is also provided.
//
// As it is unclear if a filesystem attachment is for a unit or a machine either
// one of the name values will be set but not both.
type FilesystemAttachmentID struct {
	// FilesystemID is the ID of the filesystem resource that the attachment is
	// for.
	FilesystemID string

	// MachineName is the name of the machine the filesystem attachment is
	// against. Only one of [FilesystemAttachmentID.MachineName] or
	// [FilesystemAttachmentID.UnitName] will be set. It is reasonable to expect
	// one of these values to be set.
	MachineName *string

	// UnitName is the name of the unit the filesystem attachment is against.
	// Only one of [FilesystemAttachmentID.MachineName] or
	// [FilesystemAttachmentID.UnitName] will be set. It is reasonable to expect
	// one of these values to be set.
	UnitName *string
}
