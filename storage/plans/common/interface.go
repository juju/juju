// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/juju/storage"
)

type Plan interface {
	AttachVolume(volumeInfo map[string]string) (storage.BlockDevice, error)
	DetachVolume(volumeInfo map[string]string) error
}
