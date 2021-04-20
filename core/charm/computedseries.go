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
	if Format(c) < FormatV2 {
		return c.Meta().Series, nil
	}

	// If we have V2 metadata *and* a non-empty containers collection,
	// then this is a side-car based charm and we return "kubernetes"
	// instead of translating the collection of supplied bases.
	if IsKubernetes(c) {
		return []string{coreseries.Kubernetes.String()}, nil
	}

	// We use a set to ensure uniqueness and a slice to ensure that we
	// preserve the order of elements as they appear in the manifest.
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
