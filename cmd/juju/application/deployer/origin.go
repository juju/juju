// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"github.com/juju/charm/v7"
	"github.com/juju/errors"

	"github.com/juju/juju/api/application"
)

func deduceOrigin(url *charm.URL) (application.CharmOrigin, error) {
	if url == nil {
		return application.CharmOrigin{}, errors.NotValidf("charm url")
	}

	switch url.Schema {
	case "cs":
		return application.CharmOrigin{
			Source: application.OriginCharmStore,
		}, nil
	case "local":
		return application.CharmOrigin{
			Source: application.OriginLocal,
		}, nil
	default:
		return application.CharmOrigin{
			Source: application.OriginCharmHub,
		}, nil
	}
}
