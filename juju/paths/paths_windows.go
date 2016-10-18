// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build windows

const (
	Temp         string = winTemp
	Log                 = winLog
	Data                = winData
	MetricsSpool        = winMetricsSpool
	Storage             = "C:/Juju/lib/juju/storage"
	Conf                = "C:/Juju/etc"
	JujuRun             = "C:/Juju/bin/juju-run.exe"
	JujuDumpLogs        = "C:/Juju/bin/juju-dumplogs.exe"
	Cert                = "C:/Juju/certs"
	UniterState         = "C:/Juju/lib/juju/uniter/state"
)
