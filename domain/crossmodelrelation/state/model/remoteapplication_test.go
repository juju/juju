// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"maps"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/crossmodelrelation"
	crossmodelrelationerrors "github.com/juju/juju/domain/crossmodelrelation/errors"
	"github.com/juju/juju/domain/life"
	internalerrors "github.com/juju/juju/internal/errors"
	internaluuid "github.com/juju/juju/internal/uuid"
)

type modelRemoteApplicationSuite struct {
	baseSuite
}

func TestModelRemoteApplicationSuite(t *testing.T) {
	tc.Run(t, &modelRemoteApplicationSuite{})
}

func (s *modelRemoteApplicationSuite) TestAddRemoteApplicationOffererInsertsApplicationAndCharm(c *tc.C) {
	applicationUUID := tc.Must(c, internaluuid.NewUUID).String()
	charmUUID := tc.Must(c, internaluuid.NewUUID).String()

	charm := charm.Charm{
		ReferenceName: "bar",
		Source:        charm.CMRSource,
		Metadata: charm.Metadata{
			Name:        "foo",
			Description: "remote offerer application",
			Provides: map[string]charm.Relation{
				"db": {
					Name:      "db",
					Role:      charm.RoleProvider,
					Interface: "db",
					Limit:     1,
					Scope:     charm.ScopeGlobal,
				},
			},
			Requires: map[string]charm.Relation{
				"cache": {
					Name:      "cache",
					Role:      charm.RoleRequirer,
					Interface: "cacher",
					Scope:     charm.ScopeGlobal,
				},
			},
			Peers: map[string]charm.Relation{},
		},
	}
	err := s.state.AddRemoteApplicationOfferer(c.Context(), "foo", crossmodelrelation.AddRemoteApplicationOffererArgs{
		AddRemoteApplicationArgs: crossmodelrelation.AddRemoteApplicationArgs{
			ApplicationUUID:       applicationUUID,
			CharmUUID:             charmUUID,
			RemoteApplicationUUID: tc.Must(c, internaluuid.NewUUID).String(),
			OfferUUID:             tc.Must(c, internaluuid.NewUUID).String(),
			Charm:                 charm,
		},
		EncodedMacaroon: []byte("encoded macaroon"),
	})
	c.Assert(err, tc.ErrorIsNil)

	s.assertApplicationRemoteOfferer(c, applicationUUID)
	s.assertApplicationRemoteOffererStatus(c, applicationUUID)
	s.assertApplication(c, applicationUUID)
	s.assertCharmMetadata(c, applicationUUID, charmUUID, charm)
}

func (s *modelRemoteApplicationSuite) TestAddRemoteApplicationOffererInsertsApplicationEndpointBindings(c *tc.C) {
	applicationUUID := tc.Must(c, internaluuid.NewUUID).String()
	charmUUID := tc.Must(c, internaluuid.NewUUID).String()

	charm := charm.Charm{
		ReferenceName: "bar",
		Source:        charm.CMRSource,
		Metadata: charm.Metadata{
			Name:        "foo",
			Description: "remote offerer application",
			Provides: map[string]charm.Relation{
				"db": {
					Name:      "db",
					Role:      charm.RoleProvider,
					Interface: "db",
					Limit:     1,
					Scope:     charm.ScopeGlobal,
				},
			},
			Requires: map[string]charm.Relation{
				"cache": {
					Name:      "cache",
					Role:      charm.RoleRequirer,
					Interface: "cacher",
					Scope:     charm.ScopeGlobal,
				},
			},
			Peers: map[string]charm.Relation{},
		},
	}
	err := s.state.AddRemoteApplicationOfferer(c.Context(), "foo", crossmodelrelation.AddRemoteApplicationOffererArgs{
		AddRemoteApplicationArgs: crossmodelrelation.AddRemoteApplicationArgs{
			ApplicationUUID:       applicationUUID,
			CharmUUID:             charmUUID,
			RemoteApplicationUUID: tc.Must(c, internaluuid.NewUUID).String(),
			OfferUUID:             tc.Must(c, internaluuid.NewUUID).String(),
			Charm:                 charm,
		},
		EncodedMacaroon: []byte("encoded macaroon"),
	})
	c.Assert(err, tc.ErrorIsNil)

	// This should contain three endpoints:
	// - cache (requirer)
	// - db (provider)
	// - juju-info (provider, automatically added)

	endpoints := s.fetchApplicationEndpoints(c, applicationUUID)
	c.Assert(endpoints, tc.HasLen, 3)

	c.Check(endpoints[0].charmRelationName, tc.Equals, "cache")
	c.Check(endpoints[1].charmRelationName, tc.Equals, "db")
	c.Check(endpoints[2].charmRelationName, tc.Equals, "juju-info")
}

func (s *modelRemoteApplicationSuite) TestAddRemoteApplicationOffererInsertsApplicationAndCharmWithNoRelations(c *tc.C) {
	applicationUUID := tc.Must(c, internaluuid.NewUUID).String()
	charmUUID := tc.Must(c, internaluuid.NewUUID).String()

	charm := charm.Charm{
		ReferenceName: "bar",
		Source:        charm.CMRSource,
		Metadata: charm.Metadata{
			Name:        "foo",
			Description: "remote offerer application",
		},
	}
	err := s.state.AddRemoteApplicationOfferer(c.Context(), "foo", crossmodelrelation.AddRemoteApplicationOffererArgs{
		AddRemoteApplicationArgs: crossmodelrelation.AddRemoteApplicationArgs{
			ApplicationUUID:       applicationUUID,
			CharmUUID:             charmUUID,
			RemoteApplicationUUID: tc.Must(c, internaluuid.NewUUID).String(),
			OfferUUID:             tc.Must(c, internaluuid.NewUUID).String(),
			Charm:                 charm,
		},
		EncodedMacaroon: []byte("encoded macaroon"),
	})
	c.Assert(err, tc.ErrorIsNil)

	s.assertApplicationRemoteOfferer(c, applicationUUID)
	s.assertApplication(c, applicationUUID)
	s.assertCharmMetadata(c, applicationUUID, charmUUID, charm)

	// This should contain one endpoint:
	// - juju-info (provider, automatically added)

	endpoints := s.fetchApplicationEndpoints(c, applicationUUID)
	c.Assert(endpoints, tc.HasLen, 1)

	c.Check(endpoints[0].charmRelationName, tc.Equals, "juju-info")
}

