// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package config

import (
	"github.com/juju/os/series"

	"github.com/juju/juju/core/paths"
)

var (
	JujuRun        = paths.MustSucceed(paths.JujuRun(series.MustHostSeries()))
	JujuDumpLogs   = paths.MustSucceed(paths.JujuDumpLogs(series.MustHostSeries()))
	JujuIntrospect = paths.MustSucceed(paths.JujuIntrospect(series.MustHostSeries()))
	LogDir         = paths.MustSucceed(paths.LogDir(series.MustHostSeries()))
)
