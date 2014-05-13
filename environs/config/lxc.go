// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package config

import (
	"strconv"

	"launchpad.net/juju-core/version/ubuntu"
)

func useFastLXC(preferredSeries string) bool {
	if preferredSeries == "" {
		return false
	}
	vers, err := ubuntu.SeriesVersion(preferredSeries)
	if err != nil {
		return false
	}
	value, err := strconv.ParseFloat(vers, 64)
	if err != nil {
		return false
	}
	return value >= 14.04
}