func (s *modelRemoteApplicationSuite) TestAddRemoteApplicationOffererInsertsApplicationAndCharmTwice(c *tc.C) {
	applicationUUID := tc.Must(c, internaluuid.NewUUID).String()
	charmUUID := tc.Must(c, internaluuid.NewUUID).String()

	charm := charm.Charm{
		ReferenceName: "bar",
		Source:        charm.CMRSource,
		Metadata: charm.Metadata{
			Name:        "foo",
			Description: "remote offerer application",
			Provides: map[string]charm.Relation{
				"db": {
					Name:      "db",
					Role:      charm.RoleProvider,
					Interface: "db",
					Limit:     1,
					Scope:     charm.ScopeGlobal,
				},
			},
			Requires: map[string]charm.Relation{
				"cache": {
					Name:      "cache",
					Role:      charm.RoleRequirer,
					Interface: "cacher",
					Scope:     charm.ScopeGlobal,
				},
			},
			Peers: map[string]charm.Relation{},
		},
	}
	err := s.state.AddRemoteApplicationOfferer(c.Context(), "foo", crossmodelrelation.AddRemoteApplicationOffererArgs{
		AddRemoteApplicationArgs: crossmodelrelation.AddRemoteApplicationArgs{
			ApplicationUUID:       applicationUUID,
			CharmUUID:             charmUUID,
			RemoteApplicationUUID: tc.Must(c, internaluuid.NewUUID).String(),
			OfferUUID:             tc.Must(c, internaluuid.NewUUID).String(),
			Charm:                 charm,
		},
		EncodedMacaroon: []byte("encoded macaroon"),
	})
	c.Assert(err, tc.ErrorIsNil)

	s.assertApplicationRemoteOfferer(c, applicationUUID)
	s.assertApplication(c, applicationUUID)
	s.assertCharmMetadata(c, applicationUUID, charmUUID, charm)

	err = s.state.AddRemoteApplicationOfferer(c.Context(), "foo", crossmodelrelation.AddRemoteApplicationOffererArgs{
		AddRemoteApplicationArgs: crossmodelrelation.AddRemoteApplicationArgs{
			ApplicationUUID:       applicationUUID,
			CharmUUID:             charmUUID,
			RemoteApplicationUUID: tc.Must(c, internaluuid.NewUUID).String(),
			OfferUUID:             tc.Must(c, internaluuid.NewUUID).String(),
			Charm:                 charm,
		},
		EncodedMacaroon: []byte("encoded macaroon"),
	})
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationAlreadyExists)
}

func (s *modelRemoteApplicationSuite) TestAddRemoteApplicationOffererInsertsApplicationAndCharmTwiceSameOfferUUIDDifferentName(c *tc.C) {
	applicationUUID := tc.Must(c, internaluuid.NewUUID).String()
	charmUUID := tc.Must(c, internaluuid.NewUUID).String()
	offerUUID := tc.Must(c, internaluuid.NewUUID).String()

	charm := charm.Charm{
		ReferenceName: "bar",
		Source:        charm.CMRSource,
		Metadata: charm.Metadata{
			Name:        "foo",
			Description: "remote offerer application",
			Provides: map[string]charm.Relation{
				"db": {
					Name:      "db",
					Role:      charm.RoleProvider,
					Interface: "db",
					Limit:     1,
					Scope:     charm.ScopeGlobal,
				},
			},
			Requires: map[string]charm.Relation{
				"cache": {
					Name:      "cache",
					Role:      charm.RoleRequirer,
					Interface: "cacher",
					Scope:     charm.ScopeGlobal,
				},
			},
			Peers: map[string]charm.Relation{},
		},
	}

	err := s.state.AddRemoteApplicationOfferer(c.Context(), "foo", crossmodelrelation.AddRemoteApplicationOffererArgs{
		AddRemoteApplicationArgs: crossmodelrelation.AddRemoteApplicationArgs{
			ApplicationUUID:       applicationUUID,
			CharmUUID:             charmUUID,
			RemoteApplicationUUID: tc.Must(c, internaluuid.NewUUID).String(),
			OfferUUID:             offerUUID,
			Charm:                 charm,
		},
		EncodedMacaroon: []byte("encoded macaroon"),
	})
	c.Assert(err, tc.ErrorIsNil)

	s.assertApplicationRemoteOfferer(c, applicationUUID)
	s.assertApplication(c, applicationUUID)
	s.assertCharmMetadata(c, applicationUUID, charmUUID, charm)

	err = s.state.AddRemoteApplicationOfferer(c.Context(), "bar", crossmodelrelation.AddRemoteApplicationOffererArgs{
		AddRemoteApplicationArgs: crossmodelrelation.AddRemoteApplicationArgs{
			ApplicationUUID:       applicationUUID,
			CharmUUID:             charmUUID,
			RemoteApplicationUUID: tc.Must(c, internaluuid.NewUUID).String(),
			OfferUUID:             offerUUID,
			Charm:                 charm,
		},
		EncodedMacaroon: []byte("encoded macaroon"),
	})
	c.Assert(err, tc.ErrorIs, crossmodelrelationerrors.OfferAlreadyConsumed)
}

func (s *modelRemoteApplicationSuite) TestAddRemoteApplicationOffererInsertsVersionSequence(c *tc.C) {
	applicationUUID := tc.Must(c, internaluuid.NewUUID).String()
	charmUUID := tc.Must(c, internaluuid.NewUUID).String()
	offerUUID := tc.Must(c, internaluuid.NewUUID).String()

	charm := charm.Charm{
		ReferenceName: "bar",
		Source:        charm.CMRSource,
		Metadata: charm.Metadata{
			Name:        "foo",
			Description: "remote offerer application",
		},
	}
	err := s.state.AddRemoteApplicationOfferer(c.Context(), "foo", crossmodelrelation.AddRemoteApplicationOffererArgs{
		AddRemoteApplicationArgs: crossmodelrelation.AddRemoteApplicationArgs{
			ApplicationUUID:       applicationUUID,
			CharmUUID:             charmUUID,
			OfferUUID:             offerUUID,
			RemoteApplicationUUID: tc.Must(c, internaluuid.NewUUID).String(),
			Charm:                 charm,
		},
		EncodedMacaroon: []byte("encoded macaroon"),
	})
	c.Assert(err, tc.ErrorIsNil)

	// Ensure that we have a sequence table.

	var sequence int
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `
SELECT value
FROM sequence
WHERE namespace=?`, "remote-offerer-application_"+offerUUID).Scan(&sequence)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(sequence, tc.Equals, 0)
}

