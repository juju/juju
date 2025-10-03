// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"maps"
	"testing"

	"github.com/juju/tc"

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

	// Create an offer in the database
	s.createOffer(c, offerUUID)

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

	// Create an offer in the database
	s.createOffer(c, offerUUID)

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

	// Create an offer in the database
	s.createOffer(c, offerUUID)

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

	// Create an offer in the database
	s.createOffer(c, offerUUID)

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

	remoteApp1 := s.getApplicationRemoteConsumer(c, applicationUUID1)
	c.Check(remoteApp1.Version, tc.Equals, uint64(0))

	remoteApp2 := s.getApplicationRemoteConsumer(c, applicationUUID2)
	c.Check(remoteApp2.Version, tc.Equals, uint64(1)) // Same offer, so version increments
}

func (s *modelRemoteApplicationSuite) assertApplicationRemoteConsumer(c *tc.C, applicationUUID string) {
	var count int
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRow(`
SELECT COUNT(*)
FROM application_remote_consumer
WHERE application_uuid = ?
`, applicationUUID).Scan(&count)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 1)
}

func (s *modelRemoteApplicationSuite) getApplicationRemoteConsumer(c *tc.C, applicationUUID string) struct {
	UUID                string
	ApplicationUUID     string
	OfferConnectionUUID string
	Version             uint64
} {
	var result struct {
		UUID                string
		ApplicationUUID     string
		OfferConnectionUUID string
		Version             uint64
	}
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRow(`
SELECT uuid, application_uuid, offer_connection_uuid, version
FROM application_remote_consumer
WHERE application_uuid = ?
`, applicationUUID).Scan(&result.UUID, &result.ApplicationUUID, &result.OfferConnectionUUID, &result.Version)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	return result
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
