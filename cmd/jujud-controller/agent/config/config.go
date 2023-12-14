// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package config

import (
	"github.com/juju/juju/core/paths"
)

var (
	DataDir = paths.DataDir(paths.CurrentOS())
	LogDir  = paths.LogDir(paths.CurrentOS())
)
