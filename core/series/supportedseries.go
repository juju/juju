// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package series

import (
	"strings"
	"sync"

	"github.com/juju/collections/set"
	"github.com/juju/errors"

	coreos "github.com/juju/juju/core/os"
)

const (
	// Daily defines if a image-stream is set to this, then you get a different
	// set of logic. In this case if you want to test drive new releases, it's
	// required that the image-stream modelconfig is set from released to
	// daily.
	Daily = "daily"
)

// ControllerSeries returns all the controller series available to it at the
// execution time.
func ControllerSeries(requestedSeries, imageStream string) (set.Strings, error) {
	supported, err := seriesForTypes(requestedSeries, imageStream)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return set.NewStrings(controllerSeries(supported)...), nil
}

// WorkloadSeries returns the supported workload series available to it at the
// execution time.
func WorkloadSeries(requestedSeries, imageStream string) (set.Strings, error) {
	supported, err := seriesForTypes(requestedSeries, imageStream)
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := set.NewStrings(workloadSeries(supported, false)...)
	// Noble is opt in for 2.9 so remove it
	// from the default choices. The user can
	// still use --force if they want noble.
	result.Remove(Noble.String())
	return result, nil
}

// AllWorkloadSeries returns all the workload series (supported or not).
func AllWorkloadSeries(requestedSeries, imageStream string) (set.Strings, error) {
	supported, err := seriesForTypes(requestedSeries, imageStream)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return set.NewStrings(workloadSeries(supported, true)...), nil
}

// AllWorkloadOSTypes returns all the workload os types (supported or not).
func AllWorkloadOSTypes(requestedSeries, imageStream string) (set.Strings, error) {
	supported, err := seriesForTypes(requestedSeries, imageStream)
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := set.NewStrings()
	for _, series := range workloadSeries(supported, true) {
		result.Add(DefaultOSTypeNameFromSeries(series))
	}
	return result, nil
}

func seriesForTypes(requestedSeries, imageStream string) (map[SeriesName]seriesVersion, error) {
	// We support all of the juju series AND all the ESM supported series.
	// Juju is congruent with the Ubuntu release cycle for it's own series (not
	// including centos and windows), so that should be reflected here.
	//
	// For non-LTS releases; they'll appear in juju/os as default available, but
	// after reading the `/usr/share/distro-info/ubuntu.csv` on the Ubuntu distro
	// the non-LTS should disappear if they're not in the release window for that
	// series.
	seriesVersionsMutex.Lock()
	defer seriesVersionsMutex.Unlock()
	composeSeriesVersions()
	if requestedSeries != "" && imageStream == Daily {
		setSupported(allSeriesVersions, requestedSeries)
	}

	return allSeriesVersions, nil
}

// GetOSFromSeries will return the operating system based
// on the series that is passed to it
func GetOSFromSeries(series string) (coreos.OSType, error) {
	if series == "" {
		return coreos.Unknown, errors.NotValidf("series %q", series)
	}

	seriesVersionsMutex.Lock()
	defer seriesVersionsMutex.Unlock()

	seriesName := SeriesName(series)
	return getOSFromSeries(seriesName)
}

// DefaultOSTypeNameFromSeries returns the operating system based
// on the given series, defaulting to Ubuntu for unknown series.
func DefaultOSTypeNameFromSeries(series string) string {
	osType, err := GetOSFromSeries(series)
	if err != nil {
		osType = coreos.Ubuntu
	}
	return strings.ToLower(osType.String())
}

const (
	genericLinuxSeries  = "genericlinux"
	genericLinuxVersion = "genericlinux"
)

func composeSeriesVersions() {
	allSeriesVersions = make(map[SeriesName]seriesVersion)
	for k, v := range ubuntuSeries {
		allSeriesVersions[k] = v
	}
	for _, v := range windowsVersions {
		allSeriesVersions[SeriesName(v.Version)] = v
	}
	for _, v := range windowsNanoVersions {
		allSeriesVersions[SeriesName(v.Version)] = v
	}
	for k, v := range macOSXSeries {
		allSeriesVersions[k] = v
	}
	for k, v := range centosSeries {
		allSeriesVersions[k] = v
	}
	for k, v := range opensuseSeries {
		allSeriesVersions[k] = v
	}
	for k, v := range kubernetesSeries {
		allSeriesVersions[k] = v
	}
	allSeriesVersions[genericLinuxSeries] = seriesVersion{
		WorkloadType: OtherWorkloadType,
		Version:      genericLinuxVersion,
		Supported:    true,
	}

	versionSeries = make(map[string]string, len(allSeriesVersions))
	for k, v := range allSeriesVersions {
		versionSeries[v.Version] = string(k)
	}
}

// Windows versions come in various flavors:
// Standard, Datacenter, etc. We use string prefix match them to one
// of the following. Specify the longest name in a particular series first
// For example, if we have "Win 2012" and "Win 2012 R2", we specify "Win 2012 R2" first.
// We need to make sure we manually update this list with each new windows release.
var windowsVersionMatchOrder = []string{
	"Hyper-V Server 2012 R2",
	"Hyper-V Server 2012",
	"Windows Server 2008 R2",
	"Windows Server 2012 R2",
	"Windows Server 2012",
	"Hyper-V Server 2016",
	"Windows Server 2016",
	"Windows Server 2019",
	"Windows Storage Server 2012 R2",
	"Windows Storage Server 2012",
	"Windows Storage Server 2016",
	"Windows 7",
	"Windows 8.1",
	"Windows 8",
	"Windows 10",
}

