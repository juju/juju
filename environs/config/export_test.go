// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package config

var (
	DistroLtsSeries = &distroLtsSeries
)

func ResetCachedLtsSeries() {
	latestLtsSeries = ""
}
