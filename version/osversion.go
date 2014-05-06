// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !darwin

package version

func getOSVersion() string {
	return readSeries(lsbReleaseFile)
}
