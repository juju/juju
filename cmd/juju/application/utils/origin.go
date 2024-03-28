// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"github.com/juju/charm/v12"
	"github.com/juju/errors"

	commoncharm "github.com/juju/juju/api/common/charm"
	"github.com/juju/juju/core/arch"
	corebase "github.com/juju/juju/core/base"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/constraints"
)

// MakeOrigin creates an origin from a schema, revision, channel and a platform.
// Depending on what the schema is, will then construct the correct
// origin for that application.
func MakeOrigin(schema charm.Schema, revision int, channel charm.Channel, platform corecharm.Platform) (commoncharm.Origin, error) {
	// Arch is ultimately determined for non-local cases in the API call
	// to `ResolveCharm`. To ensure we always have an architecture, even if
	// somehow the MakePlatform doesn't find one fill one in.
	// Additionally, `ResolveCharm` is not called for local charms, which are
	// simply uploaded and deployed. We satisfy the requirement for
	// non-empty platform architecture by making our best guess here.
	architecture := platform.Architecture
	if architecture == "" {
		architecture = arch.DefaultArchitecture
	}

	var origin commoncharm.Origin
	// Legacy k8s charms - assume ubuntu focal.
	if platform.OS == corebase.Kubernetes.String() || platform.Channel == corebase.Kubernetes.String() {
		b := corebase.LegacyKubernetesBase()
		platform.OS = b.OS
		platform.Channel = b.Channel.Track
	}
	switch schema {
	case charm.Local:
		origin = commoncharm.Origin{
			Source:       commoncharm.OriginLocal,
			Architecture: architecture,
		}
	case charm.CharmHub:
		var track *string
		if channel.Track != "" {
			track = &channel.Track
		}
		var branch *string
		if channel.Branch != "" {
			branch = &channel.Branch
		}
		origin = commoncharm.Origin{
			Source:       commoncharm.OriginCharmHub,
			Risk:         string(channel.Risk),
			Track:        track,
			Branch:       branch,
			Architecture: architecture,
		}
	default:
		return commoncharm.Origin{}, errors.NotSupportedf("charm source %q", schema)
	}
	if revision >= 0 {
		origin.Revision = &revision
	}
	if platform.OS != "" && platform.Channel != "" {
		base, err := corebase.ParseBase(platform.OS, platform.Channel)
		if err != nil {
			return commoncharm.Origin{}, err
		}
		origin.Base = base
	}
	return origin, nil
}

// MakePlatform creates a Platform (architecture, os and base) from a set of
// constraints and a base.
func MakePlatform(cons constraints.Value, base corebase.Base, modelCons constraints.Value) corecharm.Platform {
	return corecharm.Platform{
		Architecture: constraints.ArchOrDefault(cons, &modelCons),
		OS:           base.OS,
		Channel:      base.Channel.Track,
	}
}
