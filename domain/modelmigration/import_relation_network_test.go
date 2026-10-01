// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration_test

import (
	"github.com/juju/clock"
	"github.com/juju/description/v12"
	"github.com/juju/tc"
	"gopkg.in/macaroon.v2"

	cmrstate "github.com/juju/juju/domain/crossmodelrelation/state/model"
	migrationtesting "github.com/juju/juju/domain/modelmigration/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

// TestRelationEgressOverride checks the 3.6 --via policy on a consuming model.
// Import must persist the per-relation CIDRs even when the model has a different
// egress-subnets default. Workers cannot reconstruct this administrator-supplied
// policy from unit addresses after migration.
func (s *legacyImportSuite) TestRelationEgressOverride(c *tc.C) {
	desc, coordinator, scope := s.setupImport(c)
	desc.UpdateConfig(map[string]any{"egress-subnets": "203.0.113.0/24"})
	addLegacyApplication(desc, "client")
	mac, err := macaroon.New([]byte("root-key"), []byte("offer"), "source", macaroon.LatestVersion)
	c.Assert(err, tc.ErrorIsNil)
	macJSON, err := mac.MarshalJSON()
	c.Assert(err, tc.ErrorIsNil)
	remote := desc.AddRemoteApplication(description.RemoteApplicationArgs{
		Name: "database", OfferUUID: "cfa46843-ebf2-4fff-8519-c1fb5a9816f3",
		SourceModelUUID: "4ddd6454-931d-4278-8779-b0b7208994d9",
		URL:             "source:admin/production.database", Macaroon: string(macJSON),
	})
	remote.AddEndpoint(description.RemoteEndpointArgs{Name: "db", Role: "provider", Interface: "db"})
	remote.SetStatus(description.StatusArgs{Value: "active"})
	const relationKey = "client:db database:db"
	const relationToken = "6049aa01-76c9-462d-8440-964a6e26aac2"
	rel := desc.AddRelation(description.RelationArgs{Id: 42, Key: relationKey})
	rel.AddEndpoint(description.EndpointArgs{ApplicationName: "client", Name: "db", Role: "requirer", Interface: "db"})
	rel.AddEndpoint(description.EndpointArgs{ApplicationName: "database", Name: "db", Role: "provider", Interface: "db"})
	rel.SetStatus(description.StatusArgs{Value: "joined"})
	desc.AddRemoteEntity(description.RemoteEntityArgs{ID: "application-database", Token: "13ea2791-5e78-40d8-88c5-e9451444b45d"})
	desc.AddRemoteEntity(description.RemoteEntityArgs{ID: "relation-client.db#database.db", Token: relationToken})
	desc.SetSequence("relation", 43)
	cidrs := []string{"198.51.100.0/25", "192.0.2.0/24"}
	desc.AddRelationNetwork(description.RelationNetworkArgs{
		ID: relationKey + ":egress:override", RelationKey: relationKey, CIDRS: cidrs,
	})
	c.Assert(coordinator.Perform(c.Context(), scope, desc), tc.ErrorIsNil)
	state := cmrstate.NewState(scope.ModelDB(), scope.ModelUUID(), clock.WallClock, loggertesting.WrapCheckLog(c))
	imported, err := state.GetRelationNetworkEgress(c.Context(), relationToken)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(imported, tc.SameContents, cidrs)
}

func addLegacyApplication(desc description.Model, name string) description.Application {
	app := desc.AddApplication(description.ApplicationArgs{Name: name, CharmURL: "ch:" + name + "-1"})
	app.SetCharmOrigin(description.CharmOriginArgs{
		Source: "charm-hub", ID: "deadbeef", Hash: "deadbeef2", Revision: 1,
		Channel: "latest/stable", Platform: "amd64/ubuntu/22.04",
	})
	app.SetCharmMetadata(description.CharmMetadataArgs{
		Name: name, Requires: map[string]description.CharmMetadataRelation{
			"db": migrationtesting.Relation{Name_: "db", Role_: "requirer", InterfaceName_: "db", Scope_: "global"},
		},
	})
	app.SetCharmManifest(description.CharmManifestArgs{Bases: []description.CharmManifestBase{
		migrationtesting.ManifestBase{Name_: "ubuntu", Channel_: "22.04/stable", Architectures_: []string{"amd64"}},
	}})
	app.SetStatus(description.StatusArgs{Value: "active"})
	return app
}
