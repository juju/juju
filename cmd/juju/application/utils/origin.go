// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"github.com/juju/charm/v8"
	"github.com/juju/errors"

	commoncharm "github.com/juju/juju/api/common/charm"
	corecharm "github.com/juju/juju/core/charm"
)

func DeduceOrigin(url *charm.URL, channel corecharm.Channel) (commoncharm.Origin, error) {
	if url == nil {
		return commoncharm.Origin{}, errors.NotValidf("charm url")
	}

	switch url.Schema {
	case "cs":
		return commoncharm.Origin{
			Source: commoncharm.OriginCharmStore,
			Risk:   string(channel.Risk),
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
			Source: commoncharm.OriginCharmHub,
			Risk:   string(channel.Risk),
			Track:  track,
		}, nil
	}
}
