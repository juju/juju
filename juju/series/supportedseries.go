// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package series

import (
	"sync"

	"github.com/juju/errors"
	"github.com/juju/utils/os"
)

type unknownOSForSeriesError string

func (e unknownOSForSeriesError) Error() string {
	return `unknown OS for series: "` + string(e) + `"`
}

// IsUnknownOSForSeriesError returns true if err is of type unknownOSForSeriesError.
func IsUnknownOSForSeriesError(err error) bool {
	_, ok := errors.Cause(err).(unknownOSForSeriesError)
	return ok
}

type unknownSeriesVersionError string

func (e unknownSeriesVersionError) Error() string {
	return `unknown version for series: "` + string(e) + `"`
}

// IsUnknownSeriesVersionError returns true if err is of type unknownSeriesVersionError.
func IsUnknownSeriesVersionError(err error) bool {
	_, ok := errors.Cause(err).(unknownSeriesVersionError)
	return ok
}

var defaultVersionIDs = map[string]string{
	"arch": "rolling",
}

// seriesVersions provides a mapping between series names and versions.
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
	"win10":       "win10",
	"centos7":     "centos7",
	"arch":        "rolling",
}

var centosSeries = map[string]string{
	"centos7": "centos7",
}

var archSeries = map[string]string{
	"arch": "rolling",
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

// Windows versions come in various flavors:
// Standard, Datacenter, etc. We use string prefix match them to one
// of the following. Specify the longest name in a particular series first
// For example, if we have "Win 2012" and "Win 2012 R2", we specify "Win 2012 R2" first.
// We need to make sure we manually update this list with each new windows release.
var windowsVersionMatchOrder = []string{
	"Hyper-V Server 2012 R2",
	"Hyper-V Server 2012",
	"Windows Server 2012 R2",
	"Windows Server 2012",
	"Windows Storage Server 2012 R2",
	"Windows Storage Server 2012",
	"Windows 7",
	"Windows 8.1",
	"Windows 8",
	"Windows 10",
}

// windowsVersions is a mapping consisting of the output from
// the following WMI query: (gwmi Win32_OperatingSystem).Name
var windowsVersions = map[string]string{
	"Hyper-V Server 2012 R2":         "win2012hvr2",
	"Hyper-V Server 2012":            "win2012hv",
	"Windows Server 2012 R2":         "win2012r2",
	"Windows Server 2012":            "win2012",
	"Windows Storage Server 2012 R2": "win2012r2",
	"Windows Storage Server 2012":    "win2012",
	"Windows 7":                      "win7",
	"Windows 8.1":                    "win81",
	"Windows 8":                      "win8",
	"Windows 10":                     "win10",
}

// GetOSFromSeries will return the operating system based
// on the series that is passed to it
func GetOSFromSeries(series string) (os.OSType, error) {
	if series == "" {
		return os.Unknown, errors.NotValidf("series %q", series)
	}
	if _, ok := ubuntuSeries[series]; ok {
		return os.Ubuntu, nil
	}
	if _, ok := centosSeries[series]; ok {
		return os.CentOS, nil
	}
	if _, ok := archSeries[series]; ok {
		return os.Arch, nil
	}
	for _, val := range windowsVersions {
		if val == series {
			return os.Windows, nil
		}
	}
	for _, val := range macOSXSeries {
		if val == series {
			return os.OSX, nil
		}
	}
	return os.Unknown, errors.Trace(unknownOSForSeriesError(series))
}

var (
	seriesVersionsMutex sync.Mutex
)

// SeriesVersion returns the version for the specified series.
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

	return "", errors.Trace(unknownSeriesVersionError(series))
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
func OSSupportedSeries(os os.OSType) []string {
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

var updatedseriesVersions bool

func updateSeriesVersions() {
	if !updatedseriesVersions {
		updateLocalSeriesVersions()
		updatedseriesVersions = true
	}
}
