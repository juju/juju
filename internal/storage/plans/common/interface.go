// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/juju/domain/blockdevice"
)

type Plan interface {
	AttachVolume(volumeInfo map[string]string) (blockdevice.BlockDevice, error)
	DetachVolume(volumeInfo map[string]string) error
}
