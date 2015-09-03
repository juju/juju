// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package version

import (
	"github.com/juju/juju/juju/os"
)

func osVersion() (string, error) {
	return readSeries()
}

// ReleaseVersion looks for the value of VERSION_ID in the content of
// the os-release.  If the value is not found, the file is not found, or
// an error occurs reading the file, an empty string is returned.
func ReleaseVersion() string {
	release, err := os.ReadOSRelease(osReleaseFile)
	if err != nil {
		return ""
	}
	return release["VERSION_ID"]
}
