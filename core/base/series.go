// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package base

import (
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/core/os/ostype"
)

// GetBaseFromSeries returns the Base infor for a series.
func GetBaseFromSeries(series string) (Base, error) {
	var result Base
	osName, err := getOSFromSeries(series)
	if err != nil {
		return result, errors.NotValidf("series %q", series)
	}
	osVersion, err := getSeriesVersion(series)
	if err != nil {
		return result, errors.NotValidf("series %q", series)
	}
	result.OS = strings.ToLower(osName.String())
	result.Channel = MakeDefaultChannel(osVersion)
	return result, nil
}

// GetSeriesFromBase returns the series name for a
// given Base. This is needed to support legacy series.
func GetSeriesFromBase(v Base) (string, error) {
	var osSeries map[SeriesName]seriesVersion
	switch strings.ToLower(v.OS) {
	case UbuntuOS:
		osSeries = ubuntuSeries
	case CentosOS:
		osSeries = centosSeries
	}
	for s, vers := range osSeries {
		if vers.Version == v.Channel.Track {
			return string(s), nil
		}
	}
	return "", errors.NotFoundf("os %q version %q", v.OS, v.Channel.Track)
}

// getOSFromSeries will return the operating system based
// on the series that is passed to it
func getOSFromSeries(series string) (ostype.OSType, error) {
	if series == "" {
		return ostype.Unknown, errors.NotValidf("series %q", series)
	}

	seriesVersionsMutex.Lock()
	defer seriesVersionsMutex.Unlock()

	seriesName := SeriesName(series)
	osType, err := lookupOSType(seriesName)
	if err == nil {
		return osType, nil
	}

	updateSeriesVersionsOnce()
	return lookupOSType(seriesName)
}

func lookupOSType(series SeriesName) (ostype.OSType, error) {
	if _, ok := ubuntuSeries[series]; ok {
		return ostype.Ubuntu, nil
	}
	if _, ok := centosSeries[series]; ok {
		return ostype.CentOS, nil
	}
	if series == genericLinuxSeries {
		return ostype.GenericLinux, nil
	}

	return ostype.Unknown, errors.Trace(unknownOSForSeriesError(series))
}

func getSeriesVersion(series string) (string, error) {
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
