// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package base

import (
	"strings"
	"sync"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	jujuos "github.com/juju/os/v2"
	"github.com/juju/os/v2/series"

	"github.com/juju/juju/core/os/ostype"
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

// AllWorkloadVersions returns all the workload versions (supported or not).
func AllWorkloadVersions(requestedSeries, imageStream string) (set.Strings, error) {
	supported, err := seriesForTypes(UbuntuDistroInfo, time.Now(), requestedSeries, imageStream)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return set.NewStrings(supported.workloadVersions(true)...), nil
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
	// For non-LTS releases; they'll appear in juju/os as default available, but
	// after reading the `/usr/share/distro-info/ubuntu.csv` on the Ubuntu distro
	// the non-LTS should disappear if they're not in the release window for that
	// series.
	seriesVersionsMutex.Lock()
	defer seriesVersionsMutex.Unlock()
	updateSeriesVersionsOnce()
	all := getAllSeriesVersions()
	if requestedSeries != "" && imageStream == Daily {
		setSupported(all, requestedSeries)
	}
	source := series.NewDistroInfo(path)
	supported := newSupportedInfo(source, all)
	if err := supported.compile(now); err != nil {
		return nil, errors.Trace(err)
	}

	return supported, nil
}

func getAllSeriesVersions() map[SeriesName]seriesVersion {
	copy := make(map[SeriesName]seriesVersion, len(allSeriesVersions))
	for name, v := range allSeriesVersions {
		copy[name] = v
	}
	return copy
}

// GetOSFromSeries will return the operating system based
// on the series that is passed to it
func GetOSFromSeries(series string) (ostype.OSType, error) {
	if series == "" {
		return ostype.Unknown, errors.NotValidf("series %q", series)
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
		osType = ostype.Ubuntu
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
			key := SeriesName(seriesName)
			if _, known := ubuntuSeries[key]; known {
				// We only update unknown/new series.
				continue
			}
			ubuntuSeries[key] = seriesVersion{
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
	for k, v := range centosSeries {
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

// UbuntuVersions returns the ubuntu versions as a map.
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

func getOSFromSeries(series SeriesName) (ostype.OSType, error) {
	if _, ok := ubuntuSeries[series]; ok {
		return ostype.Ubuntu, nil
	}
	if _, ok := centosSeries[series]; ok {
		return ostype.CentOS, nil
	}
	if _, ok := kubernetesSeries[series]; ok {
		return ostype.Kubernetes, nil
	}
	if series == genericLinuxSeries {
		return ostype.GenericLinux, nil
	}

	return ostype.Unknown, errors.Trace(unknownOSForSeriesError(series))
}

var (
	logger = loggo.GetLogger("juju.juju.base")

	seriesVersionsMutex sync.Mutex
)

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