func (s *modelRemoteApplicationSuite) TestAddRemoteApplicationOffererInsertsVersionRespectsSequence(c *tc.C) {
	applicationUUID := tc.Must(c, internaluuid.NewUUID).String()
	charmUUID := tc.Must(c, internaluuid.NewUUID).String()
	offerUUID := tc.Must(c, internaluuid.NewUUID).String()

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO sequence (namespace, value)
VALUES (?, ?)
`, "remote-offerer-application_"+offerUUID, 42)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	charm := charm.Charm{
		ReferenceName: "bar",
		Source:        charm.CMRSource,
		Metadata: charm.Metadata{
			Name:        "foo",
			Description: "remote offerer application",
		},
	}

	err = s.state.AddRemoteApplicationOfferer(c.Context(), "foo", crossmodelrelation.AddRemoteApplicationOffererArgs{
		AddRemoteApplicationArgs: crossmodelrelation.AddRemoteApplicationArgs{
			ApplicationUUID:       applicationUUID,
			CharmUUID:             charmUUID,
			OfferUUID:             offerUUID,
			RemoteApplicationUUID: tc.Must(c, internaluuid.NewUUID).String(),
			Charm:                 charm,
		},
		EncodedMacaroon: []byte("encoded macaroon"),
	})
	c.Assert(err, tc.ErrorIsNil)

	// Ensure that we have a sequence table.

	var sequence int
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `
SELECT value
FROM sequence
WHERE namespace=?`, "remote-offerer-application_"+offerUUID).Scan(&sequence)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(sequence, tc.Equals, 43)
}

func (s *modelRemoteApplicationSuite) TestGetRemoteApplicationOfferers(c *tc.C) {
	applicationUUID := tc.Must(c, internaluuid.NewUUID).String()
	charmUUID := tc.Must(c, internaluuid.NewUUID).String()
	offerUUID := tc.Must(c, internaluuid.NewUUID).String()
	offererModelUUID := tc.Must(c, internaluuid.NewUUID).String()

	mac := newMacaroon(c, "encoded macaroon")
	macBytes := tc.Must(c, mac.MarshalJSON)

	charm := charm.Charm{
		ReferenceName: "bar",
		Source:        charm.CMRSource,
		Metadata: charm.Metadata{
			Name:        "foo",
			Description: "remote offerer application",
			Provides: map[string]charm.Relation{
				"db": {
					Name:      "db",
					Role:      charm.RoleProvider,
					Interface: "db",
					Limit:     1,
					Scope:     charm.ScopeGlobal,
				},
			},
			Requires: map[string]charm.Relation{
				"cache": {
					Name:      "cache",
					Role:      charm.RoleRequirer,
					Interface: "cacher",
					Scope:     charm.ScopeGlobal,
				},
			},
		},
	}
	err := s.state.AddRemoteApplicationOfferer(c.Context(), "foo", crossmodelrelation.AddRemoteApplicationOffererArgs{
		AddRemoteApplicationArgs: crossmodelrelation.AddRemoteApplicationArgs{
			ApplicationUUID:       applicationUUID,
			CharmUUID:             charmUUID,
			RemoteApplicationUUID: tc.Must(c, internaluuid.NewUUID).String(),
			OfferUUID:             offerUUID,
			Charm:                 charm,
		},
		OffererModelUUID: offererModelUUID,
		OfferURL:         "controller:qualifier/model.offername",
		EncodedMacaroon:  macBytes,
	})
	c.Assert(err, tc.ErrorIsNil)

	results, err := s.state.GetRemoteApplicationOfferers(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(results, tc.DeepEquals, []crossmodelrelation.RemoteApplicationOfferer{{
		ApplicationUUID:  applicationUUID,
		ApplicationName:  "foo",
		Life:             life.Alive,
		OfferUUID:        offerUUID,
		OfferURL:         "controller:qualifier/model.offername",
		ConsumeVersion:   0,
		OffererModelUUID: offererModelUUID,
		Macaroon:         mac,
	}})
}

func (s *modelRemoteApplicationSuite) TestGetRemoteApplicationOfferersEmpty(c *tc.C) {
	// Initially there are no remote application offerers.
	results, err := s.state.GetRemoteApplicationOfferers(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(results, tc.HasLen, 0)
}

func (s *modelRemoteApplicationSuite) TestAddRemoteApplicationConsumer(c *tc.C) {
	applicationUUID := tc.Must(c, internaluuid.NewUUID).String()
	charmUUID := tc.Must(c, internaluuid.NewUUID).String()
	offerUUID := tc.Must(c, internaluuid.NewUUID).String()
	relationUUID := tc.Must(c, internaluuid.NewUUID).String()

	// Local resources needed:
	localApplicationUUID := tc.Must(c, internaluuid.NewUUID).String()
	localCharmUUID := tc.Must(c, internaluuid.NewUUID).String()
	// Create an offer in the database.
	s.createOffer(c, offerUUID)
	// Create a charm in the database.
	s.createCharm(c, localCharmUUID)
	// Create an application in the database.
	s.createApplication(c, localApplicationUUID, localCharmUUID, offerUUID)

	charm := charm.Charm{
		ReferenceName: "bar",
		Source:        charm.CMRSource,
		Metadata: charm.Metadata{
			Name:        "foo",
			Description: "remote consumer application",
			Provides: map[string]charm.Relation{
				"db": {
					Name:      "db",
					Role:      charm.RoleProvider,
					Interface: "db",
					Limit:     1,
					Scope:     charm.ScopeGlobal,
				},
			},
			Requires: map[string]charm.Relation{
				"cache": {
					Name:      "cache",
					Role:      charm.RoleRequirer,
					Interface: "cacher",
					Scope:     charm.ScopeGlobal,
				},
			},
			Peers: map[string]charm.Relation{},
		},
	}
	err := s.state.AddRemoteApplicationConsumer(c.Context(), "foo", crossmodelrelation.AddRemoteApplicationConsumerArgs{
		AddRemoteApplicationArgs: crossmodelrelation.AddRemoteApplicationArgs{
			ApplicationUUID:       applicationUUID,
			CharmUUID:             charmUUID,
			RemoteApplicationUUID: tc.Must(c, internaluuid.NewUUID).String(),
			OfferUUID:             offerUUID,
			Charm:                 charm,
		},
		RelationUUID: relationUUID,
	})
	c.Assert(err, tc.ErrorIsNil)

	s.assertApplicationRemoteConsumer(c, applicationUUID)
	s.assertApplication(c, applicationUUID)
	s.assertCharmMetadata(c, applicationUUID, charmUUID, charm)

	endpoints := s.fetchApplicationEndpoints(c, applicationUUID)
	c.Assert(endpoints, tc.HasLen, 3)

	c.Check(endpoints[0].charmRelationName, tc.Equals, "cache")
	c.Check(endpoints[1].charmRelationName, tc.Equals, "db")
	c.Check(endpoints[2].charmRelationName, tc.Equals, "juju-info")

	// Fetch the synthetic relation from the application_remote_relation table:
	var syntheticRelationUUID string
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `
SELECT r.uuid
FROM   relation AS r
JOIN   application_remote_relation AS arr ON r.uuid = arr.relation_uuid
WHERE  arr.consumer_relation_uuid = ?`, relationUUID).
			Scan(&syntheticRelationUUID)
	})
	c.Assert(err, tc.ErrorIsNil)
	// Check that the synthetic relation has been created with the expected
	// UUID and ID 0 (the first relation created in the model).
	s.assertRelation(c, syntheticRelationUUID, 0)
}

func (s *modelRemoteApplicationSuite) TestAddRemoteApplicationConsumerTwiceSameApp(c *tc.C) {
	applicationUUID := tc.Must(c, internaluuid.NewUUID).String()
	charmUUID := tc.Must(c, internaluuid.NewUUID).String()
	offerUUID := tc.Must(c, internaluuid.NewUUID).String()
	relationUUID0 := tc.Must(c, internaluuid.NewUUID).String()
	relationUUID1 := tc.Must(c, internaluuid.NewUUID).String()

	// Local resources needed:
	localApplicationUUID := tc.Must(c, internaluuid.NewUUID).String()
	localCharmUUID := tc.Must(c, internaluuid.NewUUID).String()
	// Create an offer in the database.
	s.createOffer(c, offerUUID)
	// Create a charm in the database.
	s.createCharm(c, localCharmUUID)
	// Create an application in the database.
	s.createApplication(c, localApplicationUUID, localCharmUUID, offerUUID)

	charm := charm.Charm{
		ReferenceName: "bar",
		Source:        charm.CMRSource,
		Metadata: charm.Metadata{
			Name:        "foo",
			Description: "remote consumer application",
			Provides: map[string]charm.Relation{
				"db": {
					Name:      "db",
					Role:      charm.RoleProvider,
					Interface: "db",
					Limit:     1,
					Scope:     charm.ScopeGlobal,
				},
			},
			Requires: map[string]charm.Relation{
				"cache": {
					Name:      "cache",
					Role:      charm.RoleRequirer,
					Interface: "cacher",
					Scope:     charm.ScopeGlobal,
				},
			},
			Peers: map[string]charm.Relation{},
		},
	}
	err := s.state.AddRemoteApplicationConsumer(c.Context(), "foo", crossmodelrelation.AddRemoteApplicationConsumerArgs{
		AddRemoteApplicationArgs: crossmodelrelation.AddRemoteApplicationArgs{
			ApplicationUUID:       applicationUUID,
			CharmUUID:             charmUUID,
			RemoteApplicationUUID: tc.Must(c, internaluuid.NewUUID).String(),
			OfferUUID:             offerUUID,
			Charm:                 charm,
		},
		RelationUUID: relationUUID0,
	})
	c.Assert(err, tc.ErrorIsNil)

	s.assertApplicationRemoteConsumer(c, applicationUUID)
	s.assertApplication(c, applicationUUID)
	s.assertCharmMetadata(c, applicationUUID, charmUUID, charm)

	err = s.state.AddRemoteApplicationConsumer(c.Context(), "foo", crossmodelrelation.AddRemoteApplicationConsumerArgs{
		AddRemoteApplicationArgs: crossmodelrelation.AddRemoteApplicationArgs{
			ApplicationUUID:       applicationUUID,
			CharmUUID:             charmUUID,
			RemoteApplicationUUID: tc.Must(c, internaluuid.NewUUID).String(),
			OfferUUID:             offerUUID,
			Charm:                 charm,
		},
		RelationUUID: relationUUID1,
	})
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationAlreadyExists)
}

func (s *modelRemoteApplicationSuite) TestAddRemoteApplicationConsumerTwoApps(c *tc.C) {
	applicationUUID := tc.Must(c, internaluuid.NewUUID).String()
	charmUUID := tc.Must(c, internaluuid.NewUUID).String()
	offerUUID := tc.Must(c, internaluuid.NewUUID).String()
	relationUUID0 := tc.Must(c, internaluuid.NewUUID).String()
	relationUUID1 := tc.Must(c, internaluuid.NewUUID).String()

	// Local resources needed:
	localApplicationUUID := tc.Must(c, internaluuid.NewUUID).String()
	localCharmUUID := tc.Must(c, internaluuid.NewUUID).String()
	// Create an offer in the database.
	s.createOffer(c, offerUUID)
	// Create a charm in the database.
	s.createCharm(c, localCharmUUID)
	// Create an application in the database.
	s.createApplication(c, localApplicationUUID, localCharmUUID, offerUUID)

	charm1 := charm.Charm{
		ReferenceName: "bar",
		Source:        charm.CMRSource,
		Metadata: charm.Metadata{
			Name:        "foo",
			Description: "remote consumer application",
			Provides: map[string]charm.Relation{
				"db": {
					Name:      "db",
					Role:      charm.RoleProvider,
					Interface: "db",
					Limit:     1,
					Scope:     charm.ScopeGlobal,
				},
			},
			Requires: map[string]charm.Relation{
				"cache": {
					Name:      "cache",
					Role:      charm.RoleRequirer,
					Interface: "cacher",
					Scope:     charm.ScopeGlobal,
				},
			},
			Peers: map[string]charm.Relation{},
		},
	}

	charm2 := charm.Charm{
		ReferenceName: "baz",
		Source:        charm.CMRSource,
		Metadata: charm.Metadata{
			Name:        "bar",
			Description: "remote consumer application 2",
			Provides: map[string]charm.Relation{
				"db": {
					Name:      "db",
					Role:      charm.RoleProvider,
					Interface: "db",
					Limit:     1,
					Scope:     charm.ScopeGlobal,
				},
			},
			Requires: map[string]charm.Relation{
				"cache": {
					Name:      "cache",
					Role:      charm.RoleRequirer,
					Interface: "cacher",
					Scope:     charm.ScopeGlobal,
				},
			},
			Peers: map[string]charm.Relation{},
		},
	}

	err := s.state.AddRemoteApplicationConsumer(c.Context(), "foo", crossmodelrelation.AddRemoteApplicationConsumerArgs{
		AddRemoteApplicationArgs: crossmodelrelation.AddRemoteApplicationArgs{
			ApplicationUUID:       applicationUUID,
			CharmUUID:             charmUUID,
			RemoteApplicationUUID: tc.Must(c, internaluuid.NewUUID).String(),
			OfferUUID:             offerUUID,
			Charm:                 charm1,
		},
		RelationUUID: relationUUID0,
	})
	c.Assert(err, tc.ErrorIsNil)

	s.assertApplicationRemoteConsumer(c, applicationUUID)
	s.assertApplication(c, applicationUUID)
	s.assertCharmMetadata(c, applicationUUID, charmUUID, charm1)

	err = s.state.AddRemoteApplicationConsumer(c.Context(), "bar", crossmodelrelation.AddRemoteApplicationConsumerArgs{
		AddRemoteApplicationArgs: crossmodelrelation.AddRemoteApplicationArgs{
			ApplicationUUID:       tc.Must(c, internaluuid.NewUUID).String(),
			CharmUUID:             tc.Must(c, internaluuid.NewUUID).String(),
			RemoteApplicationUUID: tc.Must(c, internaluuid.NewUUID).String(),
			OfferUUID:             offerUUID,
			Charm:                 charm2,
		},
		RelationUUID: relationUUID1,
	})
	c.Assert(err, tc.ErrorIsNil) // Should succeed since different applications can consume same offer
}

func (s *modelRemoteApplicationSuite) TestAddRemoteApplicationConsumerCheckVersion(c *tc.C) {
	applicationUUID1 := tc.Must(c, internaluuid.NewUUID).String()
	applicationUUID2 := tc.Must(c, internaluuid.NewUUID).String()
	charmUUID1 := tc.Must(c, internaluuid.NewUUID).String()
	charmUUID2 := tc.Must(c, internaluuid.NewUUID).String()
	offerUUID := tc.Must(c, internaluuid.NewUUID).String()
	relationUUID0 := tc.Must(c, internaluuid.NewUUID).String()
	relationUUID1 := tc.Must(c, internaluuid.NewUUID).String()

	// Local resources needed:
	localApplicationUUID := tc.Must(c, internaluuid.NewUUID).String()
	localCharmUUID := tc.Must(c, internaluuid.NewUUID).String()
	// Create an offer in the database.
	s.createOffer(c, offerUUID)
	// Create a charm in the database.
	s.createCharm(c, localCharmUUID)
	// Create an application in the database.
	s.createApplication(c, localApplicationUUID, localCharmUUID, offerUUID)

	charm1 := charm.Charm{
		ReferenceName: "bar",
		Source:        charm.CMRSource,
		Metadata: charm.Metadata{
			Name:        "foo",
			Description: "remote consumer application",
		},
	}

	charm2 := charm.Charm{
		ReferenceName: "baz",
		Source:        charm.CMRSource,
		Metadata: charm.Metadata{
			Name:        "bar",
			Description: "remote consumer application 2",
		},
	}

	err := s.state.AddRemoteApplicationConsumer(c.Context(), "foo", crossmodelrelation.AddRemoteApplicationConsumerArgs{
		AddRemoteApplicationArgs: crossmodelrelation.AddRemoteApplicationArgs{
			ApplicationUUID:       applicationUUID1,
			CharmUUID:             charmUUID1,
			RemoteApplicationUUID: tc.Must(c, internaluuid.NewUUID).String(),
			OfferUUID:             offerUUID,
			Charm:                 charm1,
		},
		RelationUUID: relationUUID0,
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.AddRemoteApplicationConsumer(c.Context(), "bar", crossmodelrelation.AddRemoteApplicationConsumerArgs{
		AddRemoteApplicationArgs: crossmodelrelation.AddRemoteApplicationArgs{
			ApplicationUUID:       applicationUUID2,
			CharmUUID:             charmUUID2,
			RemoteApplicationUUID: tc.Must(c, internaluuid.NewUUID).String(),
			OfferUUID:             offerUUID,
			Charm:                 charm2,
		},
		RelationUUID: relationUUID1,
	})
	c.Assert(err, tc.ErrorIsNil)

	remoteApp1Version := s.getApplicationRemoteConsumerVersion(c, applicationUUID1)
	c.Check(remoteApp1Version, tc.Equals, uint64(0))

	remoteApp2Version := s.getApplicationRemoteConsumerVersion(c, applicationUUID2)
	c.Check(remoteApp2Version, tc.Equals, uint64(1)) // Same offer, so version increments
}

func (s *modelRemoteApplicationSuite) TestGetApplicationUUIDByOfferUUID(c *tc.C) {
	// Create application, charm and offer first
	applicationUUID := tc.Must(c, internaluuid.NewUUID).String()
	charmUUID := tc.Must(c, internaluuid.NewUUID).String()
	offerUUID := tc.Must(c, internaluuid.NewUUID).String()
	s.createOffer(c, offerUUID)
	s.createCharm(c, charmUUID)
	s.createApplication(c, applicationUUID, charmUUID, offerUUID)

	// Retrieve application UUID by offer UUID - should return the correct UUID
	gotName, gotUUID, err := s.state.GetApplicationNameAndUUIDByOfferUUID(c.Context(), offerUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotName, tc.Equals, "existing-app")
	c.Check(gotUUID.String(), tc.Equals, applicationUUID)
}

func (s *modelRemoteApplicationSuite) TestGetApplicationUUIDByOfferUUIDNotExists(c *tc.C) {
	// Test with non-existing offer UUID - should return application not found (no linked application)
	nonExistentUUID := tc.Must(c, internaluuid.NewUUID).String()
	_, _, err := s.state.GetApplicationNameAndUUIDByOfferUUID(c.Context(), nonExistentUUID)
	c.Assert(err, tc.ErrorMatches, "application not found")
}

func (s *modelRemoteApplicationSuite) TestGetRemoteApplicationConsumersSingle(c *tc.C) {
	offerUUID := tc.Must(c, internaluuid.NewUUID).String()
	relationUUID := tc.Must(c, internaluuid.NewUUID).String()
	appUUID := tc.Must(c, internaluuid.NewUUID).String()
	charmUUID := tc.Must(c, internaluuid.NewUUID).String()

	// Local (offerer-side) application setup.
	localAppUUID := tc.Must(c, internaluuid.NewUUID).String()
	localCharmUUID := tc.Must(c, internaluuid.NewUUID).String()
	s.createOffer(c, offerUUID)
	s.createCharm(c, localCharmUUID)
	s.createApplication(c, localAppUUID, localCharmUUID, offerUUID)

	ch := charm.Charm{
		ReferenceName: "rem-1",
		Source:        charm.CMRSource,
		Metadata: charm.Metadata{
			Name:        "foo",
			Description: "remote consumer application",
			Provides: map[string]charm.Relation{
				"db": {Name: "db", Role: charm.RoleProvider, Interface: "db", Limit: 1, Scope: charm.ScopeGlobal},
			},
			Requires: map[string]charm.Relation{},
			Peers:    map[string]charm.Relation{},
		},
	}

	err := s.state.AddRemoteApplicationConsumer(c.Context(), "foo", crossmodelrelation.AddRemoteApplicationConsumerArgs{
		AddRemoteApplicationArgs: crossmodelrelation.AddRemoteApplicationArgs{
			ApplicationUUID:       appUUID,
			CharmUUID:             charmUUID,
			RemoteApplicationUUID: tc.Must(c, internaluuid.NewUUID).String(),
			OfferUUID:             offerUUID,
			Charm:                 ch,
		},
		RelationUUID: relationUUID,
	})
	c.Assert(err, tc.ErrorIsNil)

	results, err := s.state.GetRemoteApplicationConsumers(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 1)
	c.Check(results[0].ApplicationName, tc.Equals, "foo")
	c.Check(results[0].OfferUUID, tc.Equals, offerUUID)
	c.Check(results[0].ConsumeVersion, tc.Equals, 0)
	c.Check(results[0].Life, tc.Equals, life.Alive)
}

func (s *modelRemoteApplicationSuite) TestGetRemoteApplicationConsumersMultiple(c *tc.C) {
	offerUUID := tc.Must(c, internaluuid.NewUUID).String()
	relationUUID1 := tc.Must(c, internaluuid.NewUUID).String()
	relationUUID2 := tc.Must(c, internaluuid.NewUUID).String()
	appUUID1 := tc.Must(c, internaluuid.NewUUID).String()
	appUUID2 := tc.Must(c, internaluuid.NewUUID).String()
	charmUUID1 := tc.Must(c, internaluuid.NewUUID).String()
	charmUUID2 := tc.Must(c, internaluuid.NewUUID).String()

	localAppUUID := tc.Must(c, internaluuid.NewUUID).String()
	localCharmUUID := tc.Must(c, internaluuid.NewUUID).String()
	s.createOffer(c, offerUUID)
	s.createCharm(c, localCharmUUID)
	s.createApplication(c, localAppUUID, localCharmUUID, offerUUID)

	makeCharm := func(name string) charm.Charm {
		return charm.Charm{
			ReferenceName: name,
			Source:        charm.CMRSource,
			Metadata: charm.Metadata{
				Name:        name,
				Description: "remote consumer app",
				Provides: map[string]charm.Relation{
					"db": {Name: "db", Role: charm.RoleProvider, Interface: "db", Limit: 1, Scope: charm.ScopeGlobal},
				},
				Requires: map[string]charm.Relation{},
				Peers:    map[string]charm.Relation{},
			},
		}
	}

	err := s.state.AddRemoteApplicationConsumer(c.Context(), "foo", crossmodelrelation.AddRemoteApplicationConsumerArgs{
		AddRemoteApplicationArgs: crossmodelrelation.AddRemoteApplicationArgs{
			ApplicationUUID:       appUUID1,
			CharmUUID:             charmUUID1,
			RemoteApplicationUUID: tc.Must(c, internaluuid.NewUUID).String(),
			OfferUUID:             offerUUID,
			Charm:                 makeCharm("foo"),
		},
		RelationUUID: relationUUID1,
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.AddRemoteApplicationConsumer(c.Context(), "bar", crossmodelrelation.AddRemoteApplicationConsumerArgs{
		AddRemoteApplicationArgs: crossmodelrelation.AddRemoteApplicationArgs{
			ApplicationUUID:       appUUID2,
			CharmUUID:             charmUUID2,
			RemoteApplicationUUID: tc.Must(c, internaluuid.NewUUID).String(),
			OfferUUID:             offerUUID,
			Charm:                 makeCharm("bar"),
		},
		RelationUUID: relationUUID2,
	})
	c.Assert(err, tc.ErrorIsNil)

	results, err := s.state.GetRemoteApplicationConsumers(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 2)

	// Order not strictly guaranteed; map by name.
	found := map[string]crossmodelrelation.RemoteApplicationConsumer{}
	for _, r := range results {
		found[r.ApplicationName] = r
	}

	foo := found["foo"]
	bar := found["bar"]

	// Versions should increment per offer (shared offerUUID).
	if foo.ConsumeVersion == 0 {
		c.Check(bar.ConsumeVersion, tc.Equals, 1)
	} else {
		c.Check(foo.ConsumeVersion, tc.Equals, 1)
		c.Check(bar.ConsumeVersion, tc.Equals, 0)
	}

	c.Check(foo.OfferUUID, tc.Equals, offerUUID)
	c.Check(bar.OfferUUID, tc.Equals, offerUUID)
}

func (s *modelRemoteApplicationSuite) TestGetRemoteApplicationConsumersEmpty(c *tc.C) {
	results, err := s.state.GetRemoteApplicationConsumers(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(results, tc.HasLen, 0)
}

func (s *modelRemoteApplicationSuite) TestGetRemoteApplicationConsumersFiltersDead(c *tc.C) {
	offerUUID := tc.Must(c, internaluuid.NewUUID).String()
	relationUUID := tc.Must(c, internaluuid.NewUUID).String()
	appUUID := tc.Must(c, internaluuid.NewUUID).String()
	charmUUID := tc.Must(c, internaluuid.NewUUID).String()

	localAppUUID := tc.Must(c, internaluuid.NewUUID).String()
	localCharmUUID := tc.Must(c, internaluuid.NewUUID).String()
	s.createOffer(c, offerUUID)
	s.createCharm(c, localCharmUUID)
	s.createApplication(c, localAppUUID, localCharmUUID, offerUUID)

	ch := charm.Charm{
		ReferenceName: "dead-one",
		Source:        charm.CMRSource,
		Metadata: charm.Metadata{
			Name:        "dead-one",
			Description: "remote consumer application",
			Provides: map[string]charm.Relation{
				"db": {Name: "db", Role: charm.RoleProvider, Interface: "db", Limit: 1, Scope: charm.ScopeGlobal},
			},
			Requires: map[string]charm.Relation{},
			Peers:    map[string]charm.Relation{},
		},
	}

	err := s.state.AddRemoteApplicationConsumer(c.Context(), "dead-one", crossmodelrelation.AddRemoteApplicationConsumerArgs{
		AddRemoteApplicationArgs: crossmodelrelation.AddRemoteApplicationArgs{
			ApplicationUUID:       appUUID,
			CharmUUID:             charmUUID,
			RemoteApplicationUUID: tc.Must(c, internaluuid.NewUUID).String(),
			OfferUUID:             offerUUID,
			Charm:                 ch,
		},
		RelationUUID: relationUUID,
	})
	c.Assert(err, tc.ErrorIsNil)

	// Mark consumer record as Dead (life_id = 2) so it should be filtered out.
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, e := tx.ExecContext(ctx, `
UPDATE application_remote_consumer
SET life_id = 2
WHERE consumer_application_uuid = ?`, appUUID)
		return e
	})
	c.Assert(err, tc.ErrorIsNil)

	results, err := s.state.GetRemoteApplicationConsumers(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(results, tc.HasLen, 0)
}

func (s *modelRemoteApplicationSuite) assertApplicationRemoteConsumer(c *tc.C, applicationUUID string) {
	var count int
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRow(`
SELECT COUNT(*)
FROM application_remote_consumer
WHERE consumer_application_uuid = ?
`, applicationUUID).Scan(&count)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 1)
}

func (s *modelRemoteApplicationSuite) getApplicationRemoteConsumerVersion(c *tc.C, applicationUUID string) uint64 {

	var version uint64
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRow(`
SELECT version
FROM application_remote_consumer
WHERE consumer_application_uuid = ?
`, applicationUUID).Scan(&version)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	return version
}

func (s *modelRemoteApplicationSuite) createOffer(c *tc.C, offerUUID string) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		// Create an offer record
		_, err := tx.Exec(`
INSERT INTO offer (uuid, name)
VALUES (?, 'test-offer')
`, offerUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelRemoteApplicationSuite) createApplication(c *tc.C, applicationUUID string, charmUUID string, offerUUID string) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		// Create an application record
		_, err := tx.Exec(`
INSERT INTO application (uuid, name, charm_uuid, life_id, space_uuid)
VALUES (?, 'existing-app', ?, 0, ?)
`, applicationUUID, charmUUID, network.AlphaSpaceId)
		if err != nil {
			return err
		}
		// Insert charm_relation and application_endpoint records
		insertCharmRelation := `INSERT INTO charm_relation (uuid, charm_uuid, scope_id, role_id, name) VALUES (?, ?, ?, ?, ?)`
		_, err = tx.ExecContext(ctx, insertCharmRelation, "charm-relation0-uuid", charmUUID, "0", "0", "endpoint0")
		if err != nil {
			return err
		}
		insertEndpoint := `INSERT INTO application_endpoint (uuid, application_uuid, space_uuid, charm_relation_uuid) VALUES (?, ?, ?, ?)`
		_, err = tx.ExecContext(ctx, insertEndpoint, "app-endpoint0-uuid", applicationUUID, network.AlphaSpaceId, "charm-relation0-uuid")
		if err != nil {
			return err
		}
		// Insert an offer endpoint record
		insertOfferEndpoint := `INSERT INTO offer_endpoint (offer_uuid, endpoint_uuid) VALUES (?, ?)`
		_, err = tx.ExecContext(ctx, insertOfferEndpoint, offerUUID, "app-endpoint0-uuid")
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelRemoteApplicationSuite) createCharm(c *tc.C, charmUUID string) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		// Create a charm record
		_, err := tx.Exec(`
INSERT INTO charm (uuid, reference_name, source_id)
VALUES (?, 'existing-charm', 1)
`, charmUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelRemoteApplicationSuite) assertApplicationRemoteOfferer(c *tc.C, uuid string) {
	var (
		gotLifeID   int
		gotVersion  int
		gotMacaroon string
	)
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `
SELECT aro.life_id, aro.version, aro.macaroon 
FROM application_remote_offerer AS aro
JOIN application AS a ON aro.application_uuid = a.uuid
WHERE a.uuid=?`, uuid).
			Scan(&gotLifeID, &gotVersion, &gotMacaroon)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotLifeID, tc.Equals, 0)
	c.Check(gotVersion, tc.Equals, 0)
	c.Check(gotMacaroon, tc.Equals, "encoded macaroon")
}

func (s *modelRemoteApplicationSuite) assertApplicationRemoteOffererStatus(c *tc.C, uuid string) {
	var gotStatusID int
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `
SELECT aros.status_id 
FROM application_remote_offerer_status AS aros
JOIN application_remote_offerer AS aro ON aros.application_remote_offerer_uuid = aro.uuid
JOIN application AS a ON aro.application_uuid = a.uuid
WHERE a.uuid=?`, uuid).
			Scan(&gotStatusID)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotStatusID, tc.Equals, 1)
}

func (s *modelRemoteApplicationSuite) assertApplication(c *tc.C, uuid string) {
	var (
		gotName      string
		gotUUID      string
		gotCharmUUID string
	)
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT uuid, charm_uuid, name FROM application WHERE uuid=?", uuid).
			Scan(&gotUUID, &gotCharmUUID, &gotName)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotName, tc.Equals, "foo")
}

func (s *modelRemoteApplicationSuite) assertRelation(c *tc.C, relationUUID string, relationID int) {
	var (
		gotUUID    string
		gotID      int
		gotLifID   int
		gotScopeID int
	)
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT * FROM relation WHERE relation_id=?", relationID).
			Scan(&gotUUID, &gotID, &gotLifID, &gotScopeID)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotUUID, tc.Equals, relationUUID)
	c.Check(gotID, tc.Equals, relationID)
	c.Check(gotLifID, tc.Equals, 0)   // life.Alive
	c.Check(gotScopeID, tc.Equals, 0) // scope.Global
}

func (s *modelRemoteApplicationSuite) assertCharmMetadata(c *tc.C, appUUID, charmUUID string, expected charm.Charm) {
	var (
		gotReferenceName string
		gotSourceName    string
		gotCharmName     string

		gotProvides = make(map[string]charm.Relation)
		gotRequires = make(map[string]charm.Relation)
	)
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `
SELECT ch.reference_name, cs.name, cm.name
FROM application
JOIN charm AS ch ON application.charm_uuid = ch.uuid
JOIN charm_metadata AS cm ON ch.uuid = cm.charm_uuid
JOIN charm_source AS cs ON ch.source_id = cs.id
WHERE application.uuid=?`, appUUID).
			Scan(&gotReferenceName, &gotSourceName, &gotCharmName)
		if err != nil {
			return err
		}

		rows, err := tx.QueryContext(ctx, `
SELECT name, role_id, interface, capacity, scope_id
FROM charm_relation
WHERE charm_uuid = ?`, charmUUID)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()

		for rows.Next() {
			var (
				relName  string
				roleID   int
				iface    string
				capacity int
				scopeID  int
			)
			if err := rows.Scan(&relName, &roleID, &iface, &capacity, &scopeID); err != nil {
				return err
			}
			rel := charm.Relation{
				Name:      relName,
				Interface: iface,
				Limit:     capacity,
			}
			switch roleID {
			case 0:
				rel.Role = charm.RoleProvider
			case 1:
				rel.Role = charm.RoleRequirer
			default:
				return internalerrors.Errorf("unknown role ID %d", roleID)
			}
			switch scopeID {
			case 0:
				rel.Scope = charm.ScopeGlobal
			default:
				return internalerrors.Errorf("unknown scope ID %d", scopeID)
			}
			switch rel.Role {
			case charm.RoleProvider:
				gotProvides[rel.Name] = rel
			case charm.RoleRequirer:
				gotRequires[rel.Name] = rel
			}
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Check(gotReferenceName, tc.Equals, expected.ReferenceName)
	c.Check(gotSourceName, tc.Equals, "cmr")
	c.Check(gotCharmName, tc.Equals, expected.Metadata.Name)

	// Every remote application will automatically get a "juju-info" provider
	// relation.
	// Check that it has been added correctly.
	provides := make(map[string]charm.Relation)
	maps.Copy(provides, expected.Metadata.Provides)
	provides["juju-info"] = charm.Relation{
		Name:      "juju-info",
		Role:      charm.RoleProvider,
		Interface: "juju-info",
		Limit:     0,
		Scope:     charm.ScopeGlobal,
	}

	c.Check(gotProvides, tc.DeepEquals, provides)
	c.Check(gotRequires, tc.DeepEquals, expected.Metadata.Requires)
}

type applicationEndpoint struct {
	charmRelationUUID string
	charmRelationName string
	spaceName         string
}

func (s *modelRemoteApplicationSuite) fetchApplicationEndpoints(c *tc.C, appID string) []applicationEndpoint {
	var endpoints []applicationEndpoint
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.Query(`
SELECT ae.charm_relation_uuid, s.name, cr.name AS charm_relation_name
FROM application_endpoint ae
JOIN charm_relation cr ON cr.uuid=ae.charm_relation_uuid
LEFT JOIN space s ON s.uuid=ae.space_uuid
WHERE ae.application_uuid=?
ORDER BY cr.name`, appID)
		defer func() { _ = rows.Close() }()
		if err != nil {
			return err
		}
		for rows.Next() {
			var (
				uuid              string
				spaceName         *string
				charmRelationName string
			)
			if err := rows.Scan(&uuid, &spaceName, &charmRelationName); err != nil {
				return err
			}
			endpoints = append(endpoints, applicationEndpoint{
				charmRelationUUID: uuid,
				charmRelationName: charmRelationName,
				spaceName:         nilEmpty(spaceName),
			})
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	return endpoints
}

func (s *modelRemoteApplicationSuite) TestGetConsumerRelationUUIDsFiltering(c *tc.C) {
	ctx := c.Context()

	// Create a remote offerer application with at least one endpoint (juju-info
	// is added automatically).
	remoteAppUUID := tc.Must(c, internaluuid.NewUUID).String()
	remoteCharmUUID := tc.Must(c, internaluuid.NewUUID).String()
	remoteOfferUUID := tc.Must(c, internaluuid.NewUUID).String()
	remoteCharm := charm.Charm{
		ReferenceName: "remote-ref",
		Source:        charm.CMRSource,
		Metadata: charm.Metadata{
			Name:        "remote-app",
			Description: "remote offerer",
			// No relations specified; juju-info will be auto-added.
			Provides: map[string]charm.Relation{},
			Requires: map[string]charm.Relation{},
			Peers:    map[string]charm.Relation{},
		},
	}
	err := s.state.AddRemoteApplicationOfferer(ctx, "remote-app", crossmodelrelation.AddRemoteApplicationOffererArgs{
		AddRemoteApplicationArgs: crossmodelrelation.AddRemoteApplicationArgs{
			ApplicationUUID:       remoteAppUUID,
			CharmUUID:             remoteCharmUUID,
			RemoteApplicationUUID: tc.Must(c, internaluuid.NewUUID).String(),
			OfferUUID:             remoteOfferUUID,
			Charm:                 remoteCharm,
		},
		EncodedMacaroon: []byte("m"),
	})
	c.Assert(err, tc.ErrorIsNil)

	remoteEndpointUUID := s.getAppEndpointUUID(c, remoteAppUUID, "juju-info")

	// Create a non-offerer local application and one endpoint named "endpoint0".
	localOfferUUID := tc.Must(c, internaluuid.NewUUID).String()
	s.createOffer(c, localOfferUUID)
	localCharmUUID := tc.Must(c, internaluuid.NewUUID).String()
	s.createCharm(c, localCharmUUID)
	localAppUUID := tc.Must(c, internaluuid.NewUUID).String()
	s.createApplication(c, localAppUUID, localCharmUUID, localOfferUUID)
	localEndpointUUID := s.getAppEndpointUUID(c, localAppUUID, "endpoint0")

	// Create two relations: one linked to the remote offerer app endpoint, one
	// to the local app endpoint.
	remoteRelUUID := s.addRelation(c).String()
	localRelUUID := s.addRelation(c).String()

	s.query(c, `
INSERT INTO relation_endpoint (uuid, relation_uuid, endpoint_uuid)
VALUES (?, ?, ?)`, internaluuid.MustNewUUID().String(), remoteRelUUID, remoteEndpointUUID)
	s.query(c, `
INSERT INTO relation_endpoint (uuid, relation_uuid, endpoint_uuid)
VALUES (?, ?, ?)`, internaluuid.MustNewUUID().String(), localRelUUID, localEndpointUUID)

	// Filter both relation UUIDs; expect only the remote relation returned.
	got, err := s.state.GetConsumerRelationUUIDs(ctx, remoteRelUUID, localRelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.HasLen, 1)
	c.Check(got[0], tc.Equals, remoteRelUUID)
}

func (s *modelRemoteApplicationSuite) TestGetConsumerRelationUUIDsEmptyArgsReturnsAll(c *tc.C) {
	ctx := c.Context()

	// Create a remote offerer application with two endpoints: juju-info and db.
	remoteAppUUID := tc.Must(c, internaluuid.NewUUID).String()
	remoteCharmUUID := tc.Must(c, internaluuid.NewUUID).String()
	remoteOfferUUID := tc.Must(c, internaluuid.NewUUID).String()
	remoteCharm := charm.Charm{
		ReferenceName: "remote-ref-2",
		Source:        charm.CMRSource,
		Metadata: charm.Metadata{
			Name:        "remote-app-2",
			Description: "remote offerer",
			Provides: map[string]charm.Relation{
				"db": {
					Name:      "db",
					Role:      charm.RoleProvider,
					Interface: "db",
					Limit:     1,
					Scope:     charm.ScopeGlobal,
				},
			},
			Requires: map[string]charm.Relation{},
			Peers:    map[string]charm.Relation{},
		},
	}
	err := s.state.AddRemoteApplicationOfferer(ctx, "remote-app-2", crossmodelrelation.AddRemoteApplicationOffererArgs{
		AddRemoteApplicationArgs: crossmodelrelation.AddRemoteApplicationArgs{
			ApplicationUUID:       remoteAppUUID,
			CharmUUID:             remoteCharmUUID,
			RemoteApplicationUUID: tc.Must(c, internaluuid.NewUUID).String(),
			OfferUUID:             remoteOfferUUID,
			Charm:                 remoteCharm,
		},
		EncodedMacaroon: []byte("m"),
	})
	c.Assert(err, tc.ErrorIsNil)

	epJujuInfo := s.getAppEndpointUUID(c, remoteAppUUID, "juju-info")
	epDB := s.getAppEndpointUUID(c, remoteAppUUID, "db")

	// Create two relations both associated with the remote offerer app.
	rel1 := s.addRelation(c).String()
	rel2 := s.addRelation(c).String()

	s.query(c, `
INSERT INTO relation_endpoint (uuid, relation_uuid, endpoint_uuid)
VALUES (?, ?, ?)`, internaluuid.MustNewUUID().String(), rel1, epJujuInfo)
	s.query(c, `
INSERT INTO relation_endpoint (uuid, relation_uuid, endpoint_uuid)
VALUES (?, ?, ?)`, internaluuid.MustNewUUID().String(), rel2, epDB)

	// Call with no args; expect both relation UUIDs returned (order not
	// guaranteed).
	got, err := s.state.GetConsumerRelationUUIDs(ctx)
	c.Assert(err, tc.ErrorIsNil)

	gotSet := map[string]struct{}{}
	for _, r := range got {
		gotSet[r] = struct{}{}
	}
	c.Check(gotSet, tc.DeepEquals, map[string]struct{}{rel1: {}, rel2: {}})
}

func (s *modelRemoteApplicationSuite) TestGetConsumerRelationUUIDsNoMatches(c *tc.C) {
	ctx := c.Context()

	// Create a local (non-offerer) application and a relation for it.
	localOfferUUID := tc.Must(c, internaluuid.NewUUID).String()
	s.createOffer(c, localOfferUUID)
	localCharmUUID := tc.Must(c, internaluuid.NewUUID).String()
	s.createCharm(c, localCharmUUID)
	localAppUUID := tc.Must(c, internaluuid.NewUUID).String()
	s.createApplication(c, localAppUUID, localCharmUUID, localOfferUUID)
	localEndpointUUID := s.getAppEndpointUUID(c, localAppUUID, "endpoint0")

	relLocal := s.addRelation(c).String()
	s.query(c, `
INSERT INTO relation_endpoint (uuid, relation_uuid, endpoint_uuid)
VALUES (?, ?, ?)`, internaluuid.MustNewUUID().String(), relLocal, localEndpointUUID)

	// Pass only non-offerer relation UUIDs; expect empty result.
	got, err := s.state.GetConsumerRelationUUIDs(ctx, relLocal)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.HasLen, 0)
}

func (s *modelRemoteApplicationSuite) TestGetConsumerRelationUUIDsDeduplicates(c *tc.C) {
	ctx := c.Context()

	// Create a remote offerer application with two endpoints (juju-info and
	// db).
	remoteAppUUID := tc.Must(c, internaluuid.NewUUID).String()
	remoteCharmUUID := tc.Must(c, internaluuid.NewUUID).String()
	remoteOfferUUID := tc.Must(c, internaluuid.NewUUID).String()
	remoteCharm := charm.Charm{
		ReferenceName: "remote-ref-3",
		Source:        charm.CMRSource,
		Metadata: charm.Metadata{
			Name:        "remote-app-3",
			Description: "remote offerer",
			Provides: map[string]charm.Relation{
				"db": {
					Name:      "db",
					Role:      charm.RoleProvider,
					Interface: "db",
					Limit:     1,
					Scope:     charm.ScopeGlobal,
				},
			},
			Requires: map[string]charm.Relation{},
			Peers:    map[string]charm.Relation{},
		},
	}
	err := s.state.AddRemoteApplicationOfferer(ctx, "remote-app-3", crossmodelrelation.AddRemoteApplicationOffererArgs{
		AddRemoteApplicationArgs: crossmodelrelation.AddRemoteApplicationArgs{
			ApplicationUUID:       remoteAppUUID,
			CharmUUID:             remoteCharmUUID,
			RemoteApplicationUUID: tc.Must(c, internaluuid.NewUUID).String(),
			OfferUUID:             remoteOfferUUID,
			Charm:                 remoteCharm,
		},
		EncodedMacaroon: []byte("m"),
	})
	c.Assert(err, tc.ErrorIsNil)

	epJujuInfo := s.getAppEndpointUUID(c, remoteAppUUID, "juju-info")
	epDB := s.getAppEndpointUUID(c, remoteAppUUID, "db")

	// Create a single relation that is linked to two endpoints of the same
	// remote application.
	rel := s.addRelation(c).String()
	s.query(c, `
INSERT INTO relation_endpoint (uuid, relation_uuid, endpoint_uuid)
VALUES (?, ?, ?)`, internaluuid.MustNewUUID().String(), rel, epJujuInfo)
	s.query(c, `
INSERT INTO relation_endpoint (uuid, relation_uuid, endpoint_uuid)
VALUES (?, ?, ?)`, internaluuid.MustNewUUID().String(), rel, epDB)

	// Provide duplicate relation UUIDs in input; expect a single entry in
	// output.
	got, err := s.state.GetConsumerRelationUUIDs(ctx, rel, rel)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.HasLen, 1)
	c.Check(got[0], tc.Equals, rel)
}

// getAppEndpointUUID returns the application_endpoint.uuid for the given
// application and charm relation (endpoint) name.
func (s *modelRemoteApplicationSuite) getAppEndpointUUID(c *tc.C, applicationUUID string, endpointName string) string {
	var epUUID string
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `
SELECT ae.uuid
FROM application_endpoint AS ae
JOIN charm_relation AS cr ON cr.uuid = ae.charm_relation_uuid
WHERE ae.application_uuid = ? AND cr.name = ?`, applicationUUID, endpointName).Scan(&epUUID)
	})
	c.Assert(err, tc.ErrorIsNil)
	return epUUID
}
