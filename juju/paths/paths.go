// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package paths

import (
	"github.com/juju/utils/os"
)

// These values must be known at run-time so that we can build scripts
// for OSs other than our processes host OS. If a value is not needed
// at runtime, do not put it here. Instead, place it in the file which
// is guarded by a compilation flag for its proper OS.
const (
	nixData         string = "/var/lib/juju"
	nixLog                 = "/var/log"
	nixTemp                = "/tmp"
	nixMetricsSpool        = "/var/lib/juju/metricspool"

	winData         = "C:/Juju/lib/juju"
	winLog          = "C:/Juju/log"
	winTemp         = "C:/Juju/tmp"
	winMetricsSpool = "C:/Juju/lib/juju/metricspool"
)

// DataForOS returns the correct Data path for the given OS. If the OS
// is known at compile-time, use the Data const instead.
func DataForOS(osType os.OSType) string {
	if osType == os.Windows {
		return winData
	}
	return nixData
}

// LogForOS returns the correct Log path for the given OS. If the OS
// is known at compile-time, use the Log const instead.
func LogForOS(osType os.OSType) string {
	if osType == os.Windows {
		return winLog
	}
	return nixLog
}

// MetricsSpoolForOS returns the correct MetricsSpool path for the given OS. If the OS
// is known at compile-time, use the MetricsSpool const instead.
func MetricsSpoolForOS(osType os.OSType) string {
	if osType == os.Windows {
		return winMetricsSpool
	}
	return nixMetricsSpool
}

// TempForOS returns the correct Temp path for the given OS. If the OS
// is known at compile-time, use the Temp const instead.
func TempForOS(osType os.OSType) string {
	if osType == os.Windows {
		return winTemp
	}
	return nixTemp
}
