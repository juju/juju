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
	for _, wSeries := range supported.workloadSeries(true) {
		result.Add(DefaultOSTypeNameFromSeries(wSeries))
	}
	return result, nil
}

func seriesForTypes(path string, now time.Time, requestedSeries, imageStream string) (*supportedInfo, error) {
	// We support all of the juju series AND all the ESM supported series.
	// Juju is congruent with the Ubuntu release cycle for its own series (not
	// including centos), so that should be reflected here.
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
	seriesName := SeriesName(series)
	osType, err := getOSFromSeries(seriesName)
	if err == nil {
		return osType, nil
	}

	seriesVersionsMutex.Lock()
	defer seriesVersionsMutex.Unlock()

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
			ubuntuSeries[SeriesName(seriesName)] = seriesVersion{
				WorkloadType:             ControllerWorkloadType,
				Version:                  s.Version,
				LTS:                      s.LTS,
				Supported:                s.Supported,
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

	return coreos.Unknown, errors.Trace(unknownOSForSeriesError(series))
}

var (
	logger = loggo.GetLogger("juju.juju.series")

	seriesVersionsMutex sync.Mutex
)

// latestLtsSeries is used to ensure we only do
// the work to determine the latest lts series once.
var latestLtsSeries string

// LatestLTS returns the Latest LTS Release found in distro-info
func LatestLTS() string {
	if latestLtsSeries != "" {
		return latestLtsSeries
	}

	seriesVersionsMutex.Lock()
	defer seriesVersionsMutex.Unlock()
	updateSeriesVersionsOnce()

	var latest SeriesName
	for k, seriesVersion := range ubuntuSeries {
		if !seriesVersion.LTS || !seriesVersion.Supported {
			continue
		}
		if seriesVersion.Version > ubuntuSeries[latest].Version {
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

var updatedseriesVersions bool

func updateSeriesVersionsOnce() {
	if !updatedseriesVersions {
		if err := updateSeriesVersions(); err != nil {
			logger.Warningf("failed to update distro info: %v", err)
		}
		updateVersionSeries()
		updatedseriesVersions = true
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
