// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !windows

package paths

// These constants are mirrored in paths_windows.go which contains the
// relavant values for Windows. If you update something here, be sure
// to update it there as well. It is only necessary to document it
// once, here.

const (

	// Temp is the path where Juju may put temporary data.
	Temp string = nixTemp

	// Log is the path where Juju may put log files.
	Log = nixLog

	// Data is the path where Juju may put tools, charms, locks, etc.
	Data = nixData

	// MetricsSpool is the path where Juju may store metrics.
	MetricsSpool = nixMetricsSpool

	// Storage is the path where Juju may mount machine-level
	// storaage.
	Storage = "/var/lib/juju/storage"

	// Conf is the path where Juju may store configuration files.
	Conf = "/etc/juju"

	// JujuRun is the path to the juju-run binary.
	JujuRun = "/usr/bin/juju-run"

	// JujuDumpLogs is the path to the juju-dumplogs binary.
	JujuDumpLogs = "/usr/bin/juju-dumplogs"

	// Cert is the path where Juju may put certificates added by
	// default to the Juju client API certification pool.
	Cert = "/etc/juju/certs.d"
)
