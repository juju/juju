// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"fmt"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
)

// commonModelFacadeNames lists facades that are shared between CAAS
// and IAAS models.
var commonModelFacadeNames = set.NewStrings(
	"Action",
	"ActionPruner",
	"AllWatcher",
	"Agent",
	"Annotations",
	"Application",
	"Block",
	"CharmRevisionUpdater",
	"Charms",
	"Cleaner",
	"Client",
	"Cloud",
	"CredentialValidator",
	"CrossController",
	"CrossModelRelations",
	"ExternalControllerUpdater",
	"FilesystemAttachmentsWatcher",
	"LeadershipService",
	"LifeFlag",
	"Logger",
	"MeterStatus",
	"MigrationFlag",
	"MigrationMaster",
	"MigrationMinion",
	"MigrationStatusWatcher",
	"MigrationTarget",
	"ModelConfig",
	"ModelUpgrader",
	"NotifyWatcher",
	"OfferStatusWatcher",
	"Pinger",
	"ProxyUpdater",
	"Resources",
	"GetResource",
	"GetResourceInfo",
	"RelationStatusWatcher",
	"RelationUnitsWatcher",
	"ResourcesHookContext",
	"RemoteRelations",
	"Resumer",
	"RetryStrategy",
	"Singular",
	"StatusHistory",
	"Storage",
	"StorageProvisioner",
	"StringsWatcher",
	"Undertaker",
	"Uniter",
	"Upgrader",
	"VolumeAttachmentsWatcher",
	"RemoteRelationWatcher",
)

// caasModelFacadeNames lists facades that are only used with CAAS
// models.
var caasModelFacadeNames = set.NewStrings(
	"CAASAdmission",
	"CAASAgent",
	"CAASFirewaller",
	"CAASOperator",
	"CAASOperatorProvisioner",
	"CAASOperatorUpgrader",
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
