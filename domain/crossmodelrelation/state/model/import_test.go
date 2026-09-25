// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"testing"

	"github.com/juju/tc"

	coreapplication "github.com/juju/juju/core/application"
	corerelation "github.com/juju/juju/core/relation"
	appcharm "github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/crossmodelrelation"
	"github.com/juju/juju/domain/crossmodelrelation/internal"
	internalcharm "github.com/juju/juju/domain/deployment/charm"
	"github.com/juju/juju/domain/life"
	domainsecret "github.com/juju/juju/domain/secret"
	internaluuid "github.com/juju/juju/internal/uuid"
)

type importOfferSuite struct {
	baseSuite
}

func TestImportOfferSuite(t *testing.T) {
	tc.Run(t, &importOfferSuite{})
}

func (s *importOfferSuite) TestImportOffers(c *tc.C) {
	// Arrange
	charmUUID := s.addCharm(c)
	s.addCharmMetadata(c, charmUUID, false)
	relation := internalcharm.Relation{
		Name:      "db",
		Role:      internalcharm.RoleProvider,
		Interface: "db",
		Scope:     internalcharm.ScopeGlobal,
	}
	relationUUID := s.addCharmRelation(c, charmUUID, relation)
	relation2 := internalcharm.Relation{
		Name:      "log",
		Role:      internalcharm.RoleProvider,
		Interface: "log",
		Scope:     internalcharm.ScopeGlobal,
	}
	relationUUID2 := s.addCharmRelation(c, charmUUID, relation2)
	relation3 := internalcharm.Relation{
		Name:      "public",
		Role:      internalcharm.RoleProvider,
		Interface: "public",
		Scope:     internalcharm.ScopeGlobal,
	}
	relationUUID3 := s.addCharmRelation(c, charmUUID, relation3)

	appName := "test-application"
	appUUID := s.addApplication(c, charmUUID, appName)
	endpointUUID := s.addApplicationEndpoint(c, appUUID, relationUUID)
	endpointUUID2 := s.addApplicationEndpoint(c, appUUID, relationUUID2)
	endpointUUID3 := s.addApplicationEndpoint(c, appUUID, relationUUID3)

	args := []crossmodelrelation.OfferImport{
		{
			UUID:            internaluuid.MustNewUUID(),
			ApplicationName: appName,
			Endpoints:       []string{relation.Name, relation2.Name},
			Name:            "test-offer",
		},
		{
			UUID:            internaluuid.MustNewUUID(),
			ApplicationName: appName,
			Endpoints:       []string{relation3.Name},
			Name:            "second",
		},
	}

	// Act
	err := s.state.ImportOffers(c.Context(), args)

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(err, tc.IsNil)
	obtainedOffers := s.readOffers(c)
	c.Check(obtainedOffers, tc.SameContents, []nameAndUUID{
		{
			UUID: args[0].UUID.String(),
			Name: args[0].Name,
		}, {
			UUID: args[1].UUID.String(),
			Name: args[1].Name,
		},
	})
	obtainedEndpoints := s.readOfferEndpoints(c)
	c.Check(obtainedEndpoints, tc.SameContents, []offerEndpoint{
		{
			OfferUUID:    args[0].UUID.String(),
			EndpointUUID: endpointUUID,
		}, {
			OfferUUID:    args[0].UUID.String(),
			EndpointUUID: endpointUUID2,
		}, {
			OfferUUID:    args[1].UUID.String(),
			EndpointUUID: endpointUUID3,
		},
	})
}

