// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"github.com/juju/juju/state"
)

// migrateJobManageNetworking adds the job JobManageNetworking to all
// machines except for:
//
// - machines in a MAAS environment,
// - machines in a manual environment,
// - bootstrap node (host machine) in a local environment, and
// - manually provisioned machines.
func migrateJobManageNetworking(context Context) error {
	logger.Debugf("migrating machine jobs into ones with JobManageNetworking based on rules")

	return state.MigrateJobManageNetworking(context.State())
}
