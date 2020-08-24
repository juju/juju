// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"github.com/juju/charm/v8"
	"github.com/juju/errors"

	apicharm "github.com/juju/juju/api/common/charm"
)

func deduceOrigin(url *charm.URL) (apicharm.Origin, error) {
	if url == nil {
		return apicharm.Origin{}, errors.NotValidf("charm url")
	}

	switch url.Schema {
	case "cs":
		return apicharm.Origin{
			Source: apicharm.OriginCharmStore,
		}, nil
	case "local":
		return apicharm.Origin{
			Source: apicharm.OriginLocal,
		}, nil
	default:
		return apicharm.Origin{
			Source: apicharm.OriginCharmHub,
		}, nil
	}
}
