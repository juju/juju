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

// VolumeAttachmentUUID represents the unique id for a storage volume
// attachment in the model.
type VolumeAttachmentUUID uuid

// VolumeUUID represents the unique id for a storage volume instance.
type VolumeUUID uuid

type uuid string

// NewFileystemAttachmentUUID creates a new, valid storage filesystem attachment
// identifier.
func NewFilesystemAttachmentUUID() (FilesystemAttachmentUUID, error) {
	u, err := newUUID()
	return FilesystemAttachmentUUID(u), err
}

// GenFilesystemAttachmentUUID generates a new [FilesystemAttachmentUUID] for
// testing purposes.
func GenFilesystemAttachmentUUID(c interface{ Fatal(...any) }) FilesystemAttachmentUUID {
	uuid, err := NewFilesystemAttachmentUUID()
	if err != nil {
		c.Fatal(err)
	}
	return uuid
}

// NewFileystemUUID creates a new, valid storage filesystem identifier.
func NewFileystemUUID() (FilesystemUUID, error) {
	u, err := newUUID()
	return FilesystemUUID(u), err
}

// GenFilesystemUUID generates a new [FilesystemUUID] for testing purposes.
func GenFilesystemUUID(c interface{ Fatal(...any) }) FilesystemUUID {
	uuid, err := NewFileystemUUID()
	if err != nil {
		c.Fatal(err)
	}
	return uuid
}

// NewVolumeAttachmentUUID creates a new, valid storage volume attachment
// identifier.
func NewVolumeAttachmentUUID() (VolumeAttachmentUUID, error) {
	u, err := newUUID()
	return VolumeAttachmentUUID(u), err
}

// GenVolumeAttachmentUUID generates a new [VolumeAttachmentUUID] for testing
// purposes.
func GenVolumeAttachmentUUID(c interface{ Fatal(...any) }) VolumeAttachmentUUID {
	uuid, err := NewVolumeAttachmentUUID()
	if err != nil {
		c.Fatal(err)
	}
	return uuid
}

// NewVolumeUUID creates a new, valid storage volume identifier.
func NewVolumeUUID() (VolumeUUID, error) {
	u, err := newUUID()
	return VolumeUUID(u), err
}

// GenVolumeUUID generates a new [VolumeUUID] for testing purposes.
func GenVolumeUUID(c interface{ Fatal(...any) }) VolumeUUID {
	uuid, err := NewVolumeUUID()
	if err != nil {
		c.Fatal(err)
	}
	return uuid
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
func (u VolumeAttachmentUUID) String() string {
	return uuid(u).String()
}

// String returns the string representation of this uuid. This function
// satisfies the [fmt.Stringer] interface.
func (u VolumeUUID) String() string {
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
func (u VolumeAttachmentUUID) Validate() error {
	return uuid(u).validate()
}

// Validate returns an error if the [VolumeUUID] is not valid.
func (u VolumeUUID) Validate() error {
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
