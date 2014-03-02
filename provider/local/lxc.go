// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local

import (
	"io/ioutil"
	"os"
	"strconv"
	"strings"

	"launchpad.net/juju-core/instance"
)

// envKeyTestingForceSlow is an environment variable name to force the slow
// lxc path. Setting to any non-empty value will force the slow path.
const envKeyTestingForceSlow = "JUJU_TESTING_LXC_FORCE_SLOW"

// lsbReleaseFile is the name of the file that is read in order to determine
// the release version of ubuntu.
var lsbReleaseFile = "/etc/lsb-release"

func useFastLXC(containerType instance.ContainerType) bool {
	if containerType != instance.LXC {
		return false
	}
	if os.Getenv(envKeyTestingForceSlow) != "" {
		return false
	}
	release := getReleaseVersion()
	if release == "" {
		return false
	}
	value, err := strconv.ParseFloat(release, 64)
	if err != nil {
		return false
	}
	return value >= 14.04
}

// getReleaseVersion looks for the value of DISTRIB_RELEASE in the content of
// the lsbReleaseFile.  If the value is not found, the file is not found, or
// an error occurs reading the file, an empty string is returned.
func getReleaseVersion() string {
	content, err := ioutil.ReadFile(lsbReleaseFile)
	if err != nil {
		return ""
	}
	const prefix = "DISTRIB_RELEASE="
	for _, line := range strings.Split(string(content), "\n") {
		if strings.HasPrefix(line, prefix) {
			return strings.Trim(line[len(prefix):], "\t '\"")
		}
	}
	return ""
}
