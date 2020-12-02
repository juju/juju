// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"strings"

	"github.com/juju/charm/v8"
	"github.com/juju/errors"
	osseries "github.com/juju/os/series"

	commoncharm "github.com/juju/juju/api/common/charm"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/constraints"
)

// DeduceOrigin attempts to deduce the origin from a channel and a platform.
// Depending on what the charm URL schema is, will then construct the correct
// origin for that application.
func DeduceOrigin(url *charm.URL, channel corecharm.Channel, platform corecharm.Platform) (commoncharm.Origin, error) {
	if url == nil {
		return commoncharm.Origin{}, errors.NotValidf("charm url")
	}

	switch url.Schema {
	case "cs":
		return commoncharm.Origin{
			Source: commoncharm.OriginCharmStore,
			Risk:   string(channel.Risk),
			Series: platform.Series,
		}, nil
	case "local":
		return commoncharm.Origin{
			Source: commoncharm.OriginLocal,
		}, nil
	default:
		var track *string
		if channel.Track != "" {
			track = &channel.Track
		}
		return commoncharm.Origin{
			Source:       commoncharm.OriginCharmHub,
			Risk:         string(channel.Risk),
			Track:        track,
			Architecture: platform.Architecture,
			OS:           platform.OS,
			Series:       platform.Series,
		}, nil
	}
}

// DeducePlatform attempts to create a Platform (architecture, os and series)
// from a set of constraints or a free style series.
func DeducePlatform(cons constraints.Value, series string) (corecharm.Platform, error) {
	var arch string
	if cons.HasArch() {
		arch = *cons.Arch
	}

	var os string
	if series != "" {
		sys, err := osseries.GetOSFromSeries(series)
		if err != nil {
			return corecharm.Platform{}, errors.Trace(err)
		}
		os = strings.ToLower(sys.String())
	}

	return corecharm.Platform{
		Architecture: arch,
		OS:           os,
		Series:       series,
	}, nil
}
