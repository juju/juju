// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"strings"

	"github.com/juju/charm/v8"
	"github.com/juju/errors"
	osseries "github.com/juju/os/v2/series"

	commoncharm "github.com/juju/juju/api/common/charm"
	"github.com/juju/juju/core/arch"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/constraints"
	coreseries "github.com/juju/juju/core/series"
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
	// Additionally, `ResolveCharm` is not called for local charms, which are
	// simply uploaded and deployed. We satisfy the requirement for
	// non-empty platform architecture by making our best guess here.
	architecture := platform.Architecture
	if architecture == "" {
		architecture = arch.DefaultArchitecture
	}

	var origin commoncharm.Origin
	switch url.Schema {
	case "cs":
		origin = commoncharm.Origin{
			Source:       commoncharm.OriginCharmStore,
			Risk:         string(channel.Risk),
			Architecture: architecture,
			OS:           platform.OS,
			Series:       platform.Series,
		}
	case "local":
		origin = commoncharm.Origin{
			Source:       commoncharm.OriginLocal,
			Architecture: architecture,
			OS:           platform.OS,
			Series:       platform.Series,
		}
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
		origin = commoncharm.Origin{
			Source:       commoncharm.OriginCharmHub,
			Revision:     revision,
			Risk:         string(channel.Risk),
			Track:        track,
			Branch:       branch,
			Architecture: architecture,
			OS:           platform.OS,
			Series:       platform.Series,
		}
	}

	// If there is a series, ensure there is an OS.
	if origin.Series != "" && origin.OS == "" {
		os, err := coreseries.GetOSFromSeries(origin.Series)
		if err != nil {
			return commoncharm.Origin{}, err
		}
		origin.OS = strings.ToLower(os.String())
	}
	return origin, nil
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
