// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build darwin

package version

import (
	"fmt"
	"syscall"
)

func sysctlVersion() (string, error) {
	return syscall.Sysctl("kern.osrelease")
}

var getSysctlVersion = sysctlVersion

// osVersion returns the best approximation to what version this machine is.
// If we are unable to determine the OSVersion, we return "unknown".
func osVersion() string {
	majorVersion, err := kernelToMajor(getSysctlVersion)
	if err != nil {
		logger.Infof("unable to determine OS version: %v", err)
		return "unknown"
	}
	return fmt.Sprintf("darwin%d", majorVersion)
}
