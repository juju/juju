// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package container

// StorageConfig defines how the container will be configured to support
// storage requirements.
type StorageConfig struct {

	// AllowMount is true is the container is required to allow
	// mounting block devices.
	AllowMount bool
}
