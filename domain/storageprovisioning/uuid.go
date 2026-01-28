// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioning

import (
	"github.com/juju/juju/internal/errors"
	internaluuid "github.com/juju/juju/internal/uuid"
)

// FilesystemAttachmentUUID represents the unique id for a storage filesystem
// attachment in the model.
type FilesystemAttachmentUUID uuid

// FilesystemUUID represents the unique id for a storage filesystem
// in the model.
type FilesystemUUID uuid

// VolumeAttachmentPlanUUID represents the unique id for a storage volume
// attachment plan in the model.
type VolumeAttachmentPlanUUID uuid

type uuid string

// NewFileystemAttachmentUUID creates a new, valid storage filesystem attachment
// identifier.
func NewFilesystemAttachmentUUID() (FilesystemAttachmentUUID, error) {
	u, err := newUUID()
	return FilesystemAttachmentUUID(u), err
}

// NewFilesystemUUID creates a new, valid storage filesystem identifier.
func NewFilesystemUUID() (FilesystemUUID, error) {
	u, err := newUUID()
	return FilesystemUUID(u), err
}

// NewVolumeAttachmentPlanUUID creates a new, valid volume attachment plan
// identifier.
func NewVolumeAttachmentPlanUUID() (VolumeAttachmentPlanUUID, error) {
	u, err := newUUID()
	return VolumeAttachmentPlanUUID(u), err
}

// newUUID creates a new UUID using the internal uuid package.
func newUUID() (uuid, error) {
	id, err := internaluuid.NewUUID()
	if err != nil {
		return "", errors.Capture(err)
	}
	return uuid(id.String()), nil
}

// String returns the string representation of this uuid. This function
// satisfies the [fmt.Stringer] interface.
func (u FilesystemAttachmentUUID) String() string {
	return uuid(u).String()
}

// String returns the string representation of this uuid. This function
// satisfies the [fmt.Stringer] interface.
func (u FilesystemUUID) String() string {
	return uuid(u).String()
}

// String returns the string representation of this uuid. This function
// satisfies the [fmt.Stringer] interface.
func (u VolumeAttachmentPlanUUID) String() string {
	return uuid(u).String()
}

// String returns the string representation of this uuid. This function
// satisfies the [fmt.Stringer] interface.
func (u uuid) String() string {
	return string(u)
}

// Validate returns an error if the [FilesystemAttachmentUUID] is not valid.
func (u FilesystemAttachmentUUID) Validate() error {
	return uuid(u).validate()
}

// Validate returns an error if the [FilesystemUUID] is not valid.
func (u FilesystemUUID) Validate() error {
	return uuid(u).validate()
}

// Validate returns an error if the [VolumeAttachmentUUID] is not valid.
func (u VolumeAttachmentPlanUUID) Validate() error {
	return uuid(u).validate()
}

// validate checks that [uuid] is a valid uuid returning an error if it is not.
func (u uuid) validate() error {
	if u == "" {
		return errors.New("empty uuid")
	}
	internaluuid.IsValidUUIDString(u.String())
	if !internaluuid.IsValidUUIDString(string(u)) {
		return errors.Errorf("invalid uuid %q", u)
	}
	return nil
}
