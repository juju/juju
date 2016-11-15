// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package paths

// collection couples together all the paths Juju knows about into a
// struct. Instances should be passed by value.
type Collection struct {

	// data is the path where Juju may put tools, charms, locks, etc.
	Data string

	// log is the path where Juju may put log files.
	Log string

	// CloudInitOutputLogPath is the path to the log for cloud-init.
	CloudInitOutputLogPath string

	// temp is the path where Juju may put temporary data.
	Temp string

	// metricsSpool is the path where Juju may store metrics.
	MetricsSpool string

	// storage is the path where Juju may mount machine-level
	// storaage.
	Storage string

	// Conf is the path where Juju may store configuration files.
	Conf string

	// jujuRun is the path to the juju-run binary.
	JujuRun string

	// jujuDumpLogs is the path to the juju-dumplogs binary.
	JujuDumpLogs string

	// Cert is the path where Juju may put certificates added by
	// default to the Juju client API certification pool.
	Cert string
}

var (
	Nix = Collection{
		Cert:                   "/etc/juju/certs.d",
		Conf:                   "/etc/juju",
		Data:                   "/var/lib/juju",
		JujuDumpLogs:           "/usr/bin/juju-dumplogs",
		CloudInitOutputLogPath: "/var/log/cloud-init-output.log",
		JujuRun:                "/usr/bin/juju-run",
		Log:                    "/var/log",
		MetricsSpool:           "/var/lib/juju/metricspool",
		Storage:                "/var/lib/juju/storage",
		Temp:                   "/tmp",
	}
	Windows = Collection{
		Cert:         "C:/Juju/certs",
		Conf:         "C:/Juju/etc",
		Data:         "C:/Juju/lib/juju",
		JujuDumpLogs: "C:/Juju/bin/juju-dumplogs.exe",
		JujuRun:      "C:/Juju/bin/juju-run.exe",
		Log:          "C:/Juju/log",
		MetricsSpool: "C:/Juju/lib/juju/metricspool",
		Storage:      "C:/Juju/lib/juju/storage",
		Temp:         "C:/Juju/tmp",
		CloudInitOutputLogPath: "C:/Juju/log/cloud-init-output.log",
	}
)