// WindowsVersionSeries returns the series (eg: win2012r2) for the specified version
// (eg: Windows Server 2012 R2 Standard)
func WindowsVersionSeries(version string) (string, error) {
	if version == "" {
		return "", errors.Trace(unknownVersionSeriesError(""))
	}
	for _, val := range windowsVersionMatchOrder {
		if strings.HasPrefix(version, val) {
			if vers, ok := windowsVersions[val]; ok {
				return vers.Version, nil
			}
		}
	}
	return "", errors.Trace(unknownVersionSeriesError(""))
}

// CentOSVersionSeries validates that the supplied series (eg: centos7)
// is supported.
func CentOSVersionSeries(version string) (string, error) {
	if version == "" {
		return "", errors.Trace(unknownVersionSeriesError(""))
	}
	if ser, ok := centosSeries[SeriesName(version)]; ok {
		return ser.Version, nil
	}
	return "", errors.Trace(unknownVersionSeriesError(""))

}

var seriesVersionOnce sync.Once

// SeriesVersion returns the version for the specified series.
func SeriesVersion(series string) (string, error) {
	if series == "" {
		return "", errors.Trace(unknownSeriesVersionError(""))
	}
	seriesVersionsMutex.Lock()
	defer seriesVersionsMutex.Unlock()
	seriesVersionOnce.Do(composeSeriesVersions)
	seriesName := SeriesName(series)
	if vers, ok := allSeriesVersions[seriesName]; ok {
		return vers.Version, nil
	}
	return "", errors.Trace(unknownSeriesVersionError(series))
}

// VersionSeries returns the series (e.g.trusty) for the specified version (e.g. 14.04).
func VersionSeries(version string) (string, error) {
	if version == "" {
		return "", errors.Trace(unknownVersionSeriesError(""))
	}
	seriesVersionsMutex.Lock()
	defer seriesVersionsMutex.Unlock()
	seriesVersionOnce.Do(composeSeriesVersions)
	if ser, ok := versionSeries[version]; ok {
		return ser, nil
	}
	return "", errors.Trace(unknownVersionSeriesError(version))
}

// UbuntuSeriesVersion returns the ubuntu version for the specified series.
func UbuntuSeriesVersion(series string) (string, error) {
	if series == "" {
		return "", errors.Trace(unknownSeriesVersionError(""))
	}
	seriesVersionsMutex.Lock()
	defer seriesVersionsMutex.Unlock()

	seriesName := SeriesName(series)
	if vers, ok := ubuntuSeries[seriesName]; ok {
		return vers.Version, nil
	}
	return "", errors.Trace(unknownSeriesVersionError(series))
}

// UbuntuSeries returns the ubuntu series names.
func UbuntuSeries() set.Strings {
	seriesVersionsMutex.Lock()
	defer seriesVersionsMutex.Unlock()

	result := set.NewStrings()
	for seriesName := range ubuntuSeries {
		result.Add(seriesName.String())
	}
	return result
}

// WindowsVersions returns all windows versions as a map
// If we have nan and windows version in common, nano takes precedence
func WindowsVersions() map[string]string {
	save := make(map[string]string)
	for seriesName, val := range windowsVersions {
		save[seriesName] = val.Version
	}
	return save
}

// IsWindowsNano tells us whether the provided series is a
// nano series. It may seem futile at this point, but more
// nano series will come up with time.
// This is here and not in a windows specific package
// because we might want to take decisions dependant on
// whether we have a nano series or not in more general code.
func IsWindowsNano(series string) bool {
	for _, val := range windowsNanoVersions {
		if val.Version == series {
			return true
		}
	}
	return false
}

func getOSFromSeries(series SeriesName) (coreos.OSType, error) {
	if _, ok := ubuntuSeries[series]; ok {
		return coreos.Ubuntu, nil
	}
	if _, ok := macOSXSeries[series]; ok {
		return coreos.OSX, nil
	}
	if _, ok := centosSeries[series]; ok {
		return coreos.CentOS, nil
	}
	if _, ok := opensuseSeries[series]; ok {
		return coreos.OpenSUSE, nil
	}
	if _, ok := kubernetesSeries[series]; ok {
		return coreos.Kubernetes, nil
	}
	if series == genericLinuxSeries {
		return coreos.GenericLinux, nil
	}
	for _, val := range windowsVersions {
		if val.Version == string(series) {
			return coreos.Windows, nil
		}
	}
	for _, val := range windowsNanoVersions {
		if val.Version == string(series) {
			return coreos.Windows, nil
		}
	}

	return coreos.Unknown, errors.Trace(unknownOSForSeriesError(series))
}

// versionSeries provides a mapping between versions and series names.
var (
	seriesVersionsMutex sync.Mutex
	versionSeries       map[string]string
	allSeriesVersions   map[SeriesName]seriesVersion
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

type unknownVersionSeriesError string

func (e unknownVersionSeriesError) Error() string {
	return `unknown series for version: "` + string(e) + `"`
}
