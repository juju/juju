// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package series

import (
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/os/series"
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

// ControllerSeries returns all the controller series available to it at the
// execution time.
func ControllerSeries(now time.Time, requestedSeries, imageStream string) (set.Strings, error) {
	supported, err := seriesForTypes(series.UbuntuDistroInfo, now, requestedSeries, imageStream)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return set.NewStrings(supported.ControllerSeries()...), nil
}

// WorkloadSeries returns all the workload series available to it at the
// execution time.
func WorkloadSeries(now time.Time, requestedSeries, imageStream string) (set.Strings, error) {
	supported, err := seriesForTypes(series.UbuntuDistroInfo, now, requestedSeries, imageStream)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return set.NewStrings(supported.WorkloadSeries()...), nil
}

func seriesForTypes(path string, now time.Time, requestedSeries, imageStream string) (*SupportedInfo, error) {
	// We support all of the juju series AND all the ESM supported series.
	// Juju is congruent with the Ubuntu release cycle for it's own series (not
	// including centos and windows), so that should be reflected here.
	//
	// For non-LTS releases; they'll appear in juju/os as default available, but
	// after reading the `/usr/share/distro-info/ubuntu.csv` on the Ubuntu distro
	// the non-LTS should disapear if they're not in the release window for that
	// series.
	defaultSeries := DefaultSeries()
	if requestedSeries != "" && imageStream == Daily {
		SetSupported(defaultSeries, requestedSeries)
	}

	source := series.NewDistroInfo(path)
	supported := NewSupportedInfo(source, defaultSeries)
	if err := supported.Compile(now); err != nil {
		return nil, errors.Trace(err)
	}

	return supported, nil
}
