// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package series

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

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
	case strings.ToLower(jujuos.Arch.String()):
		return getValue(archSeries, values["VERSION_ID"])
	case strings.ToLower(jujuos.CentOS.String()):
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
	bufRdr := bufio.NewReader(f)
	// Only find info for precise or later.
	// TODO: only add in series that are supported (i.e. before end of life)
	preciseOrLaterFound := false
	for {
		line, err := bufRdr.ReadString('\n')
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading distro info file file: %v", err)
		}
		// lines are of the form: "12.04 LTS,Precise Pangolin,precise,2011-10-13,2012-04-26,2017-04-26"
		parts := strings.Split(line, ",")
		// Ignore any malformed lines.
		if len(parts) < 3 {
			continue
		}
		series := parts[2]
		if series == "precise" {
			preciseOrLaterFound = true
		}
		if series != "precise" && !preciseOrLaterFound {
			continue
		}
		// the numeric version may contain a LTS moniker so strip that out.
		seriesInfo := strings.Split(parts[0], " ")
		seriesVersions[series] = seriesInfo[0]
		ubuntuSeries[series] = seriesInfo[0]
		if len(seriesInfo) == 2 && seriesInfo[1] == "LTS" {
			ubuntuLts[series] = true
		}
	}
	return nil
}
