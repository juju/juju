// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package series

import (
	"encoding/csv"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/juju/errors"
	jujuos "github.com/juju/utils/os"
)

var (
	// osReleaseFile is the name of the file that is read in order to determine
	// the linux type release version.
	osReleaseFile = "/etc/os-release"
)

func readSeries() (string, error) {
	values, err := jujuos.ReadOSRelease(osReleaseFile)
	if err != nil {
		return "unknown", err
	}
	updateSeriesVersionsOnce()
	return seriesFromOSRelease(values)
}

func seriesFromOSRelease(values map[string]string) (string, error) {
	switch values["ID"] {
	case strings.ToLower(jujuos.Ubuntu.String()):
		return getValue(ubuntuSeries, values["VERSION_ID"])
	case strings.ToLower(jujuos.CentOS.String()):
		codename := fmt.Sprintf("%s%s", values["ID"], values["VERSION_ID"])
		return getValue(centosSeries, codename)
	case strings.ToLower(jujuos.OpenSUSE.String()):
		codename := fmt.Sprintf("%s%s",
			values["ID"],
			strings.Split(values["VERSION_ID"], ".")[0])
		return getValue(opensuseSeries, codename)
	default:
		return genericLinuxSeries, nil
	}
}

func getValue(from map[string]string, val string) (string, error) {
	for serie, ver := range from {
		if ver == val {
			return serie, nil
		}
	}
	return "unknown", errors.New("could not determine series")
}

// ReleaseVersion looks for the value of VERSION_ID in the content of
// the os-release.  If the value is not found, the file is not found, or
// an error occurs reading the file, an empty string is returned.
func ReleaseVersion() string {
	release, err := jujuos.ReadOSRelease(osReleaseFile)
	if err != nil {
		return ""
	}
	return release["VERSION_ID"]
}

func updateLocalSeriesVersions() error {
	return updateDistroInfo()
}

var distroInfo = "/usr/share/distro-info/ubuntu.csv"

// updateDistroInfo updates seriesVersions from /usr/share/distro-info/ubuntu.csv if possible..
func updateDistroInfo() error {
	// We need to find the series version eg 12.04 from the series eg precise. Use the information found in
	// /usr/share/distro-info/ubuntu.csv provided by distro-info-data package.
	f, err := os.Open(distroInfo)
	if err != nil {
		// On non-Ubuntu systems this file won't exist but that's expected.
		return nil
	}
	defer f.Close()

	csvReader := csv.NewReader(f)
	csvReader.FieldsPerRecord = -1
	records, err := csvReader.ReadAll()
	if err != nil {
		return errors.Annotatef(err, "reading %s", distroInfo)
	}
	fieldNames := records[0]
	records = records[1:]

	// We ignore all series prior to precise.
	//
	// TODO(axw) only add in series that are supported? (i.e. before end of life)
	// Can we really do this? Users might have Extended Security Maintenance.

	now := time.Now()
	var foundPrecise bool
	for _, fields := range records {
		var version, series string
		var release string
		for i, field := range fields {
			if i >= len(fieldNames) {
				break
			}
			switch fieldNames[i] {
			case "version":
				version = field
			case "series":
				series = field
			case "release":
				release = field
			}
		}
		if version == "" || series == "" || release == "" {
			// Ignore malformed line.
			continue
		}
		if !foundPrecise {
			if series != "precise" {
				continue
			}
			foundPrecise = true
		}

		releaseDate, err := time.Parse("2006-01-02", release)
		if err != nil {
			// Ignore lines with invalid release dates.
			continue
		}

		// The numeric version may contain a LTS moniker so strip that out.
		trimmedVersion := strings.TrimSuffix(version, " LTS")
		seriesVersions[series] = trimmedVersion
		ubuntuSeries[series] = trimmedVersion
		if trimmedVersion != version && !now.Before(releaseDate) {
			// We only record that a series is LTS if its release
			// date has passed. This allows the series to be tested
			// pre-release, without affecting default series.
			ubuntuLTS[series] = true
		}
	}
	return nil
}
