// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"testing"

	"github.com/juju/tc"

	coreapplication "github.com/juju/juju/core/application"
	appcharm "github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/crossmodelrelation"
	internalcharm "github.com/juju/juju/domain/deployment/charm"
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
				OfferUUID:       internaluuid.MustNewUUID().String(),
				URL:             "ctrl:admin/prod.kafka",
				SourceModelUUID: internaluuid.MustNewUUID().String(),
				Macaroon:        "test-macaroon-data",
				Endpoints:       endpoints,
				SyntheticCharm:  buildTestSyntheticCharm("remote-kafka", endpoints),
				Bindings:        map[string]string{"client": "alpha", "zookeeper": "beta"},
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
				OfferUUID:       internaluuid.MustNewUUID().String(),
				URL:             "ctrl:admin/prod.mysql",
				SourceModelUUID: internaluuid.MustNewUUID().String(),
				Macaroon:        "test-macaroon",
				Endpoints:       endpoints,
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
				Name:            "remote-mysql",
				OfferUUID:       internaluuid.MustNewUUID().String(),
				URL:             "ctrl:admin/model.mysql",
				SourceModelUUID: internaluuid.MustNewUUID().String(),
				Macaroon:        "macaroon1",
				Endpoints:       endpoints1,
				SyntheticCharm:  buildTestSyntheticCharm("remote-mysql", endpoints1),
			},
		},
		{
			RemoteApplicationImport: crossmodelrelation.RemoteApplicationImport{
				Name:            "remote-postgres",
				OfferUUID:       internaluuid.MustNewUUID().String(),
				URL:             "ctrl:admin/model.postgres",
				SourceModelUUID: internaluuid.MustNewUUID().String(),
				Macaroon:        "macaroon2",
				Endpoints:       endpoints2,
				SyntheticCharm:  buildTestSyntheticCharm("remote-postgres", endpoints2),
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
			Name:           "foo",
			OfferUUID:      offerUUID,
			SyntheticCharm: charm,
			Units:          []string{"foo/0"},
		},
		RelationUUID:                relationUUID,
		ConsumerModelUUID:           consumerModelUUID,
		ConsumerApplicationUUID:     consumerApplicationUUID,
		ConsumerApplicationEndpoint: "db",
		OffererApplicationUUID:      offerApplicationUUID.String(),
		OffererApplicationEndpoint:  "offer-endpoint",
		UserName:                    "consumer-user",
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
	// UUID and ID 0 (the first relation created in the model).
	s.assertRelation(c, relationUUID, 0)

	s.assertRelationEndpoints(c, relationUUID, offerApplicationUUID.String(), consumerApplicationUUID)
}
