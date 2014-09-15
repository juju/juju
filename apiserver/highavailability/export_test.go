// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package highavailability

import "github.com/juju/names"

func EnvironTag(haServer *HighAvailabilityAPI) names.EnvironTag {
	return haServer.state.EnvironTag()
}
