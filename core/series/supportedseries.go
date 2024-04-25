// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package series

import (
	"strings"
	"sync"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	jujuos "github.com/juju/os/v2"
	"github.com/juju/os/v2/series"

	coreos "github.com/juju/juju/core/os"
)

const (
	// Daily defines if a image-stream is set to this, then you get a different
	// set of logic. In this case if you want to test drive new releases, it's
	// required that the image-stream modelconfig is set from released to
	// daily.
	Daily = "daily"
)

// UbuntuDistroInfo is the path for the Ubuntu distro info file.
var UbuntuDistroInfo = series.UbuntuDistroInfo

// SupportedSeriesFunc describes a function that has commonality between
// controller and workload types.
type SupportedSeriesFunc = func(time.Time, string, string) (set.Strings, error)

// ControllerSeries returns all the controller series available to it at the
// execution time.
func ControllerSeries(now time.Time, requestedSeries, imageStream string) (set.Strings, error) {
	supported, err := seriesForTypes(UbuntuDistroInfo, now, requestedSeries, imageStream)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return set.NewStrings(supported.controllerSeries()...), nil
}

// WorkloadSeries returns the supported workload series available to it at the
// execution time.
func WorkloadSeries(now time.Time, requestedSeries, imageStream string) (set.Strings, error) {
	supported, err := seriesForTypes(UbuntuDistroInfo, now, requestedSeries, imageStream)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return set.NewStrings(supported.workloadSeries(false)...), nil
}

// AllWorkloadSeries returns all the workload series (supported or not).
func AllWorkloadSeries(requestedSeries, imageStream string) (set.Strings, error) {
	supported, err := seriesForTypes(UbuntuDistroInfo, time.Now(), requestedSeries, imageStream)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return set.NewStrings(supported.workloadSeries(true)...), nil
}

// AllWorkloadOSTypes returns all the workload os types (supported or not).
func AllWorkloadOSTypes(requestedSeries, imageStream string) (set.Strings, error) {
	supported, err := seriesForTypes(UbuntuDistroInfo, time.Now(), requestedSeries, imageStream)
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := set.NewStrings()
	for _, series := range supported.workloadSeries(true) {
		result.Add(DefaultOSTypeNameFromSeries(series))
	}
	return result, nil
}

func seriesForTypes(path string, now time.Time, requestedSeries, imageStream string) (*supportedInfo, error) {
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

	source := series.NewDistroInfo(path)
	supported := newSupportedInfo(source, allSeriesVersions)
	if err := supported.compile(now); err != nil {
		return nil, errors.Trace(err)
	}

	return supported, nil
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
	osType, err := getOSFromSeries(seriesName)
	if err == nil {
		return osType, nil
	}

	updateSeriesVersionsOnce()
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

// LocalSeriesVersionInfo is patched for tests.
var LocalSeriesVersionInfo = series.LocalSeriesVersionInfo

func updateSeriesVersions() error {
	hostOS, sInfo, err := LocalSeriesVersionInfo()
	if err != nil {
		return errors.Trace(err)
	}
	switch hostOS {
	case jujuos.Ubuntu:
		for seriesName, s := range sInfo {
			// If a series not known and listed as supported, don't support it.
			supported := false
			if current, ok := ubuntuSeries[SeriesName(seriesName)]; ok {
				supported = current.Supported
			}

			ubuntuSeries[SeriesName(seriesName)] = seriesVersion{
				WorkloadType:             ControllerWorkloadType,
				Version:                  s.Version,
				LTS:                      s.LTS,
				Supported:                s.Supported && supported,
				ESMSupported:             s.ESMSupported,
				IgnoreDistroInfoUpdate:   false,
				UpdatedByLocalDistroInfo: s.CreatedByLocalDistroInfo,
			}
		}
	default:
	}
	composeSeriesVersions()
	return nil
}

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

// SeriesVersion returns the version for the specified series.
func SeriesVersion(series string) (string, error) {
	if series == "" {
		return "", errors.Trace(unknownSeriesVersionError(""))
	}
	seriesVersionsMutex.Lock()
	defer seriesVersionsMutex.Unlock()

	seriesName := SeriesName(series)
	if vers, ok := allSeriesVersions[seriesName]; ok {
		return vers.Version, nil
	}
	updateSeriesVersionsOnce()
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
	if ser, ok := versionSeries[version]; ok {
		return ser, nil
	}
	updateSeriesVersionsOnce()
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
	updateSeriesVersionsOnce()
	if vers, ok := ubuntuSeries[seriesName]; ok {
		return vers.Version, nil
	}

	return "", errors.Trace(unknownSeriesVersionError(series))
}

// UbuntuVersions returns the ubuntu versions as a map..
func UbuntuVersions(supported, esmSupported *bool) map[string]string {
	return ubuntuVersions(supported, esmSupported, ubuntuSeries)
}

func ubuntuVersions(
	supported, esmSupported *bool, ubuntuSeries map[SeriesName]seriesVersion,
) map[string]string {
	seriesVersionsMutex.Lock()
	defer seriesVersionsMutex.Unlock()
	save := make(map[string]string)
	for seriesName, val := range ubuntuSeries {
		if supported != nil && val.Supported != *supported {
			continue
		}
		if esmSupported != nil && val.ESMSupported != *esmSupported {
			continue
		}
		save[seriesName.String()] = val.Version
	}
	return save
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

var (
	logger = loggo.GetLogger("juju.juju.series")

	seriesVersionsMutex sync.Mutex
)

// latestLtsSeries is used to ensure we only do
// the work to determine the latest lts series once.
var latestLtsSeries string

// LatestLts returns the Latest LTS Release found in distro-info
func LatestLts() string {
	if latestLtsSeries != "" {
		return latestLtsSeries
	}

	seriesVersionsMutex.Lock()
	defer seriesVersionsMutex.Unlock()
	updateSeriesVersionsOnce()

	var latest SeriesName
	for k, version := range ubuntuSeries {
		if !version.LTS || !version.Supported {
			continue
		}
		if version.Version > ubuntuSeries[latest].Version {
			latest = k
		}
	}

	latestLtsSeries = string(latest)
	return latestLtsSeries
}

// versionSeries provides a mapping between versions and series names.
var (
	versionSeries     map[string]string
	allSeriesVersions map[SeriesName]seriesVersion
)

// UpdateSeriesVersions forces an update of the series versions by querying
// distro-info if possible.
func UpdateSeriesVersions() error {
	seriesVersionsMutex.Lock()
	defer seriesVersionsMutex.Unlock()

	if err := updateSeriesVersions(); err != nil {
		return err
	}
	updateVersionSeries()
	return nil
}

var updatedSeriesVersions bool

func updateSeriesVersionsOnce() {
	if !updatedSeriesVersions {
		if err := updateSeriesVersions(); err != nil {
			logger.Warningf("failed to update distro info: %v", err)
		}
		updateVersionSeries()
		updatedSeriesVersions = true
	}
}

func updateVersionSeries() {
	versionSeries = make(map[string]string, len(allSeriesVersions))
	for k, v := range allSeriesVersions {
		versionSeries[v.Version] = string(k)
	}
}

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
