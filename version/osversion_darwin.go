// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build darwin

package version

import (
	"fmt"
	"strconv"
	"strings"
	"syscall"

	"github.com/juju/loggo"
)

var logger = loggo.GetLogger("juju.version")

func sysctlVersion() (string, error) {
	return syscall.Sysctl("kern.osrelease")
}

var getSysctlVersion = sysctlVersion

// getMajorVersion returns the Major portion of the darwin Kernel Version
func getMajorVersion() (int, error) {
	fullVersion, err := getSysctlVersion()
	if err != nil {
		return 0, err
	}
	parts := strings.SplitN(fullVersion, ".", 2)
	majorVersion, err := strconv.ParseInt(parts[0], 10, 32)
	if err != nil {
		return 0, err
	}
	return int(majorVersion), nil
}

// getOSVersion returns the best approximation to what version this machine is.
// If we are unable to determine the OSVersion, we return "unknown".
func getOSVersion() string {
	majorVersion, err := getMajorVersion()
	if err != nil {
		logger.Infof("unable to determine OS version: %v", err)
		return "unknown"
	}
	return fmt.Sprintf("darwin%d", majorVersion)
}
