// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package base

import (
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/os/v2/series"
)

// UbuntuDistroInfo is the path for the Ubuntu distro info file.
var UbuntuDistroInfo = series.UbuntuDistroInfo

// ControllerBases returns the supported workload bases available to it at the
// execution time.
func ControllerBases(now time.Time, requestedBase Base, imageStream string) ([]Base, error) {
	supported, err := supportedInfoForType(UbuntuDistroInfo, now, requestedBase, imageStream)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return supported.controllerBases(), nil
}

// WorkloadBases returns the supported workload bases available to it at the
// execution time.
func WorkloadBases(now time.Time, requestedBase Base, imageStream string) ([]Base, error) {
	supported, err := supportedInfoForType(UbuntuDistroInfo, now, requestedBase, imageStream)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return supported.workloadBases(false), nil
}

// AllWorkloadVersions returns all the workload versions (supported or not).
func AllWorkloadVersions() (set.Strings, error) {
	supported, err := supportedInfoForType(UbuntuDistroInfo, time.Now(), Base{}, "")
	if err != nil {
		return nil, errors.Trace(err)
	}
	return set.NewStrings(supported.workloadVersions(true)...), nil
}

// AllWorkloadOSTypes returns all the workload os types (supported or not).
func AllWorkloadOSTypes() (set.Strings, error) {
	supported, err := supportedInfoForType(UbuntuDistroInfo, time.Now(), Base{}, "")
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := set.NewStrings()
	for _, wBase := range supported.workloadBases(true) {
		result.Add(wBase.OS)
	}
	return result, nil
}

func supportedInfoForType(path string, now time.Time, requestedBase Base, imageStream string) (*supportedInfo, error) {
	// For non-LTS releases; they'll appear in juju/os as default available, but
	// after reading the `/usr/share/distro-info/ubuntu.csv` on the Ubuntu distro
	// the non-LTS should disappear if they're not in the release window for that
	// series.
	seriesVersionsMutex.Lock()
	defer seriesVersionsMutex.Unlock()
	updateSeriesVersionsOnce()
	all := getAllSeriesVersions()
	if !requestedBase.Empty() && imageStream == Daily {
		setSupported(all, requestedBase)
	}
	source := series.NewDistroInfo(path)
	supported := newSupportedInfo(source, all)
	if err := supported.compile(now); err != nil {
		return nil, errors.Trace(err)
	}

	return supported, nil
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
