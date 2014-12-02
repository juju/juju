// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/utils"
)

func addAvaililityZoneToInstanceData(context Context) error {
	err := state.AddAvailabilityZoneToInstanceData(
		context.State(),
		utils.AvailabilityZone,
	)
	return errors.Trace(err)
}
