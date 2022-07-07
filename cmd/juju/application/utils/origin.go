// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"strings"

	"github.com/juju/charm/v9"
	"github.com/juju/errors"
	"github.com/juju/juju/core/arch"
	osseries "github.com/juju/os/v2/series"

	commoncharm "github.com/juju/juju/api/common/charm"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/constraints"
)

// DeduceOrigin attempts to deduce the origin from a channel and a platform.
// Depending on what the charm URL schema is, will then construct the correct
// origin for that application.
func DeduceOrigin(url *charm.URL, channel charm.Channel, platform corecharm.Platform) (commoncharm.Origin, error) {
	if url == nil {
		return commoncharm.Origin{}, errors.NotValidf("charm url")
	}

	// Arch is ultimately determined for non-local cases in the API call
	// to `ResolveCharm`. To ensure we always have an architecture, even if
	// somehow the DeducePlatform doesn't find one fill one in.
	// Additionally `ResolveCharm` is not called for local charms, which are
	// simply uploaded and deployed. We satisfy the requirement for
	// non-empty platform architecture by making our best guess here.
	architecture := platform.Architecture
	if architecture == "" {
		architecture = arch.DefaultArchitecture
	}

	switch url.Schema {
	case "cs":
		return commoncharm.Origin{
			Source:       commoncharm.OriginCharmStore,
			Risk:         string(channel.Risk),
			Architecture: architecture,
			Series:       platform.Series,
		}, nil
	case "local":
		return commoncharm.Origin{
			Source:       commoncharm.OriginLocal,
			Architecture: architecture,
			OS:           platform.OS,
			Series:       platform.Series,
		}, nil
	default:
		var track *string
		if channel.Track != "" {
			track = &channel.Track
		}
		var branch *string
		if channel.Branch != "" {
			branch = &channel.Branch
		}
		var revision *int
		if url.Revision != -1 {
			revision = &url.Revision
		}
		return commoncharm.Origin{
			Source:       commoncharm.OriginCharmHub,
			Revision:     revision,
			Risk:         string(channel.Risk),
			Track:        track,
			Branch:       branch,
			Architecture: architecture,
			OS:           platform.OS,
			Series:       platform.Series,
		}, nil
	}
}

// DeducePlatform attempts to create a Platform (architecture, os and series)
// from a set of constraints or a free style series.
func DeducePlatform(cons constraints.Value, series string, modelCons constraints.Value) (corecharm.Platform, error) {
	var os string
	if series != "" {
		sys, err := osseries.GetOSFromSeries(series)
		if err != nil {
			return corecharm.Platform{}, errors.Trace(err)
		}
		os = strings.ToLower(sys.String())
	}

	return corecharm.Platform{
		Architecture: arch.ConstraintArch(cons, &modelCons),
		OS:           os,
		Series:       series,
	}, nil
}
