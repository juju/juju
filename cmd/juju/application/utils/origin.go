// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"github.com/juju/charm/v8"
	"github.com/juju/charmrepo/v6/csclient/params"
	"github.com/juju/errors"

	commoncharm "github.com/juju/juju/api/common/charm"
	corecharm "github.com/juju/juju/core/charm"
)

func DeduceOrigin(url *charm.URL, channel params.Channel) (commoncharm.Origin, error) {
	if url == nil {
		return commoncharm.Origin{}, errors.NotValidf("charm url")
	}

	switch url.Schema {
	case "cs":
		return commoncharm.Origin{
			Source: commoncharm.OriginCharmStore,
			Risk:   string(channel),
		}, nil
	case "local":
		return commoncharm.Origin{
			Source: commoncharm.OriginLocal,
		}, nil
	default:
		if channel == "" {
			return commoncharm.Origin{Source: commoncharm.OriginCharmHub}, nil
		}
		chChannel, err := corecharm.MakeChannel("", string(channel), "")
		if err != nil {
			return commoncharm.Origin{}, errors.Trace(err)
		}
		return commoncharm.Origin{
			Source: commoncharm.OriginCharmHub,
			Risk:   string(chChannel.Risk),
			Track:  &chChannel.Track,
		}, nil
	}
}
