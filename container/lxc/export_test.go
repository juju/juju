// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxc

import (
	"github.com/juju/testing"

	"github.com/juju/juju/container"
)

var (
	ContainerConfigFilename = containerConfigFilename
	ContainerDirFilesystem  = containerDirFilesystem
	GenerateNetworkConfig   = generateNetworkConfig
	NetworkConfigTemplate   = networkConfigTemplate
	RestartSymlink          = restartSymlink
	ReleaseVersion          = &releaseVersion
	PreferFastLXC           = preferFastLXC
	InitProcessCgroupFile   = &initProcessCgroupFile
	RuntimeGOOS             = &runtimeGOOS
)

func GetCreateWithCloneValue(mgr container.Manager) bool {
	return mgr.(*containerManager).createWithClone
}

// PatchTransientErrorInjection is used to patch the transientErrorInjection channel in tests,
// which is used to simulate errors in container creation
// - lxc.CreateContainer will fail with a RetryableCreationError for each value received on this
//channel
func PatchTransientErrorInjectionChannel(n chan interface{}) func() {
	return testing.PatchValue(&transientErrorInjectionChannel, n)
}
