// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"github.com/juju/charm/v8"
	"github.com/juju/collections/set"
	"github.com/juju/errors"

	coreseries "github.com/juju/juju/core/series"
)

// ComputedSeries of a charm, preserving legacy behavior.  If the charm has
// no manifest, return series from the metadata. Otherwise return the series
// listed in the manifest Bases as channels.
func ComputedSeries(c charm.CharmMeta) ([]string, error) {
	manifest := c.Manifest()
	if manifest == nil || len(manifest.Bases) == 0 {
		return c.Meta().Series, nil
	}
	// The slice must be ordered based on system appearance but
	// have unique elements.
	seriesSlice := []string(nil)
	seriesSet := set.NewStrings()
	for _, base := range manifest.Bases {
		version := base.Channel.Track
		series, err := coreseries.VersionSeries(version)
		if err != nil {
			return []string{}, errors.Trace(err)
		}
		if !seriesSet.Contains(series) {
			seriesSet.Add(series)
			seriesSlice = append(seriesSlice, series)
		}
	}
	return seriesSlice, nil
}
