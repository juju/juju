// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ubuntu

var DistroInfo = &distroInfo

func SetSeriesVersions(value map[string]string) func() {
	origVersions := seriesVersions
	origUpdated := updatedseriesVersions
	seriesVersions = value
	updatedseriesVersions = false
	return func() {
		seriesVersions = origVersions
		updatedseriesVersions = origUpdated
	}
}
