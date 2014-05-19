// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxc

import "launchpad.net/juju-core/container"

var (
	ContainerConfigFilename = containerConfigFilename
	ContainerDirFilesystem  = containerDirFilesystem
	GenerateNetworkConfig   = generateNetworkConfig
	NetworkConfigTemplate   = networkConfigTemplate
	RestartSymlink          = restartSymlink
	ReleaseVersion          = &releaseVersion
	PreferFastLXC           = preferFastLXC
)

func GetCreateWithCloneValue(mgr container.Manager) bool {
	return mgr.(*containerManager).createWithClone
}
