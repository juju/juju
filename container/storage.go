// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package container

import (
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider"
)

// StorageConfig defines how the container will be configured to support
// storage requirements.
type StorageConfig struct {

	// AllowMount is true is the container is required to allow
	// mounting block devices.
	AllowMount bool
}

// NewStorageConfig returns a StorageConfig used to specify the
// configuration the container uses to support storage.
func NewStorageConfig(volumes []storage.VolumeParams) *StorageConfig {
	allowMount := false
	// If there is a volume using a loop provider, then
	// allow mount must be true.
	for _, v := range volumes {
		allowMount = v.Provider == provider.LoopProviderType
		if allowMount {
			break
		}
	}
	// TODO(wallyworld) - add config for HostLoopProviderType
	return &StorageConfig{allowMount}
}