func (s *importOfferSuite) TestImportOffersMultipleApplications(c *tc.C) {
	// Arrange
	charmUUID1 := s.addCharm(c)
	s.addCharmMetadata(c, charmUUID1, false)
	relation1 := internalcharm.Relation{
		Name:      "db",
		Role:      internalcharm.RoleProvider,
		Interface: "mysql",
		Scope:     internalcharm.ScopeGlobal,
	}
	relationUUID1 := s.addCharmRelation(c, charmUUID1, relation1)
	appName1 := "app1"
	appUUID1 := s.addApplication(c, charmUUID1, appName1)
	endpointUUID1 := s.addApplicationEndpoint(c, appUUID1, relationUUID1)

	charmUUID2 := s.addCharm(c)
	s.addCharmMetadata(c, charmUUID2, false)
	relation2 := internalcharm.Relation{
		Name:      "endpoint",
		Role:      internalcharm.RoleProvider,
		Interface: "http",
		Scope:     internalcharm.ScopeGlobal,
	}
	relationUUID2 := s.addCharmRelation(c, charmUUID2, relation2)
	appName2 := "app2"
	appUUID2 := s.addApplication(c, charmUUID2, appName2)
	endpointUUID2 := s.addApplicationEndpoint(c, appUUID2, relationUUID2)

	args := []crossmodelrelation.OfferImport{
		{
			UUID:            internaluuid.MustNewUUID(),
			ApplicationName: appName1,
			Endpoints:       []string{relation1.Name},
			Name:            "offer1",
		},
		{
			UUID:            internaluuid.MustNewUUID(),
			ApplicationName: appName2,
			Endpoints:       []string{relation2.Name},
			Name:            "offer2",
		},
	}

	// Act
	err := s.state.ImportOffers(c.Context(), args)

	// Assert
	c.Assert(err, tc.IsNil)
	obtainedOffers := s.readOffers(c)
	c.Check(obtainedOffers, tc.SameContents, []nameAndUUID{
		{
			UUID: args[0].UUID.String(),
			Name: args[0].Name,
		}, {
			UUID: args[1].UUID.String(),
			Name: args[1].Name,
		},
	})
	obtainedEndpoints := s.readOfferEndpoints(c)
	c.Check(obtainedEndpoints, tc.SameContents, []offerEndpoint{
		{
			OfferUUID:    args[0].UUID.String(),
			EndpointUUID: endpointUUID1,
		}, {
			OfferUUID:    args[1].UUID.String(),
			EndpointUUID: endpointUUID2,
		},
	})
}

type importRemoteApplicationOfferersSuite struct {
	baseSuite
}

func TestImportRemoteApplicationOfferersSuite(t *testing.T) {
	tc.Run(t, &importRemoteApplicationOfferersSuite{})
}

func (s *importRemoteApplicationOfferersSuite) TestImportRemoteApplicationOfferers(c *tc.C) {
	// Arrange - import a remote application with provider and requirer endpoints
	endpoints := []crossmodelrelation.RemoteApplicationEndpoint{
		{
			Name:      "client",
			Role:      appcharm.RoleProvider,
			Interface: "kafka",
		},
		{
			Name:      "zookeeper",
			Role:      appcharm.RoleRequirer,
			Interface: "zookeeper",
		},
	}
	args := []crossmodelrelation.RemoteApplicationOffererImport{
		{
			RemoteApplicationImport: crossmodelrelation.RemoteApplicationImport{
				Name:            "remote-kafka",
				OfferUUID:       tc.Must(c, internaluuid.NewUUID).String(),
				URL:             "ctrl:admin/prod.kafka",
				SourceModelUUID: tc.Must(c, internaluuid.NewUUID).String(),
				Macaroon:        "test-macaroon-data",
				SyntheticCharm:  buildTestSyntheticCharm("remote-kafka", endpoints),
			},
		},
	}

	// Act
	err := s.state.ImportRemoteApplicationOfferers(c.Context(), args)

	// Assert
	c.Assert(err, tc.IsNil)

	// Verify remote application offerer was created
	var (
		offerUUID        string
		offerURL         string
		offererModelUUID string
		macaroon         string
	)
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `
			SELECT offer_uuid, offer_url, offerer_model_uuid, macaroon
			FROM application_remote_offerer rao
			JOIN application a ON rao.application_uuid = a.uuid
			WHERE a.name = ?
		`, args[0].Name).Scan(&offerUUID, &offerURL, &offererModelUUID, &macaroon)
	})
	c.Assert(err, tc.IsNil)
	c.Check(offerUUID, tc.Equals, args[0].OfferUUID)
	c.Check(offerURL, tc.Equals, args[0].URL)
	c.Check(offererModelUUID, tc.Equals, args[0].SourceModelUUID)
	c.Check(macaroon, tc.Equals, args[0].Macaroon)

	// Verify synthetic charm endpoints (excluding juju-info)
	var endpointCount int
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `
			SELECT COUNT(*)
			FROM charm_relation cr
			JOIN charm c ON cr.charm_uuid = c.uuid
			JOIN application a ON a.charm_uuid = c.uuid
			WHERE a.name = ? AND cr.name != 'juju-info'
		`, args[0].Name).Scan(&endpointCount)
	})
	c.Assert(err, tc.IsNil)
	c.Check(endpointCount, tc.Equals, 2, tc.Commentf("Expected 2 user-defined endpoints"))
}

