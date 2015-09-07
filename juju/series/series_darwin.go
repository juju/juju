// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package series

import (
	"syscall"
)

func sysctlVersion() (string, error) {
	return syscall.Sysctl("kern.osrelease")
}

// readSeries returns the best approximation to what version this machine is.
func readSeries() (string, error) {
	return macOSXSeriesFromKernelVersion(sysctlVersion)
}
