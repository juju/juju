// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrations

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate mockgen -package migrations -destination offerconntections_mock_test.go github.com/juju/juju/state/migrations MigrationOfferConnection,AllOfferConnectionSource,OfferConnectionSource,OfferConnectionModel
//go:generate mockgen -package migrations -destination relationetworks_mock_test.go github.com/juju/juju/state/migrations MigrationRelationNetworks,RelationNetworksSource,RelationNetworksModel
//go:generate mockgen -package migrations -destination remoteapplications_mock_test.go github.com/juju/juju/state/migrations MigrationRemoteApplication,AllRemoteApplicationSource,StatusSource,RemoteApplicationSource,RemoteApplicationModel
//go:generate mockgen -package migrations -destination remoteentities_mock_test.go github.com/juju/juju/state/migrations MigrationRemoteEntity,RemoteEntitiesSource,RemoteEntitiesModel
//go:generate mockgen -package migrations -destination description_mock_test.go github.com/juju/description OfferConnection,RemoteEntity,RelationNetwork,RemoteApplication,RemoteSpace,Status
//go:generate mockgen -package migrations -destination firewallrules_mock_test.go github.com/juju/juju/state/migrations MigrationFirewallRule,FirewallRuleSource,FirewallRulesModel

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
