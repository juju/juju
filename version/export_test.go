// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package version

var (
	ReadSeries                    = readSeries
	KernelToMajor                 = kernelToMajor
	MacOSXSeriesFromKernelVersion = macOSXSeriesFromKernelVersion
	MacOSXSeriesFromMajorVersion  = macOSXSeriesFromMajorVersion
	LSBReleaseFileVar             = &lsbReleaseFile
)

func SetSeriesVersions(value map[string]string) func() {
	origVersions := seriesVersions
	origUpdated := updatedseriesVersions
	seriesVersions = value
	updatedseriesVersions = len(value) != 0
	return func() {
		seriesVersions = origVersions
		updatedseriesVersions = origUpdated
	}
}
