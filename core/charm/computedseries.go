// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"strings"

	"github.com/juju/charm/v8"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"

	coreseries "github.com/juju/juju/core/series"
)

var logger = loggo.GetLogger("juju.core.charm")

// ComputedSeries of a charm, preserving legacy behavior.  If the charm has
// no manifest, return series from the metadata. Otherwise return the series
// listed in the manifest Bases as channels.
func ComputedSeries(c charm.CharmMeta) (seriesSlice []string, _ error) {
	format := Format(c)
	isKubernetes := IsKubernetes(c)
	meta := c.Meta()
	defer func(s *[]string) {
		logger.Debugf("series %q for charm %q with format %v, Kubernetes %v", strings.Join(*s, ", "), meta.Name, format, isKubernetes)
	}(&seriesSlice)

	if format < FormatV2 {
		return meta.Series, nil
	}

	// We use a set to ensure uniqueness and a slice to ensure that we
	// preserve the order of elements as they appear in the manifest.
	seriesSet := set.NewStrings()

	manifest := c.Manifest()
	if manifest != nil {
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
	}
	if isKubernetes && !seriesSet.Contains(coreseries.Kubernetes.String()) {
		seriesSlice = append(seriesSlice, coreseries.Kubernetes.String())
	}

	return seriesSlice, nil
}
