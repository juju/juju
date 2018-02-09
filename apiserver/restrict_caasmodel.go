// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/utils/set"
)

// commonModelFacadeNames lists facades that are shared between CAAS
// and IAAS models.
var commonModelFacadeNames = set.NewStrings(
	"ActionPruner",
	"Agent",
	"Application",
	"CharmRevisionUpdater",
	"Charms",
	"Cleaner",
	"Client",
	"Cloud",
	"LeadershipService",
	"LifeFlag",
	"MigrationFlag",
	"MigrationMaster",
	"MigrationMinion",
	"MigrationStatusWatcher",
	"MigrationTarget",
	"ModelConfig",
	"ModelUpgrader",
	"NotifyWatcher",
	"Pinger",
	"RelationUnitsWatcher",
	"RemoteRelations",
	"RetryStrategy",
	"Singular",
	"StatusHistory",
	"StringsWatcher",
	"Uniter",
)

// caasModelFacadeNames lists facades that are only used with CAAS
// models.
var caasModelFacadeNames = set.NewStrings(
	"CAASFirewaller",
	"CAASOperator",
	"CAASOperatorProvisioner",
	"CAASUnitProvisioner",
)

func caasModelFacadesOnly(facadeName, _ string) error {
	if !isCAASModelFacade(facadeName) {
		return errors.NewNotSupported(nil, fmt.Sprintf("facade %q not supported for a CAAS model API connection", facadeName))
	}
	return nil
}

// isCAASModelFacade reports whether the given facade name can be accessed
// using the controller connection.
func isCAASModelFacade(facadeName string) bool {
	return caasModelFacadeNames.Contains(facadeName) ||
		commonModelFacadeNames.Contains(facadeName) ||
		commonFacadeNames.Contains(facadeName)
}
