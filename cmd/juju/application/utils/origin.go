// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"github.com/juju/errors"

	commoncharm "github.com/juju/juju/api/common/charm"
	corebase "github.com/juju/juju/core/base"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/constraints"
)

// MakeOrigin creates an origin from a schema, revision, channel and a platform.
// Depending on what the schema is, will then construct the correct
// origin for that application.
func MakeOrigin(schema Schema, revision int, channel Channel, platform corecharm.Platform) (commoncharm.Origin, error) {

	var origin commoncharm.Origin
	switch schema {
	case Local:
		origin = commoncharm.Origin{
			Source:       commoncharm.OriginLocal,
			Architecture: platform.Architecture,
		}
	case CharmHub:
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
			Risk:         channel.Risk,
			Track:        track,
			Branch:       branch,
			Architecture: platform.Architecture,
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
		Architecture: constraints.ArchOrDefault(cons, modelCons),
		OS:           base.OS,
		Channel:      base.Channel.Track,
	}
}
