// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

// FilesystemAttachmentUUID represents the unique id for a storage
// FilesystemAttachment.
type FilesystemAttachmentUUID baseUUID

// NewFilesystemAttachmentUUID creates a new, valid storage FilesystemAttachment
// identifier.
func NewFilesystemAttachmentUUID() (FilesystemAttachmentUUID, error) {
	u, err := newUUID()
	return FilesystemAttachmentUUID(u), err
}

// String returns the string representation of this [FilesystemAttachmentUUID].
// This function satisfies the [fmt.Stringer] interface.
func (u FilesystemAttachmentUUID) String() string {
	return baseUUID(u).String()
}

// Validate returns an error if the [FilesystemAttachmentUUID] is not valid.
func (u FilesystemAttachmentUUID) Validate() error {
	return baseUUID(u).validate()
}
