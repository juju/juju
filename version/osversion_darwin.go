// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package version

import (
	"syscall"
)

func sysctlVersion() (string, error) {
	return syscall.Sysctl("kern.osrelease")
}

// osVersion returns the best approximation to what version this machine is.
func osVersion() (string, error) {
	return macOSXSeriesFromKernelVersion(sysctlVersion)
}