func (s *importRemoteApplicationOfferersSuite) TestImportRemoteApplicationOfferersWithUnits(c *tc.C) {
	// Arrange - import a remote application with synthetic units
	endpoints := []crossmodelrelation.RemoteApplicationEndpoint{
		{
			Name:      "db",
			Role:      appcharm.RoleProvider,
			Interface: "mysql",
		},
	}
	args := []crossmodelrelation.RemoteApplicationOffererImport{
		{
			RemoteApplicationImport: crossmodelrelation.RemoteApplicationImport{
				Name:            "remote-mysql",
				OfferUUID:       tc.Must(c, internaluuid.NewUUID).String(),
				URL:             "ctrl:admin/prod.mysql",
				SourceModelUUID: tc.Must(c, internaluuid.NewUUID).String(),
				Macaroon:        "test-macaroon",
				SyntheticCharm:  buildTestSyntheticCharm("remote-mysql", endpoints),
				Units:           []string{"remote-mysql/0", "remote-mysql/1"},
			},
		},
	}

	// Act
	err := s.state.ImportRemoteApplicationOfferers(c.Context(), args)

	// Assert
	c.Assert(err, tc.IsNil)

	// Verify synthetic units were created
	var unitNames []string
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		unitNames = nil

		rows, err := tx.QueryContext(ctx, `
			SELECT u.name
			FROM unit u
			JOIN application a ON u.application_uuid = a.uuid
			WHERE a.name = ?
			ORDER BY u.name
		`, args[0].Name)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				return err
			}
			unitNames = append(unitNames, name)
		}
		return rows.Err()
	})
	c.Assert(err, tc.IsNil)
	c.Check(unitNames, tc.DeepEquals, []string{"remote-mysql/0", "remote-mysql/1"})
}

func (s *importRemoteApplicationOfferersSuite) TestImportRemoteApplicationOfferersMultiple(c *tc.C) {
	// Arrange - import multiple remote applications
	endpoints1 := []crossmodelrelation.RemoteApplicationEndpoint{
		{
			Name:      "db",
			Role:      appcharm.RoleProvider,
			Interface: "mysql",
		},
	}
	endpoints2 := []crossmodelrelation.RemoteApplicationEndpoint{
		{
			Name:      "db",
			Role:      appcharm.RoleProvider,
			Interface: "pgsql",
		},
	}
	args := []crossmodelrelation.RemoteApplicationOffererImport{
		{
			RemoteApplicationImport: crossmodelrelation.RemoteApplicationImport{
				Name:                   "remote-mysql",
				OfferUUID:              tc.Must(c, internaluuid.NewUUID).String(),
				URL:                    "ctrl:admin/model.mysql",
				SourceModelUUID:        tc.Must(c, internaluuid.NewUUID).String(),
				Macaroon:               "macaroon1",
				SyntheticCharm:         buildTestSyntheticCharm("remote-mysql", endpoints1),
				OffererApplicationUUID: tc.Must(c, internaluuid.NewUUID).String(),
			},
		},
		{
			RemoteApplicationImport: crossmodelrelation.RemoteApplicationImport{
				Name:                   "remote-postgres",
				OfferUUID:              tc.Must(c, internaluuid.NewUUID).String(),
				URL:                    "ctrl:admin/model.postgres",
				SourceModelUUID:        tc.Must(c, internaluuid.NewUUID).String(),
				Macaroon:               "macaroon2",
				SyntheticCharm:         buildTestSyntheticCharm("remote-postgres", endpoints2),
				OffererApplicationUUID: tc.Must(c, internaluuid.NewUUID).String(),
			},
		},
	}

	// Act
	err := s.state.ImportRemoteApplicationOfferers(c.Context(), args)

	// Assert
	c.Assert(err, tc.IsNil)

	// Verify both remote applications were created
	var count int
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM application_remote_offerer").Scan(&count)
	})
	c.Assert(err, tc.IsNil)
	c.Check(count, tc.Equals, 2)
}

func (s *importRemoteApplicationOfferersSuite) TestImportRemoteApplicationOfferersEmpty(c *tc.C) {
	args := []crossmodelrelation.RemoteApplicationOffererImport{}

	err := s.state.ImportRemoteApplicationOfferers(c.Context(), args)
	c.Assert(err, tc.IsNil)

	var count int
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM application_remote_offerer").Scan(&count)
	})
	c.Assert(err, tc.IsNil)
	c.Check(count, tc.Equals, 0)
}

type importRemoteApplicationConsumersSuite struct {
	baseSuite
}

func TestImportRemoteApplicationConsumersSuite(t *testing.T) {
	tc.Run(t, &importRemoteApplicationConsumersSuite{})
}

func (s *importRemoteApplicationConsumersSuite) TestImportRemoteApplicationConsumersEmpty(c *tc.C) {
	args := []crossmodelrelation.RemoteApplicationConsumerImport{}

	err := s.state.ImportRemoteApplicationConsumers(c.Context(), args)
	c.Assert(err, tc.IsNil)

	var count int
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM application_remote_consumer").Scan(&count)
	})
	c.Assert(err, tc.IsNil)
	c.Check(count, tc.Equals, 0)
}

