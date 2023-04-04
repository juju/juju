// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"github.com/juju/charm/v9"
	"github.com/juju/errors"

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
	// Legacy k8s charms - assume ubuntu focal.
	if platform.OS == coreseries.Kubernetes.String() || platform.Channel == coreseries.Kubernetes.String() {
		b := coreseries.LegacyKubernetesBase()
		platform.OS = b.OS
		platform.Channel = b.Channel.Track
	}
	switch url.Schema {
	case "cs":
		origin = commoncharm.Origin{
			Source:       commoncharm.OriginCharmStore,
			Risk:         string(channel.Risk),
			Architecture: architecture,
		}
	case "local":
		origin = commoncharm.Origin{
			Source:       commoncharm.OriginLocal,
			Architecture: architecture,
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
		}
	}
	if platform.OS != "" && platform.Channel != "" {
		base, err := coreseries.ParseBase(platform.OS, platform.Channel)
		if err != nil {
			return commoncharm.Origin{}, err
		}
		origin.Base = base
	}
	return origin, nil
}

// DeducePlatform attempts to create a Platform (architecture, os and series)
// from a set of constraints or a free style series.
func DeducePlatform(cons constraints.Value, series string, modelCons constraints.Value) (corecharm.Platform, error) {
	var os, channel string
	if series != "" {
		base, err := coreseries.GetBaseFromSeries(series)
		if err != nil {
			return corecharm.Platform{}, errors.Trace(err)
		}
		os = base.OS
		channel = base.Channel.Track
	}

	return corecharm.Platform{
		Architecture: arch.ConstraintArch(cons, &modelCons),
		OS:           os,
		Channel:      channel,
	}, nil
}
