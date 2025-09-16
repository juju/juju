// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package blockdevice

import (
	"github.com/juju/utils/v4"

	"github.com/juju/juju/internal/errors"
	internaluuid "github.com/juju/juju/internal/uuid"
)

// BlockDeviceUUID uniquely identifies a block device in the model.
type BlockDeviceUUID string

// NewBlockDeviceUUID creates a new, valid block device identitifier.
func NewBlockDeviceUUID() (BlockDeviceUUID, error) {
	id, err := internaluuid.NewUUID()
	if err != nil {
		return "", errors.Capture(err)
	}
	return BlockDeviceUUID(id.String()), nil
}

// String returns the string representation of this UUID. This function
// satisfies the [fmt.Stringer] interface.
func (u BlockDeviceUUID) String() string {
	return string(u)
}

// Validate returns an error if the [BlockDeviceUUID] is not a valid UUID.
func (u BlockDeviceUUID) Validate() error {
	if u == "" {
		return errors.New("empty uuid")
	}
	internaluuid.IsValidUUIDString(u.String())
	if !utils.IsValidUUIDString(string(u)) {
		return errors.Errorf("invalid uuid %q", u)
	}
	return nil
}