func (s *importRemoteApplicationConsumersSuite) TestImportRemoteApplicationConsumers(c *tc.C) {
	charmUUID := tc.Must(c, internaluuid.NewUUID).String()
	offerUUID := tc.Must(c, internaluuid.NewUUID).String()
	relationUUID := tc.Must(c, internaluuid.NewUUID).String()
	consumerModelUUID := tc.Must(c, internaluuid.NewUUID).String()
	consumerApplicationUUID := tc.Must(c, internaluuid.NewUUID).String()

	// Offer resources needed:
	offerApplicationUUID := tc.Must(c, coreapplication.NewUUID)
	offerCharmUUID := tc.Must(c, internaluuid.NewUUID).String()
	// Create an offer in the database.
	s.createOffer(c, offerUUID)
	// Create a charm in the database.
	s.createCharm(c, offerCharmUUID)
	charmRelationUUID := s.createCharmRelation(c, offerCharmUUID, "offer-endpoint")
	// Create an application in the database.
	s.createApplication(c, offerApplicationUUID, offerCharmUUID, offerUUID)
	s.addApplicationEndpoint(c, offerApplicationUUID, charmRelationUUID)

	charm := appcharm.Charm{
		ReferenceName: "bar",
		Source:        appcharm.CMRSource,
		Metadata: appcharm.Metadata{
			Name:        "foo",
			Description: "remote consumer application",
			Provides: map[string]appcharm.Relation{
				"db": {
					Name:      "db",
					Role:      appcharm.RoleProvider,
					Interface: "db",
					Limit:     1,
					Scope:     appcharm.ScopeGlobal,
				},
			},
			Requires: map[string]appcharm.Relation{},
			Peers:    map[string]appcharm.Relation{},
		},
	}
	err := s.state.ImportRemoteApplicationConsumers(c.Context(), []crossmodelrelation.RemoteApplicationConsumerImport{{
		RemoteApplicationImport: crossmodelrelation.RemoteApplicationImport{
			Name:                   "foo",
			OfferUUID:              offerUUID,
			SyntheticCharm:         charm,
			Units:                  []string{"foo/0"},
			OffererApplicationUUID: offerApplicationUUID.String(),
		},
		RelationUUID:                relationUUID,
		RelationID:                  42,
		RelationScope:               appcharm.ScopeGlobal,
		RelationSuspended:           true,
		RelationSuspendedReason:     "imported suspended",
		ConsumerModelUUID:           consumerModelUUID,
		ConsumerApplicationUUID:     consumerApplicationUUID,
		ConsumerApplicationEndpoint: "db",
		OffererApplicationEndpoint:  "offer-endpoint",
		UserName:                    "consumer-user",
		SyntheticApplicationUUID:    consumerApplicationUUID,
		SyntheticCharmUUID:          charmUUID,
	}})
	c.Assert(err, tc.ErrorIsNil)

	s.assertApplicationRemoteConsumer(c, consumerApplicationUUID)
	s.assertApplication(c, consumerApplicationUUID)
	s.assertCharmMetadata(c, consumerApplicationUUID, charmUUID, charm)

	endpoints := s.fetchApplicationEndpoints(c, consumerApplicationUUID)
	if c.Check(endpoints, tc.HasLen, 2) {
		c.Check(endpoints[0].charmRelationName, tc.Equals, "db")
		c.Check(endpoints[1].charmRelationName, tc.Equals, "juju-info")
	}

	// Check that the synthetic relation has been created with the expected
	// UUID and the relation ID imported from the source model.
	s.assertRelation(c, relationUUID, 42)
	s.assertRelationSuspended(c, relationUUID, true, "imported suspended")

	s.assertRelationEndpoints(c, relationUUID, offerApplicationUUID.String(), consumerApplicationUUID)
}

