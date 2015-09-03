// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package version

import (
	"fmt"
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/juju/os"
)

func osVersion() (string, error) {
	return readSeries()
}

func readSeries() (string, error) {
	values, err := os.ReadOSRelease(osReleaseFile)
	if err != nil {
		return "unknown", err
	}
	updateSeriesVersions()
	return seriesFromOSRelease(values)
}

func seriesFromOSRelease(values map[string]string) (string, error) {
	switch values["ID"] {
	case strings.ToLower(os.Ubuntu.String()):
		return getValue(ubuntuSeries, values["VERSION_ID"])
	case strings.ToLower(os.Arch.String()):
		return getValue(archSeries, values["VERSION_ID"])
	case strings.ToLower(os.CentOS.String()):
		codename := fmt.Sprintf("%s%s", values["ID"], values["VERSION_ID"])
		return getValue(centosSeries, codename)
	default:
		return "unknown", nil
	}
}

func getValue(from map[string]string, val string) (string, error) {
	for serie, ver := range from {
		if ver == val {
			return serie, nil
		}
	}
	return "unknown", errors.New("Could not determine series")
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
