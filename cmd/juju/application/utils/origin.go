// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"github.com/juju/charm/v8"
	"github.com/juju/charmrepo/v6/csclient/params"
	"github.com/juju/errors"

	charm2 "github.com/juju/juju/api/common/charm"
	charm3 "github.com/juju/juju/core/charm"
)

func DeduceOrigin(url *charm.URL, channel params.Channel) (charm2.Origin, error) {
	if url == nil {
		return charm2.Origin{}, errors.NotValidf("charm url")
	}

	switch url.Schema {
	case "cs":
		return charm2.Origin{
			Source: charm2.OriginCharmStore,
			Risk:   string(channel),
		}, nil
	case "local":
		return charm2.Origin{
			Source: charm2.OriginLocal,
		}, nil
	default:
		chChannel, err := charm3.MakeChannel("", string(channel), "")
		if err != nil {
			return charm2.Origin{}, errors.Trace(err)
		}
		return charm2.Origin{
			Source: charm2.OriginCharmHub,
			Risk:   string(chChannel.Risk),
			Track:  &chChannel.Track,
		}, nil
	}
}