// TestImportRemoteApplicationConsumersSharedConsumerApplication imports two
// remote application consumers that belong to the same consuming
// application, as migrated from a 3.6 consumer proxy relating to several
// offered applications. Each connection keeps its own synthetic application
// and relation, while both records reference the same consuming application.
func (s *importRemoteApplicationConsumersSuite) TestImportRemoteApplicationConsumersSharedConsumerApplication(c *tc.C) {
	consumerModelUUID := tc.Must(c, internaluuid.NewUUID).String()
	consumerApplicationUUID := tc.Must(c, internaluuid.NewUUID).String()

	// The consuming application of the first connection keeps the identity
	// of the legacy proxy, while the second connection is represented by a
	// freshly generated synthetic application, as assigned by the service
	// layer.
	synthAppUUID1 := consumerApplicationUUID
	synthAppUUID2 := tc.Must(c, coreapplication.NewUUID).String()
	const name1 = "remote-13ea27915e7840d888c5e9451444b45d"
	const name2 = "remote-a50f295556314aa4803f766a8802e33a"

	// Offer resources needed for each of the two offered applications.
	type offerer struct {
		applicationUUID coreapplication.UUID
		offerUUID       string
		charmUUID       string
	}
	offerers := []offerer{
		{applicationUUID: tc.Must(c, coreapplication.NewUUID)},
		{applicationUUID: tc.Must(c, coreapplication.NewUUID)},
	}
	for n := range offerers {
		offerers[n].offerUUID = tc.Must(c, internaluuid.NewUUID).String()
		offerers[n].charmUUID = tc.Must(c, internaluuid.NewUUID).String()

		s.createOffer(c, offerers[n].offerUUID)
		s.createCharm(c, offerers[n].charmUUID)
		charmRelationUUID := s.createCharmRelation(c, offerers[n].charmUUID, "offer-endpoint")
		s.createApplication(c, offerers[n].applicationUUID, offerers[n].charmUUID, offerers[n].offerUUID)
		s.addApplicationEndpoint(c, offerers[n].applicationUUID, charmRelationUUID)
	}

	newCharm := func(name string) appcharm.Charm {
		return appcharm.Charm{
			ReferenceName: name,
			Source:        appcharm.CMRSource,
			Metadata: appcharm.Metadata{
				Name:        name,
				Description: "remote consumer application",
				Provides: map[string]appcharm.Relation{
					"db": {
						Name:      "db",
						Role:      appcharm.RoleProvider,
						Interface: "db",
						Limit:     1,
						Scope:     appcharm.ScopeGlobal,
					},
				},
				Requires: map[string]appcharm.Relation{},
				Peers:    map[string]appcharm.Relation{},
			},
		}
	}

	input := []crossmodelrelation.RemoteApplicationConsumerImport{{
		RemoteApplicationImport: crossmodelrelation.RemoteApplicationImport{
			Name:                   name1,
			OfferUUID:              offerers[0].offerUUID,
			SyntheticCharm:         newCharm(name1),
			Units:                  []string{name1 + "/0"},
			OffererApplicationUUID: offerers[0].applicationUUID.String(),
		},
		RelationUUID:                tc.Must(c, internaluuid.NewUUID).String(),
		RelationID:                  41,
		RelationScope:               appcharm.ScopeGlobal,
		ConsumerModelUUID:           consumerModelUUID,
		ConsumerApplicationUUID:     consumerApplicationUUID,
		ConsumerApplicationEndpoint: "db",
		OffererApplicationEndpoint:  "offer-endpoint",
		UserName:                    "consumer-user",
		SyntheticApplicationUUID:    synthAppUUID1,
		SyntheticCharmUUID:          tc.Must(c, internaluuid.NewUUID).String(),
	}, {
		RemoteApplicationImport: crossmodelrelation.RemoteApplicationImport{
			Name:                   name2,
			OfferUUID:              offerers[1].offerUUID,
			SyntheticCharm:         newCharm(name2),
			OffererApplicationUUID: offerers[1].applicationUUID.String(),
		},
		RelationUUID:                tc.Must(c, internaluuid.NewUUID).String(),
		RelationID:                  42,
		RelationScope:               appcharm.ScopeGlobal,
		ConsumerModelUUID:           consumerModelUUID,
		ConsumerApplicationUUID:     consumerApplicationUUID,
		ConsumerApplicationEndpoint: "db",
		OffererApplicationEndpoint:  "offer-endpoint",
		UserName:                    "consumer-user",
		SyntheticApplicationUUID:    synthAppUUID2,
		SyntheticCharmUUID:          tc.Must(c, internaluuid.NewUUID).String(),
	}}

	err := s.state.ImportRemoteApplicationConsumers(c.Context(), input)
	c.Assert(err, tc.ErrorIsNil)

	// Both offer connections must be recorded for the same consuming
	// application, each with its own synthetic application UUID.
	type arcRow struct {
		offerConnectionUUID   string
		consumerApplicationID string
	}
	var arcRows []arcRow
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `
SELECT offer_connection_uuid, consumer_application_uuid
FROM application_remote_consumer
`)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var row arcRow
			if err := rows.Scan(&row.offerConnectionUUID, &row.consumerApplicationID); err != nil {
				return err
			}
			arcRows = append(arcRows, row)
		}
		return rows.Err()
	})
	c.Assert(err, tc.ErrorIsNil)
	if c.Check(arcRows, tc.HasLen, 2) {
		c.Check(arcRows[0].consumerApplicationID, tc.Equals, consumerApplicationUUID)
		c.Check(arcRows[1].consumerApplicationID, tc.Equals, consumerApplicationUUID)
		c.Check([]string{arcRows[0].offerConnectionUUID, arcRows[1].offerConnectionUUID}, tc.SameContents,
			[]string{synthAppUUID1, synthAppUUID2})
	}

	// Each synthetic application must exist with its own name.
	type appRow struct {
		uuid string
		name string
	}
	var appRows []appRow
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `
SELECT uuid, name FROM application
WHERE uuid IN (?, ?)
`, synthAppUUID1, synthAppUUID2)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var row appRow
			if err := rows.Scan(&row.uuid, &row.name); err != nil {
				return err
			}
			appRows = append(appRows, row)
		}
		return rows.Err()
	})
	c.Assert(err, tc.ErrorIsNil)
	if c.Check(appRows, tc.HasLen, 2) {
		byUUID := map[string]string{
			synthAppUUID1: name1,
			synthAppUUID2: name2,
		}
		for _, row := range appRows {
			c.Check(row.name, tc.Equals, byUUID[row.uuid])
		}
	}

	// Both relations of the consuming application must be imported, each
	// connected to its own offerer application and synthetic application.
	for n, consumer := range input {
		s.assertRelation(c, consumer.RelationUUID, consumer.RelationID)
		s.assertRelationEndpoints(c, consumer.RelationUUID,
			offerers[n].applicationUUID.String(), consumer.SyntheticApplicationUUID)
	}

	var offerConnections int
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM offer_connection
WHERE offer_uuid IN (?, ?)
AND remote_relation_uuid IN (?, ?)
`, offerers[0].offerUUID, offerers[1].offerUUID, input[0].RelationUUID, input[1].RelationUUID).
			Scan(&offerConnections)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(offerConnections, tc.Equals, 2)
}

type importSecretSuite struct {
	baseSuite
}

func TestImportMigrationSuite(t *testing.T) {
	tc.Run(t, &importSecretSuite{})
}

func (s *importSecretSuite) TestGetRelationUUIDByRelationKey(c *tc.C) {
	// Arrange
	charmUUID := s.addCharm(c)
	app1Name := "app1"
	app2Name := "app2"
	app1UUID := s.addApplication(c, charmUUID, app1Name)
	app2UUID := s.addApplication(c, charmUUID, app2Name)

	relation1 := internalcharm.Relation{
		Name:      "endpoint1",
		Role:      internalcharm.RoleProvider,
		Interface: "int1",
		Scope:     internalcharm.ScopeGlobal,
	}
	relationUUID1 := s.addCharmRelation(c, charmUUID, relation1)
	relation2 := internalcharm.Relation{
		Name:      "endpoint2",
		Role:      internalcharm.RoleRequirer,
		Interface: "int1",
		Scope:     internalcharm.ScopeGlobal,
	}
	relationUUID2 := s.addCharmRelation(c, charmUUID, relation2)

	endpoint1UUID := s.addApplicationEndpoint(c, app1UUID, relationUUID1)
	endpoint2UUID := s.addApplicationEndpoint(c, app2UUID, relationUUID2)

	relUUID := s.addRelation(c)
	s.addRelationEndpoint(c, relUUID.String(), endpoint1UUID)
	s.addRelationEndpoint(c, relUUID.String(), endpoint2UUID)

	key, err := corerelation.NewKeyFromString(app2Name + ":endpoint2 " + app1Name + ":endpoint1")
	c.Assert(err, tc.IsNil)

	// Act
	obtainedUUID, err := s.state.GetRelationUUIDByRelationKey(c.Context(), key)

	// Assert
	c.Assert(err, tc.IsNil)
	c.Check(obtainedUUID, tc.Equals, relUUID.String())
}

func (s *importSecretSuite) TestImportRemoteApplicationSecretGrants(c *tc.C) {
	// Arrange
	secretID := "secret-id"
	appUUID := tc.Must(c, internaluuid.NewUUID).String()
	relUUID := tc.Must(c, internaluuid.NewUUID).String()

	// secret_permission.secret_id references secret_metadata(secret_id)
	s.createSecretWithMetadata(c, secretID)

	grants := []internal.RemoteApplicationSecretGrant{
		{
			SecretID:        secretID,
			ApplicationName: "app",
			RelationKey:     "app:endpoint other:endpoint",
			ApplicationUUID: appUUID,
			RelationUUID:    relUUID,
		},
	}

	// Act
	err := s.state.ImportRemoteApplicationSecretGrants(c.Context(), grants)

	// Assert
	c.Assert(err, tc.IsNil)

	var (
		obtainedSecretID    string
		obtainedRoleID      int
		obtainedSubjectUUID string
		obtainedSubjectType int
		obtainedScopeUUID   string
		obtainedScopeType   int
	)
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `
			SELECT secret_id, role_id, subject_uuid, subject_type_id, scope_uuid, scope_type_id
			FROM secret_permission
			WHERE secret_id = ?
		`, secretID).Scan(&obtainedSecretID, &obtainedRoleID, &obtainedSubjectUUID, &obtainedSubjectType, &obtainedScopeUUID, &obtainedScopeType)
	})
	c.Assert(err, tc.IsNil)
	c.Check(obtainedSecretID, tc.Equals, secretID)
	c.Check(obtainedRoleID, tc.Equals, int(domainsecret.RoleView))
	c.Check(obtainedSubjectUUID, tc.Equals, appUUID)
	c.Check(obtainedSubjectType, tc.Equals, int(domainsecret.SubjectApplication))
	c.Check(obtainedScopeUUID, tc.Equals, relUUID)
	c.Check(obtainedScopeType, tc.Equals, int(domainsecret.ScopeRelation))
}

func (s *importSecretSuite) TestImportRemoteSecretConsumers(c *tc.C) {
	// Arrange
	secretID := "secret-id"
	// Ensure the secret exists with metadata and an initial revision.
	s.createSecretWithMetadata(c, secretID)

	consumers := []internal.RemoteUnitConsumer{
		{
			SecretID:        secretID,
			Unit:            "app/0",
			CurrentRevision: 1,
		},
	}

	// Act
	err := s.state.ImportRemoteSecretConsumers(c.Context(), consumers)

	// Assert
	c.Assert(err, tc.IsNil)

	var (
		obtainedSecretID   string
		obtainedUnitName   string
		obtainedCurrentRev int
	)
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `
			SELECT secret_id, unit_name, current_revision
			FROM secret_remote_unit_consumer
			WHERE secret_id = ?
		`, secretID).Scan(&obtainedSecretID, &obtainedUnitName, &obtainedCurrentRev)
	})
	c.Assert(err, tc.IsNil)
	c.Check(obtainedSecretID, tc.Equals, secretID)
	c.Check(obtainedUnitName, tc.Equals, "app/0")
	c.Check(obtainedCurrentRev, tc.Equals, 1)
}

func (s *importSecretSuite) TestImportRemoteSecret(c *tc.C) {
	// Arrange
	secretID := "remote-secret-id"
	sourceModelUUID := tc.Must(c, internaluuid.NewUUID).String()
	unitUUID := tc.Must(c, internaluuid.NewUUID).String()

	// Unit must exist for saveSecretConsumer to work.
	charmUUID := s.addCharm(c)
	appUUID := s.addApplication(c, charmUUID, "app")
	s.addUnitWithUUID(c, unitUUID, "app/0", appUUID.String(), charmUUID.String())

	secret := internal.RemoteSecret{
		SecretID:        secretID,
		SourceModelUUID: sourceModelUUID,
		UnitUUID:        unitUUID,
		Label:           "test-label",
		CurrentRevision: 1,
		LatestRevision:  2,
	}

	// Act
	err := s.state.ImportRemoteSecret(c.Context(), secret)

	// Assert
	c.Assert(err, tc.IsNil)

	// Verify secret exists
	var obtainedSecretID string
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "SELECT id FROM secret WHERE id = ?", secretID).Scan(&obtainedSecretID)
	})
	c.Assert(err, tc.IsNil)
	c.Check(obtainedSecretID, tc.Equals, secretID)

	// Verify unit consumer exists
	var (
		obtainedUnitUUID    string
		obtainedSourceModel string
		obtainedLabel       string
		obtainedRev         int
	)
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `
			SELECT unit_uuid, source_model_uuid, label, current_revision
			FROM secret_unit_consumer
			WHERE secret_id = ?
		`, secretID).Scan(&obtainedUnitUUID, &obtainedSourceModel, &obtainedLabel, &obtainedRev)
	})
	c.Assert(err, tc.IsNil)
	c.Check(obtainedUnitUUID, tc.Equals, unitUUID)
	c.Check(obtainedSourceModel, tc.Equals, sourceModelUUID)
	c.Check(obtainedLabel, tc.Equals, "test-label")
	c.Check(obtainedRev, tc.Equals, 1)

	// Verify secret_reference exists
	var (
		obtainedRefSecretID       string
		obtainedRefLatestRevision int
		obtainedRefMigrated       bool
	)
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `
			SELECT secret_id, latest_revision, migrated
			FROM secret_reference
			WHERE secret_id = ?
		`, secretID).Scan(&obtainedRefSecretID, &obtainedRefLatestRevision, &obtainedRefMigrated)
	})
	c.Assert(err, tc.IsNil)
	c.Check(obtainedRefSecretID, tc.Equals, secretID)
	c.Check(obtainedRefLatestRevision, tc.Equals, 2)
	c.Check(obtainedRefMigrated, tc.Equals, true)
}

func (s *importSecretSuite) TestImportRemoteSecretTwoConsumerUnit(c *tc.C) {
	// Arrange
	secretID := "remote-secret-id"
	sourceModelUUID := tc.Must(c, internaluuid.NewUUID).String()
	unitUUID1 := tc.Must(c, internaluuid.NewUUID).String()
	unitUUID2 := tc.Must(c, internaluuid.NewUUID).String()

	// Unit must exist for saveSecretConsumer to work.
	charmUUID := s.addCharm(c)
	appUUID := s.addApplication(c, charmUUID, "app")
	s.addUnitWithUUID(c, unitUUID1, "app/0", appUUID.String(), charmUUID.String())
	s.addUnitWithUUID(c, unitUUID2, "app/1", appUUID.String(), charmUUID.String())

	secret1 := internal.RemoteSecret{
		SecretID:        secretID,
		SourceModelUUID: sourceModelUUID,
		UnitUUID:        unitUUID1,
		Label:           "test-label",
		CurrentRevision: 1,
		LatestRevision:  2,
	}
	secret2 := internal.RemoteSecret{
		SecretID:        secretID,
		SourceModelUUID: sourceModelUUID,
		UnitUUID:        unitUUID2,
		Label:           "test-label",
		CurrentRevision: 2,
		LatestRevision:  2,
	}

	// Act
	tc.Must2_0(c, s.state.ImportRemoteSecret, c.Context(), secret1)

	err := s.state.ImportRemoteSecret(c.Context(), secret2)

	// Assert
	c.Assert(err, tc.IsNil)

	// Verify secret exists
	var obtainedSecretID string
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "SELECT id FROM secret WHERE id = ?", secretID).Scan(&obtainedSecretID)
	})
	c.Assert(err, tc.IsNil)
	c.Check(obtainedSecretID, tc.Equals, secretID)

	// Verify expected consumer exists
	var (
		obtainedSourceModel string
		obtainedLabel       string
		obtainedRev         int
	)
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `
			SELECT source_model_uuid, label, current_revision
			FROM secret_unit_consumer
			WHERE secret_id = ?
			AND unit_uuid = ?
		`, secretID, unitUUID2).Scan(&obtainedSourceModel, &obtainedLabel, &obtainedRev)
	})
	c.Assert(err, tc.IsNil)
	c.Check(obtainedSourceModel, tc.Equals, sourceModelUUID)
	c.Check(obtainedLabel, tc.Equals, "test-label")
	c.Check(obtainedRev, tc.Equals, 2)

	// Verify secret_reference exists
	var (
		obtainedRefSecretID       string
		obtainedRefLatestRevision int
		obtainedRefMigrated       bool
	)
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `
			SELECT secret_id, latest_revision, migrated
			FROM secret_reference
			WHERE secret_id = ?
		`, secretID).Scan(&obtainedRefSecretID, &obtainedRefLatestRevision, &obtainedRefMigrated)
	})
	c.Assert(err, tc.IsNil)
	c.Check(obtainedRefSecretID, tc.Equals, secretID)
	c.Check(obtainedRefLatestRevision, tc.Equals, 2)
	c.Check(obtainedRefMigrated, tc.Equals, true)
}

func (s *importSecretSuite) createSecretWithMetadata(c *tc.C, secretID string) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		// base secret
		if _, err := tx.ExecContext(ctx, "INSERT INTO secret (id) VALUES (?)", secretID); err != nil {
			return err
		}
		// metadata requires rotate policy id to exist; schema seeds it. Use 0 (never).
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO secret_metadata (secret_id, version, description, rotate_policy_id, auto_prune)
			VALUES (?, 1, '', 0, 0)
		`, secretID); err != nil {
			return err
		}
		// create initial revision so latest revision queries work.
		revUUID := internaluuid.MustNewUUID().String()
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO secret_revision (uuid, secret_id, revision)
			VALUES (?, ?, 1)
		`, revUUID, secretID); err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.IsNil)
}

func (s *importSecretSuite) addUnitWithUUID(c *tc.C, unitUUID string, unitName string, appUUID string, charmUUID string) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		netNodeUUID := internaluuid.MustNewUUID().String()
		_, err := tx.ExecContext(ctx, "INSERT INTO net_node (uuid) VALUES (?)", netNodeUUID)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `
            INSERT INTO unit (uuid, name, application_uuid, charm_uuid, net_node_uuid, life_id)
            VALUES (?, ?, ?, ?, ?, ?)`, unitUUID, unitName, appUUID, charmUUID, netNodeUUID, life.Alive)
		return err
	})
	c.Assert(err, tc.IsNil)
}
