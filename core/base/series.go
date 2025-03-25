// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package base

import (
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
)

// seriesBaseMapping is a hard-coded set of pairs
// of equivalent series and bases. We use this to
// convert a base into it's equivalent series
var seriesBaseMapping = []struct {
	base   Base
	series string
}{{
	base:   MakeDefaultBase(UbuntuOS, "20.04"),
	series: "focal",
}, {
	base:   MakeDefaultBase(UbuntuOS, "20.10"),
	series: "groovy",
}, {
	base:   MakeDefaultBase(UbuntuOS, "21.04"),
	series: "hirsute",
}, {
	base:   MakeDefaultBase(UbuntuOS, "21.10"),
	series: "impish",
}, {
	base:   MakeDefaultBase(UbuntuOS, "22.04"),
	series: "jammy",
}, {
	base:   MakeDefaultBase(UbuntuOS, "22.10"),
	series: "kinetic",
}, {
	base:   MakeDefaultBase(UbuntuOS, "23.04"),
	series: "lunar",
}, {
	base:   MakeDefaultBase(UbuntuOS, "23.10"),
	series: "mantic",
}, {
	base:   MakeDefaultBase(UbuntuOS, "24.04"),
	series: "noble",
}, {
	base:   MakeDefaultBase(UbuntuOS, "24.10"),
	series: "oracular",
}}

// GetSeriesFromBase returns the series name for a
// given Base. This is needed to support legacy series.
func GetSeriesFromBase(v Base) (string, error) {
	for _, pair := range seriesBaseMapping {
		if v.IsCompatible(pair.base) {
			return pair.series, nil
		}
	}
	return "", errors.Errorf("os %q version %q %w", v.OS, v.Channel.Track, coreerrors.NotFound)
}
