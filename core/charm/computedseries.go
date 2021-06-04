// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"github.com/juju/charm/v9"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"

	coreseries "github.com/juju/juju/core/series"
)

var logger = loggo.GetLogger("juju.core.charm")

// ComputedSeries of a charm, preserving legacy behavior.  If the charm has
// no manifest, return series from the metadata. Otherwise return the series
// listed in the manifest Bases as channels.
func ComputedSeries(c charm.CharmMeta) (seriesSlice []string, err error) {
	format := Format(c)
	isKubernetes := IsKubernetes(c)
	defer logger.Debugf("resolved series %v for charm %q with format %v, Kubernetes %v", seriesSlice, c.Meta().Name, format, isKubernetes)

	if format < FormatV2 {
		return c.Meta().Series, nil
	}

	// We use a set to ensure uniqueness and a slice to ensure that we
	// preserve the order of elements as they appear in the manifest.
	seriesSet := set.NewStrings()

	manifest := c.Manifest()
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
	if isKubernetes && !seriesSet.Contains(coreseries.Kubernetes.String()) {
		seriesSlice = append(seriesSlice, coreseries.Kubernetes.String())
	}

	return seriesSlice, nil
}
