// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package version

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
)

type OSType int

const (
	Unknown OSType = iota
	Ubuntu
	Windows
	OSX
	CentOS
)

func (t OSType) String() string {
	switch t {
	case Ubuntu:
		return "Ubuntu"
	case Windows:
		return "Windows"
	case OSX:
		return "OSX"
	case CentOS:
		return "CentOS"
	}
	return "Unknown"
}

// seriesVersions provides a mapping between Ubuntu series names and version numbers.
// The values here are current as of the time of writing. On Ubuntu systems, we update
// these values from /usr/share/distro-info/ubuntu.csv to ensure we have the latest values.
// On non-Ubuntu systems, these values provide a nice fallback option.
// Exported so tests can change the values to ensure the distro-info lookup works.
var seriesVersions = map[string]string{
	"precise":     "12.04",
	"quantal":     "12.10",
	"raring":      "13.04",
	"saucy":       "13.10",
	"trusty":      "14.04",
	"utopic":      "14.10",
	"vivid":       "15.04",
	"win2012hvr2": "win2012hvr2",
	"win2012hv":   "win2012hv",
	"win2012r2":   "win2012r2",
	"win2012":     "win2012",
	"win7":        "win7",
	"win8":        "win8",
	"win81":       "win81",
	"centos7":     "7",
}

var centosSeries = map[string]string{
	"centos7": "7",
}

var ubuntuSeries = map[string]string{
	"precise": "12.04",
	"quantal": "12.10",
	"raring":  "13.04",
	"saucy":   "13.10",
	"trusty":  "14.04",
	"utopic":  "14.10",
	"vivid":   "15.04",
}

// windowsVersions is a mapping consisting of the output from
// the following WMI query: (gwmi Win32_OperatingSystem).Name
// Windows versions come in various flavors:
// Standard, Datacenter, etc. We use regex to match them to one
// of the following. Specify the longest name in a particular serie first
// For example, if we have "Win 2012" and "Win 2012 R2". we specify "Win 2012 R2" first
// TODO: Replace this with actuall full names once we compile a complete
// list with al flavors
var windowsVersions = map[string]string{
	"Hyper-V Server 2012 R2":         "win2012hvr2",
	"Hyper-V Server 2012":            "win2012hv",
	"Windows Server 2012 R2":         "win2012r2",
	"Windows Server 2012":            "win2012",
	"Windows Storage Server 2012 R2": "win2012r2",
	"Windows Storage Server 2012":    "win2012",
	"Windows 7":                      "win7",
	"Windows 8":                      "win8",
	"Windows 8.1":                    "win81",
}

var distroInfo = "/usr/share/distro-info/ubuntu.csv"

// GetOSFromSeries will return the operating system based
// on the series that is passed to it
func GetOSFromSeries(series string) (OSType, error) {
	if _, ok := ubuntuSeries[series]; ok {
		return Ubuntu, nil
	}
	if _, ok := centosSeries[series]; ok {
		return CentOS, nil
	}
	for _, val := range windowsVersions {
		if val == series {
			return Windows, nil
		}
	}
	for _, val := range macOSXSeries {
		if val == series {
			return OSX, nil
		}
	}

	return Unknown, fmt.Errorf("invalid series %q", series)
}

var (
	seriesVersionsMutex   sync.Mutex
	updatedseriesVersions bool
)

// SeriesVersion returns the version number for the specified Ubuntu series.
func SeriesVersion(series string) (string, error) {
	if series == "" {
		panic("cannot pass empty series to SeriesVersion()")
	}
	seriesVersionsMutex.Lock()
	defer seriesVersionsMutex.Unlock()
	if vers, ok := seriesVersions[series]; ok {
		return vers, nil
	}
	updateSeriesVersions()
	if vers, ok := seriesVersions[series]; ok {
		return vers, nil
	}
	return "", fmt.Errorf("invalid series %q", series)
}

// SupportedSeries returns the series on which we can run Juju workloads.
func SupportedSeries() []string {
	seriesVersionsMutex.Lock()
	defer seriesVersionsMutex.Unlock()
	updateSeriesVersions()
	var series []string
	for s := range seriesVersions {
		series = append(series, s)
	}
	return series
}

// OSSupportedSeries returns the series of the specified OS on which we
// can run Juju workloads.
func OSSupportedSeries(os OSType) []string {
	var osSeries []string
	for _, series := range SupportedSeries() {
		seriesOS, err := GetOSFromSeries(series)
		if err != nil || seriesOS != os {
			continue
		}
		osSeries = append(osSeries, series)
	}
	return osSeries
}

func updateSeriesVersions() {
	if !updatedseriesVersions {
		err := updateDistroInfo()
		if err != nil {
			logger.Warningf("failed to update distro info: %v", err)
		}
		updatedseriesVersions = true
	}
}

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
	}
	return nil
}
