// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package base

import (
	"sync"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	jujuos "github.com/juju/os/v2"
	"github.com/juju/os/v2/series"
)

const (
	// Daily defines if a image-stream is set to this, then you get a different
	// set of logic. In this case if you want to test drive new releases, it's
	// required that the image-stream modelconfig is set from released to
	// daily.
	Daily = "daily"
)

// SupportedSeriesFunc describes a function that has commonality between
// controller and workload types.
type SupportedSeriesFunc = func(time.Time, string, string) (set.Strings, error)

func getAllSeriesVersions() map[SeriesName]seriesVersion {
	copy := make(map[SeriesName]seriesVersion, len(allSeriesVersions))
	for name, v := range allSeriesVersions {
		copy[name] = v
	}
	return copy
}

const (
	genericLinuxSeries  = "genericlinux"
	genericLinuxOS      = "genericlinux"
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
				OS:                       UbuntuOS,
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
	allSeriesVersions[genericLinuxSeries] = seriesVersion{
		WorkloadType: OtherWorkloadType,
		OS:           genericLinuxOS,
		Version:      genericLinuxVersion,
		Supported:    true,
	}
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
