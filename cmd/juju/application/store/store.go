// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package store

import (
	"github.com/juju/charm/v11"
	"github.com/juju/errors"

	commoncharm "github.com/juju/juju/api/common/charm"
	"github.com/juju/juju/rpc/params"
)

// AddCharmFromURL calls the appropriate client API calls to add the
// given charm URL to state.
func AddCharmFromURL(client CharmAdder, curl *charm.URL, origin commoncharm.Origin, force bool) (*charm.URL, commoncharm.Origin, error) {
	resultOrigin, err := client.AddCharm(curl, origin, force)
	if err != nil {
		if params.IsCodeUnauthorized(err) {
			return nil, commoncharm.Origin{}, errors.Forbiddenf(err.Error())
		}
		return nil, commoncharm.Origin{}, errors.Trace(err)
	}
	return curl, resultOrigin, nil
}
