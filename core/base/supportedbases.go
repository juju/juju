// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package base

import (
	"github.com/juju/errors"
)

// ControllerBases returns the supported workload bases available to it at the
// execution time.
func ControllerBases() []Base {
	return []Base{
		MakeDefaultBase("ubuntu", "22.04"),
		MakeDefaultBase("ubuntu", "20.04"),
	}
}

// WorkloadBases returns the supported workload bases available to it at the
// execution time.
func WorkloadBases() []Base {
	return []Base{
		MakeDefaultBase("ubuntu", "23.10"),
		MakeDefaultBase("ubuntu", "23.04"),
		MakeDefaultBase("ubuntu", "22.10"),
		MakeDefaultBase("ubuntu", "22.04"),
		MakeDefaultBase("ubuntu", "21.10"),
		MakeDefaultBase("ubuntu", "21.04"),
		MakeDefaultBase("ubuntu", "20.10"),
		MakeDefaultBase("ubuntu", "20.04"),
	}
}

// BaseSeriesVersion returns the series version for the given base.
// TODO(stickupkid): The underlying series version should be a base, until that
// logic has changes, just convert between base and series.
func BaseSeriesVersion(base Base) (string, error) {
	s, err := GetSeriesFromBase(base)
	if err != nil {
		return "", errors.Trace(err)
	}
	return SeriesVersion(s)
}

// UbuntuBaseVersion returns the series version for the given base.
// TODO(stickupkid): The underlying series version should be a base, until that
// logic has changes, just convert between base and series.
func UbuntuBaseVersion(base Base) (string, error) {
	s, err := GetSeriesFromBase(base)
	if err != nil {
		return "", errors.Trace(err)
	}
	return UbuntuSeriesVersion(s)
}

// LatestLTSBase returns the latest LTS base.
// TODO(stickupkid): The underlying series version should be a base, until that
// logic has changes, just convert between base and series.
func LatestLTSBase() Base {
	lts := LatestLTS()
	b, err := GetBaseFromSeries(lts)
	if err != nil {
		panic(err)
	}
	return b
}
