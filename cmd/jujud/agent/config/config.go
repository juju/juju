// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package config

import (
	"github.com/juju/os/series"

	"github.com/juju/juju/core/paths"
)

var (
	DataDir = paths.MustSucceed(paths.DataDir(series.MustHostSeries()))
	LogDir  = paths.MustSucceed(paths.LogDir(series.MustHostSeries()))
)
