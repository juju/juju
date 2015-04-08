// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package container

import "errors"

// ErrLoopMountNotAllowed is used when loop devices are requested to be
// mounted inside an LXC container, but this has not been allowed using
// an environment config setting.
var ErrLoopMountNotAllowed = errors.New(`
Mounting of loop devices inside LXC containers must be explicitly enabled using this environment config setting:
  allow-lxc-loop-mounts=true
`[1:])

// StorageConfig defines how the container will be configured to support
// storage requirements.
type StorageConfig struct {

	// AllowMount is true is the container is required to allow
	// mounting block devices.
	AllowMount bool
}
