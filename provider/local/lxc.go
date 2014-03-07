// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local

import (
	"os"
	"strconv"

	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/version"
)

// envKeyTestingForceSlow is an environment variable name to force the slow
// lxc path. Setting to any non-empty value will force the slow path.
const envKeyTestingForceSlow = "JUJU_TESTING_LXC_FORCE_SLOW"

// releaseVersion is a function that returns a string representing the
// DISTRIB_RELEASE from the /etc/lsb-release file.
var releaseVersion = version.ReleaseVersion

func useFastLXC(containerType instance.ContainerType) bool {
	if containerType != instance.LXC {
		return false
	}
	if os.Getenv(envKeyTestingForceSlow) != "" {
		return false
	}
	release := releaseVersion()
	if release == "" {
		return false
	}
	value, err := strconv.ParseFloat(release, 64)
	if err != nil {
		return false
	}
	return value >= 14.04
}
