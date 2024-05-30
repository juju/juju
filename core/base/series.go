// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package base

import (
	"strings"

	"github.com/juju/errors"
)

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
