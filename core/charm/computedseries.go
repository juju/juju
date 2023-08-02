// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"strings"

	"github.com/juju/charm/v11"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"

	corebase "github.com/juju/juju/core/base"
)

var logger = loggo.GetLogger("juju.core.charm")

// SeriesForCharm takes a requested series and a list of series supported by a
// charm and returns the series which is relevant.
// If the requested series is empty, then the first supported series is used,
// otherwise the requested series is validated against the supported series.
func SeriesForCharm(requestedSeries string, supportedSeries []string) (string, error) {
	// Old charm with no supported series.
	if len(supportedSeries) == 0 {
		if requestedSeries == "" {
			return "", errMissingSeries
		}
		return requestedSeries, nil
	}
	// Use the charm default.
	if requestedSeries == "" {
		return supportedSeries[0], nil
	}
	for _, s := range supportedSeries {
		if s == requestedSeries {
			return requestedSeries, nil
		}
	}
	return "", NewUnsupportedSeriesError(requestedSeries, supportedSeries)
}

// errMissingSeries is used to denote that SeriesForCharm could not determine
// a series because a legacy charm did not declare any.
var errMissingSeries = errors.New("series not specified and charm does not define any")

// IsMissingSeriesError returns true if err is an errMissingSeries.
func IsMissingSeriesError(err error) bool {
	return err == errMissingSeries
}

// ComputedSeries of a charm, preserving legacy behavior.  If the charm has
// no manifest, return series from the metadata. Otherwise, return the series
// listed in the manifest Bases as channels.
func ComputedSeries(c charm.CharmMeta) (seriesSlice []string, _ error) {
	format := charm.MetaFormat(c)
	meta := c.Meta()
	defer func(s *[]string) {
		logger.Debugf("series %q for charm %q with format %v", strings.Join(*s, ", "), meta.Name, format)
	}(&seriesSlice)

	manifest := c.Manifest()
	if manifest != nil {
		// We use a set to ensure uniqueness and a slice to ensure that we
		// preserve the order of elements as they appear in the manifest.
		seriesSet := set.NewStrings()
		for _, base := range manifest.Bases {
			series, err := corebase.GetSeriesFromChannel(base.Name, base.Channel.Track)
			if err != nil {
				return []string{}, errors.Trace(err)
			}
			if !seriesSet.Contains(series) {
				seriesSet.Add(series)
				seriesSlice = append(seriesSlice, series)
			}
		}
	} else if manifest == nil && format < charm.FormatV2 {
		seriesSlice = meta.Series
	}

	return seriesSlice, nil
}
