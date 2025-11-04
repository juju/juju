// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"maps"
	"testing"

	"github.com/juju/collections/set"
	"github.com/juju/tc"

	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	coreoffer "github.com/juju/juju/core/offer"
	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/crossmodelrelation"
	crossmodelrelationerrors "github.com/juju/juju/domain/crossmodelrelation/errors"
	"github.com/juju/juju/domain/life"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	"github.com/juju/juju/domain/status"
	internalcharm "github.com/juju/juju/internal/charm"
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
		ApplicationUUID:       applicationUUID,
		CharmUUID:             charmUUID,
		RemoteApplicationUUID: tc.Must(c, internaluuid.NewUUID).String(),
		OfferUUID:             tc.Must(c, internaluuid.NewUUID).String(),
		Charm:                 charm,
		EncodedMacaroon:       []byte("encoded macaroon"),
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
		ApplicationUUID:       applicationUUID,
		CharmUUID:             charmUUID,
		RemoteApplicationUUID: tc.Must(c, internaluuid.NewUUID).String(),
		OfferUUID:             tc.Must(c, internaluuid.NewUUID).String(),
		Charm:                 charm,
		EncodedMacaroon:       []byte("encoded macaroon"),
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
		ApplicationUUID:       applicationUUID,
		CharmUUID:             charmUUID,
		RemoteApplicationUUID: tc.Must(c, internaluuid.NewUUID).String(),
		OfferUUID:             tc.Must(c, internaluuid.NewUUID).String(),
		Charm:                 charm,
		EncodedMacaroon:       []byte("encoded macaroon"),
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
		ApplicationUUID:       applicationUUID,
		CharmUUID:             charmUUID,
		RemoteApplicationUUID: tc.Must(c, internaluuid.NewUUID).String(),
		OfferUUID:             tc.Must(c, internaluuid.NewUUID).String(),
		Charm:                 charm,
		EncodedMacaroon:       []byte("encoded macaroon"),
	})
	c.Assert(err, tc.ErrorIsNil)

	s.assertApplicationRemoteOfferer(c, applicationUUID)
	s.assertApplication(c, applicationUUID)
	s.assertCharmMetadata(c, applicationUUID, charmUUID, charm)

	err = s.state.AddRemoteApplicationOfferer(c.Context(), "foo", crossmodelrelation.AddRemoteApplicationOffererArgs{
		ApplicationUUID:       applicationUUID,
		CharmUUID:             charmUUID,
		RemoteApplicationUUID: tc.Must(c, internaluuid.NewUUID).String(),
		OfferUUID:             tc.Must(c, internaluuid.NewUUID).String(),
		Charm:                 charm,
		EncodedMacaroon:       []byte("encoded macaroon"),
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
		ApplicationUUID:       applicationUUID,
		CharmUUID:             charmUUID,
		RemoteApplicationUUID: tc.Must(c, internaluuid.NewUUID).String(),
		OfferUUID:             offerUUID,
		Charm:                 charm,
		EncodedMacaroon:       []byte("encoded macaroon"),
	})
	c.Assert(err, tc.ErrorIsNil)

	s.assertApplicationRemoteOfferer(c, applicationUUID)
	s.assertApplication(c, applicationUUID)
	s.assertCharmMetadata(c, applicationUUID, charmUUID, charm)

	err = s.state.AddRemoteApplicationOfferer(c.Context(), "bar", crossmodelrelation.AddRemoteApplicationOffererArgs{
		ApplicationUUID:       applicationUUID,
		CharmUUID:             charmUUID,
		RemoteApplicationUUID: tc.Must(c, internaluuid.NewUUID).String(),
		OfferUUID:             offerUUID,
		Charm:                 charm,
		EncodedMacaroon:       []byte("encoded macaroon"),
	})
	c.Assert(err, tc.ErrorIs, crossmodelrelationerrors.OfferAlreadyConsumed)
}

// TestAddRemoteApplicationOffererInsertsApplicationAndCharmTwiceSameOfferUUIDSameName
// tests the scenario where a client attempts to consume an offer which has
// already been consumed.
func (s *modelRemoteApplicationSuite) TestAddRemoteApplicationOffererInsertsApplicationAndCharmTwiceSameOfferUUIDSameName(c *tc.C) {
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
		ApplicationUUID:       applicationUUID,
		CharmUUID:             charmUUID,
		RemoteApplicationUUID: tc.Must(c, internaluuid.NewUUID).String(),
		OfferUUID:             offerUUID,
		Charm:                 charm,
		EncodedMacaroon:       []byte("encoded macaroon"),
	})
	c.Assert(err, tc.ErrorIsNil)

	s.assertApplicationRemoteOfferer(c, applicationUUID)
	s.assertApplication(c, applicationUUID)
	s.assertCharmMetadata(c, applicationUUID, charmUUID, charm)

	err = s.state.AddRemoteApplicationOfferer(c.Context(), "foo", crossmodelrelation.AddRemoteApplicationOffererArgs{
		ApplicationUUID:       applicationUUID,
		CharmUUID:             charmUUID,
		RemoteApplicationUUID: tc.Must(c, internaluuid.NewUUID).String(),
		OfferUUID:             offerUUID,
		Charm:                 charm,
		EncodedMacaroon:       []byte("encoded macaroon"),
	})
	c.Assert(err, tc.ErrorIs, crossmodelrelationerrors.OfferAlreadyConsumed)
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
		ApplicationUUID:       applicationUUID,
		CharmUUID:             charmUUID,
		RemoteApplicationUUID: tc.Must(c, internaluuid.NewUUID).String(),
		OfferUUID:             offerUUID,
		Charm:                 charm,
		OffererModelUUID:      offererModelUUID,
		OfferURL:              "controller:qualifier/model.offername",
		EncodedMacaroon:       macBytes,
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

func (s *modelRemoteApplicationSuite) TestGetRemoteApplicationOffererByApplicationName(c *tc.C) {
	applicationUUID := tc.Must(c, internaluuid.NewUUID).String()
	charmUUID := tc.Must(c, internaluuid.NewUUID).String()
	offerUUID := tc.Must(c, internaluuid.NewUUID).String()
	offererModelUUID := tc.Must(c, internaluuid.NewUUID).String()
	remoteApplicationUUID := tc.Must(c, internaluuid.NewUUID).String()

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
		ApplicationUUID:       applicationUUID,
		CharmUUID:             charmUUID,
		RemoteApplicationUUID: remoteApplicationUUID,
		OfferUUID:             offerUUID,
		Charm:                 charm,
		OffererModelUUID:      offererModelUUID,
		OfferURL:              "controller:qualifier/model.offername",
		EncodedMacaroon:       macBytes,
	})
	c.Assert(err, tc.ErrorIsNil)

	result, err := s.state.GetRemoteApplicationOffererByApplicationName(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.Equals, remoteApplicationUUID)
}

func (s *modelRemoteApplicationSuite) TestGetRemoteApplicationOffererByApplicationNameNotFound(c *tc.C) {
	// Initially there are no remote application offerers.
	result, err := s.state.GetRemoteApplicationOffererByApplicationName(c.Context(), "foo")
	c.Assert(err, tc.ErrorIs, crossmodelrelationerrors.RemoteApplicationNotFound)
	c.Check(result, tc.Equals, "")
}

func (s *modelRemoteApplicationSuite) TestAddConsumedRelation(c *tc.C) {
	charmUUID := tc.Must(c, internaluuid.NewUUID).String()
	offerUUID := tc.Must(c, internaluuid.NewUUID).String()
	relationUUID := tc.Must(c, internaluuid.NewUUID).String()
	consumerModelUUID := tc.Must(c, internaluuid.NewUUID).String()
	consumerApplicationUUID := tc.Must(c, internaluuid.NewUUID).String()
	synthApplicationUUID := tc.Must(c, internaluuid.NewUUID).String()

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
			Requires: map[string]charm.Relation{},
			Peers:    map[string]charm.Relation{},
		},
	}
	err := s.state.AddConsumedRelation(c.Context(), "foo", crossmodelrelation.AddRemoteApplicationConsumerArgs{
		OfferUUID:                   offerUUID,
		OfferEndpointName:           "offer-endpoint",
		RelationUUID:                relationUUID,
		ConsumerModelUUID:           consumerModelUUID,
		ConsumerApplicationUUID:     consumerApplicationUUID,
		ConsumerApplicationEndpoint: "db",
		SynthApplicationUUID:        synthApplicationUUID,
		CharmUUID:                   charmUUID,
		Charm:                       charm,
		Username:                    "consumer-user",
	})
	c.Assert(err, tc.ErrorIsNil)

	s.assertApplicationRemoteConsumer(c, consumerApplicationUUID)
	s.assertApplication(c, synthApplicationUUID)
	s.assertCharmMetadata(c, synthApplicationUUID, charmUUID, charm)

	endpoints := s.fetchApplicationEndpoints(c, synthApplicationUUID)
	if c.Check(endpoints, tc.HasLen, 2) {
		c.Check(endpoints[0].charmRelationName, tc.Equals, "db")
		c.Check(endpoints[1].charmRelationName, tc.Equals, "juju-info")
	}

	// Check that the synthetic relation has been created with the expected
	// UUID and ID 0 (the first relation created in the model).
	s.assertRelation(c, relationUUID, 0)

	s.assertRelationEndpoints(c, relationUUID, offerApplicationUUID.String(), synthApplicationUUID)

	s.assertRelationStatusJoining(c, relationUUID)
}

func (s *modelRemoteApplicationSuite) TestAddConsumedRelationEndpointNotFound(c *tc.C) {
	charm := charm.Charm{
		ReferenceName: "bar",
		Source:        charm.CMRSource,
		Metadata: charm.Metadata{
			Name:        "foo",
			Description: "remote consumer application",
			Provides: map[string]charm.Relation{
				"seven": {
					Name:      "seven",
					Role:      charm.RoleProvider,
					Interface: "seven",
					Limit:     1,
					Scope:     charm.ScopeGlobal,
				},
			},
			Requires: map[string]charm.Relation{},
			Peers:    map[string]charm.Relation{},
		},
	}
	err := s.state.AddConsumedRelation(c.Context(), "foo", crossmodelrelation.AddRemoteApplicationConsumerArgs{
		ConsumerApplicationEndpoint: "db",
		Charm:                       charm,
	})
	c.Assert(err, tc.ErrorIs, relationerrors.RelationEndpointNotFound)
}

func (s *modelRemoteApplicationSuite) TestAddConsumedRelationOneAndOnlyOneEndpoint(c *tc.C) {
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
				"seven": {
					Name:      "seven",
					Role:      charm.RoleProvider,
					Interface: "seven",
					Limit:     1,
					Scope:     charm.ScopeGlobal,
				},
			},
			Requires: map[string]charm.Relation{},
			Peers:    map[string]charm.Relation{},
		},
	}
	err := s.state.AddConsumedRelation(c.Context(), "foo", crossmodelrelation.AddRemoteApplicationConsumerArgs{
		ConsumerApplicationEndpoint: "db",
		Charm:                       charm,
	})
	c.Assert(err, tc.ErrorIs, relationerrors.AmbiguousRelation)
}

func (s *modelRemoteApplicationSuite) TestAddConsumedRelationTwice(c *tc.C) {
	charmUUID := tc.Must(c, internaluuid.NewUUID).String()
	offerUUID := tc.Must(c, internaluuid.NewUUID).String()
	relationUUID := tc.Must(c, internaluuid.NewUUID).String()
	consumerModelUUID := tc.Must(c, internaluuid.NewUUID).String()
	consumerApplicationUUID := tc.Must(c, internaluuid.NewUUID).String()
	synthApplicationUUID := tc.Must(c, internaluuid.NewUUID).String()

	// Offer resources needed:
	offerApplicationUUID := tc.Must(c, coreapplication.NewUUID)
	offerCharmUUID := tc.Must(c, internaluuid.NewUUID).String()
	// Create an offer in the database.
	s.createOffer(c, offerUUID)
	// Create a charm in the database.
	s.createCharm(c, offerCharmUUID)
	// Create an application in the database.
	s.createApplication(c, offerApplicationUUID, offerCharmUUID, offerUUID)

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
			Requires: map[string]charm.Relation{},
			Peers:    map[string]charm.Relation{},
		},
	}
	err := s.state.AddConsumedRelation(c.Context(), "foo", crossmodelrelation.AddRemoteApplicationConsumerArgs{
		OfferUUID:                   offerUUID,
		RelationUUID:                relationUUID,
		ConsumerModelUUID:           consumerModelUUID,
		ConsumerApplicationUUID:     consumerApplicationUUID,
		ConsumerApplicationEndpoint: "db",
		SynthApplicationUUID:        synthApplicationUUID,
		CharmUUID:                   charmUUID,
		Charm:                       charm,
		Username:                    "consumer-user",
	})
	c.Assert(err, tc.ErrorIsNil)

	s.assertApplicationRemoteConsumer(c, consumerApplicationUUID)
	s.assertApplication(c, synthApplicationUUID)
	s.assertCharmMetadata(c, synthApplicationUUID, charmUUID, charm)

	err = s.state.AddConsumedRelation(c.Context(), "foo", crossmodelrelation.AddRemoteApplicationConsumerArgs{
		OfferUUID:                   offerUUID,
		RelationUUID:                relationUUID,
		ConsumerModelUUID:           consumerModelUUID,
		ConsumerApplicationUUID:     consumerApplicationUUID,
		ConsumerApplicationEndpoint: "db",
		SynthApplicationUUID:        synthApplicationUUID,
		CharmUUID:                   charmUUID,
		Charm:                       charm,
		Username:                    "consumer-user",
	})
	c.Assert(err, tc.ErrorIs, crossmodelrelationerrors.RemoteRelationAlreadyRegistered)
}

func (s *modelRemoteApplicationSuite) TestAddConsumedRelationMultiple(c *tc.C) {
	offerUUID := tc.Must(c, internaluuid.NewUUID).String()

	consumerModelUUID := tc.Must(c, internaluuid.NewUUID).String()
	consumerApplicationUUID := tc.Must(c, internaluuid.NewUUID).String()

	charmUUID0 := tc.Must(c, internaluuid.NewUUID).String()
	charmUUID1 := tc.Must(c, internaluuid.NewUUID).String()

	relationUUID0 := tc.Must(c, internaluuid.NewUUID).String()
	relationUUID1 := tc.Must(c, internaluuid.NewUUID).String()

	synthApplicationUUID0 := tc.Must(c, internaluuid.NewUUID).String()
	synthApplicationUUID1 := tc.Must(c, internaluuid.NewUUID).String()

	// Offer resources needed:
	offerApplicationUUID := tc.Must(c, coreapplication.NewUUID)
	offerCharmUUID := tc.Must(c, internaluuid.NewUUID).String()
	// Create an offer in the database.
	s.createOffer(c, offerUUID)
	// Create a charm in the database.
	s.createCharm(c, offerCharmUUID)
	// Create an application in the database.
	s.createApplication(c, offerApplicationUUID, offerCharmUUID, offerUUID)

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
			Requires: map[string]charm.Relation{},
			Peers:    map[string]charm.Relation{},
		},
	}

	charm2 := charm.Charm{
		ReferenceName: "baz",
		Source:        charm.CMRSource,
		Metadata: charm.Metadata{
			Name:        "bar",
			Description: "remote consumer application 2",
			Provides:    map[string]charm.Relation{},
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

	err := s.state.AddConsumedRelation(c.Context(), "foo", crossmodelrelation.AddRemoteApplicationConsumerArgs{
		OfferUUID:                   offerUUID,
		RelationUUID:                relationUUID0,
		ConsumerModelUUID:           consumerModelUUID,
		ConsumerApplicationUUID:     consumerApplicationUUID,
		ConsumerApplicationEndpoint: "db",
		SynthApplicationUUID:        synthApplicationUUID0,
		CharmUUID:                   charmUUID0,
		Charm:                       charm1,
		Username:                    "consumer-user",
	})
	c.Assert(err, tc.ErrorIsNil)

	// Should succeed since different applications can consume same offer
	err = s.state.AddConsumedRelation(c.Context(), "bar", crossmodelrelation.AddRemoteApplicationConsumerArgs{
		OfferUUID:                   offerUUID,
		RelationUUID:                relationUUID1,
		ConsumerModelUUID:           consumerModelUUID,
		ConsumerApplicationUUID:     consumerApplicationUUID,
		ConsumerApplicationEndpoint: "cache",
		SynthApplicationUUID:        synthApplicationUUID1,
		CharmUUID:                   charmUUID1,
		Charm:                       charm2,
		Username:                    "consumer-user",
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelRemoteApplicationSuite) TestAddConsumedRelationDyingApplication(c *tc.C) {
	charmUUID := tc.Must(c, internaluuid.NewUUID).String()
	offerUUID := tc.Must(c, internaluuid.NewUUID).String()
	relationUUID := tc.Must(c, internaluuid.NewUUID).String()
	consumerModelUUID := tc.Must(c, internaluuid.NewUUID).String()
	consumerApplicationUUID := tc.Must(c, internaluuid.NewUUID).String()
	synthApplicationUUID := tc.Must(c, internaluuid.NewUUID).String()

	// Offer resources needed:
	offerApplicationUUID := tc.Must(c, coreapplication.NewUUID)
	offerCharmUUID := tc.Must(c, internaluuid.NewUUID).String()
	// Create an offer in the database.
	s.createOffer(c, offerUUID)
	// Create a charm in the database.
	s.createCharm(c, offerCharmUUID)
	charmRelationUUID := s.createCharmRelation(c, offerCharmUUID, "offer-endpoint")
	// Create an application in the database.
	s.createDyingApplication(c, offerApplicationUUID, offerCharmUUID, offerUUID)
	s.addApplicationEndpoint(c, offerApplicationUUID, charmRelationUUID)

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
			Peers: map[string]charm.Relation{},
		},
	}
	err := s.state.AddConsumedRelation(c.Context(), "foo", crossmodelrelation.AddRemoteApplicationConsumerArgs{
		OfferUUID:                   offerUUID,
		RelationUUID:                relationUUID,
		ConsumerModelUUID:           consumerModelUUID,
		ConsumerApplicationUUID:     consumerApplicationUUID,
		ConsumerApplicationEndpoint: "db",
		SynthApplicationUUID:        synthApplicationUUID,
		CharmUUID:                   charmUUID,
		Charm:                       charm,
		Username:                    "consumer-user",
	})
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotAlive)
}

func (s *modelRemoteApplicationSuite) TestGetApplicationUUIDByOfferUUID(c *tc.C) {
	// Create application, charm and offer first
	applicationUUID := tc.Must(c, coreapplication.NewUUID)
	charmUUID := tc.Must(c, internaluuid.NewUUID).String()
	offerUUID := tc.Must(c, internaluuid.NewUUID).String()
	s.createOffer(c, offerUUID)
	s.createCharm(c, charmUUID)
	s.createApplication(c, applicationUUID, charmUUID, offerUUID)

	// Retrieve application UUID by offer UUID - should return the correct UUID
	gotName, gotUUID, err := s.state.GetApplicationNameAndUUIDByOfferUUID(c.Context(), offerUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotName, tc.Equals, applicationUUID.String())
	c.Check(gotUUID, tc.Equals, applicationUUID.String())
}

func (s *modelRemoteApplicationSuite) TestGetApplicationUUIDByOfferUUIDNotExists(c *tc.C) {
	// Test with non-existing offer UUID - should return application not found (no linked application)
	nonExistentUUID := tc.Must(c, internaluuid.NewUUID).String()
	_, _, err := s.state.GetApplicationNameAndUUIDByOfferUUID(c.Context(), nonExistentUUID)
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *modelRemoteApplicationSuite) TestGetSyntheticApplicationUUIDByOfferUUIDNoOfferConnection(c *tc.C) {
	// Create application, charm and offer first
	applicationUUID := tc.Must(c, coreapplication.NewUUID)
	charmUUID := tc.Must(c, internaluuid.NewUUID).String()
	offerUUID := tc.Must(c, internaluuid.NewUUID).String()
	s.createOffer(c, offerUUID)
	s.createCharm(c, charmUUID)
	s.createApplication(c, applicationUUID, charmUUID, offerUUID)

	// Retrieve application UUID by offer UUID - should return the correct UUID
	_, err := s.state.GetSyntheticApplicationUUIDByOfferUUID(c.Context(), offerUUID)
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *modelRemoteApplicationSuite) TestGetSyntheticApplicationUUIDByOfferUUIDWithOfferConnection(c *tc.C) {
	// Create application, charm and offer first
	synthApplicationUUID := tc.Must(c, coreapplication.NewUUID)
	realApplicationUUID := tc.Must(c, coreapplication.NewUUID)
	synthCharmUUID := tc.Must(c, internaluuid.NewUUID).String()
	realCharmUUID := tc.Must(c, internaluuid.NewUUID).String()
	offerUUID := tc.Must(c, internaluuid.NewUUID).String()
	s.createOffer(c, offerUUID)
	s.createCharm(c, synthCharmUUID)
	s.createCharm(c, realCharmUUID)
	s.createApplication(c, synthApplicationUUID, synthCharmUUID, offerUUID)
	s.createApplication(c, realApplicationUUID, realCharmUUID, "")
	relUUID := s.addRelation(c).String()

	s.query(c, `
INSERT INTO offer_connection (uuid, offer_uuid, remote_relation_uuid, username)
VALUES (?, ?, ?, 'bob')`, synthApplicationUUID, offerUUID, relUUID)
	s.query(c, `
INSERT INTO application_remote_consumer (offer_connection_uuid, offerer_application_uuid, consumer_application_uuid, consumer_model_uuid, life_id)
VALUES (?, ?, ?, ?, 0)`, synthApplicationUUID, realApplicationUUID, tc.Must(c, coreapplication.NewUUID), tc.Must(c, coreapplication.NewUUID))

	// Retrieve application UUID by offer UUID - should return the correct UUID
	uuid, err := s.state.GetSyntheticApplicationUUIDByOfferUUID(c.Context(), offerUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(uuid, tc.Equals, synthApplicationUUID.String())
}

func (s *modelRemoteApplicationSuite) TestGetSyntheticApplicationUUIDByOfferUUIDNotFound(c *tc.C) {
	// Test with non-existing offer UUID - should return application not found
	// (no linked application)
	nonExistentUUID := tc.Must(c, internaluuid.NewUUID).String()
	_, err := s.state.GetSyntheticApplicationUUIDByOfferUUID(c.Context(), nonExistentUUID)
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *modelRemoteApplicationSuite) TestGetRemoteApplicationConsumersSingle(c *tc.C) {
	charmUUID := tc.Must(c, internaluuid.NewUUID).String()
	offerUUID := tc.Must(c, internaluuid.NewUUID).String()
	relationUUID := tc.Must(c, internaluuid.NewUUID).String()
	consumerModelUUID := tc.Must(c, internaluuid.NewUUID).String()
	consumerApplicationUUID := tc.Must(c, internaluuid.NewUUID).String()
	synthApplicationUUID := tc.Must(c, internaluuid.NewUUID).String()

	// Offer resources needed:
	offerApplicationUUID := tc.Must(c, coreapplication.NewUUID)
	offerCharmUUID := tc.Must(c, internaluuid.NewUUID).String()
	// Create an offer in the database.
	s.createOffer(c, offerUUID)
	// Create a charm in the database.
	s.createCharm(c, offerCharmUUID)
	// Create an application in the database.
	s.createApplication(c, offerApplicationUUID, offerCharmUUID, offerUUID)

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
			Requires: map[string]charm.Relation{},
			Peers:    map[string]charm.Relation{},
		},
	}
	err := s.state.AddConsumedRelation(c.Context(), "foo", crossmodelrelation.AddRemoteApplicationConsumerArgs{
		OfferUUID:                   offerUUID,
		RelationUUID:                relationUUID,
		ConsumerModelUUID:           consumerModelUUID,
		ConsumerApplicationUUID:     consumerApplicationUUID,
		ConsumerApplicationEndpoint: "db",
		SynthApplicationUUID:        synthApplicationUUID,
		CharmUUID:                   charmUUID,
		Charm:                       charm,
		Username:                    "consumer-user",
	})
	c.Assert(err, tc.ErrorIsNil)

	results, err := s.state.GetRemoteApplicationConsumers(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 1)
	c.Check(results[0].ApplicationName, tc.Equals, "foo")
	c.Check(results[0].OfferUUID, tc.Equals, offerUUID)
	c.Check(results[0].Life, tc.Equals, life.Alive)
}

func (s *modelRemoteApplicationSuite) TestGetRemoteApplicationConsumersMultiple(c *tc.C) {
	offerUUID := tc.Must(c, internaluuid.NewUUID).String()

	consumerModelUUID := tc.Must(c, internaluuid.NewUUID).String()
	consumerApplicationUUID := tc.Must(c, internaluuid.NewUUID).String()

	charmUUID0 := tc.Must(c, internaluuid.NewUUID).String()
	charmUUID1 := tc.Must(c, internaluuid.NewUUID).String()

	relationUUID0 := tc.Must(c, internaluuid.NewUUID).String()
	relationUUID1 := tc.Must(c, internaluuid.NewUUID).String()

	synthApplicationUUID0 := tc.Must(c, internaluuid.NewUUID).String()
	synthApplicationUUID1 := tc.Must(c, internaluuid.NewUUID).String()

	// Offer resources needed:
	offerApplicationUUID := tc.Must(c, coreapplication.NewUUID)
	offerCharmUUID := tc.Must(c, internaluuid.NewUUID).String()
	// Create an offer in the database.
	s.createOffer(c, offerUUID)
	// Create a charm in the database.
	s.createCharm(c, offerCharmUUID)
	// Create an application in the database.
	s.createApplication(c, offerApplicationUUID, offerCharmUUID, offerUUID)

	charm1 := charm.Charm{
		ReferenceName: "bar",
		Source:        charm.CMRSource,
		Metadata: charm.Metadata{
			Name:        "foo",
			Description: "remote consumer application",
			Provides:    map[string]charm.Relation{},
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
			Requires: map[string]charm.Relation{},
			Peers:    map[string]charm.Relation{},
		},
	}

	err := s.state.AddConsumedRelation(c.Context(), "foo", crossmodelrelation.AddRemoteApplicationConsumerArgs{
		OfferUUID:                   offerUUID,
		RelationUUID:                relationUUID0,
		ConsumerModelUUID:           consumerModelUUID,
		ConsumerApplicationUUID:     consumerApplicationUUID,
		ConsumerApplicationEndpoint: "cache",
		SynthApplicationUUID:        synthApplicationUUID0,
		CharmUUID:                   charmUUID0,
		Charm:                       charm1,
		Username:                    "consumer-user",
	})
	c.Assert(err, tc.ErrorIsNil)

	// Should succeed since different applications can consume same offer
	err = s.state.AddConsumedRelation(c.Context(), "bar", crossmodelrelation.AddRemoteApplicationConsumerArgs{
		OfferUUID:                   offerUUID,
		RelationUUID:                relationUUID1,
		ConsumerModelUUID:           consumerModelUUID,
		ConsumerApplicationUUID:     consumerApplicationUUID,
		ConsumerApplicationEndpoint: "db",
		SynthApplicationUUID:        synthApplicationUUID1,
		CharmUUID:                   charmUUID1,
		Charm:                       charm2,
		Username:                    "consumer-user",
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

	c.Check(foo.OfferUUID, tc.Equals, offerUUID)
	c.Check(bar.OfferUUID, tc.Equals, offerUUID)
}

func (s *modelRemoteApplicationSuite) TestGetRemoteApplicationConsumersEmpty(c *tc.C) {
	results, err := s.state.GetRemoteApplicationConsumers(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(results, tc.HasLen, 0)
}

func (s *modelRemoteApplicationSuite) TestGetRemoteApplicationConsumersFiltersDead(c *tc.C) {
	charmUUID := tc.Must(c, internaluuid.NewUUID).String()
	offerUUID := tc.Must(c, internaluuid.NewUUID).String()
	relationUUID := tc.Must(c, internaluuid.NewUUID).String()
	consumerModelUUID := tc.Must(c, internaluuid.NewUUID).String()
	consumerApplicationUUID := tc.Must(c, internaluuid.NewUUID).String()
	synthApplicationUUID := tc.Must(c, internaluuid.NewUUID).String()

	// Offer resources needed:
	offerApplicationUUID := tc.Must(c, coreapplication.NewUUID)
	offerCharmUUID := tc.Must(c, internaluuid.NewUUID).String()
	// Create an offer in the database.
	s.createOffer(c, offerUUID)
	// Create a charm in the database.
	s.createCharm(c, offerCharmUUID)
	// Create an application in the database.
	s.createApplication(c, offerApplicationUUID, offerCharmUUID, offerUUID)

	charm := charm.Charm{
		ReferenceName: "bar",
		Source:        charm.CMRSource,
		Metadata: charm.Metadata{
			Name:        "foo",
			Description: "remote consumer application",
			Provides:    map[string]charm.Relation{},
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
	err := s.state.AddConsumedRelation(c.Context(), "foo", crossmodelrelation.AddRemoteApplicationConsumerArgs{
		OfferUUID:                   offerUUID,
		RelationUUID:                relationUUID,
		ConsumerModelUUID:           consumerModelUUID,
		ConsumerApplicationUUID:     consumerApplicationUUID,
		ConsumerApplicationEndpoint: "cache",
		SynthApplicationUUID:        synthApplicationUUID,
		CharmUUID:                   charmUUID,
		Charm:                       charm,
		Username:                    "consumer-user",
	})
	c.Assert(err, tc.ErrorIsNil)

	// Mark consumer record as Dead (life_id = 2) so it should be filtered out.
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, e := tx.ExecContext(ctx, `
UPDATE application_remote_consumer
SET life_id = 2
WHERE consumer_application_uuid = ?`, consumerApplicationUUID)
		return e
	})
	c.Assert(err, tc.ErrorIsNil)

	results, err := s.state.GetRemoteApplicationConsumers(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(results, tc.HasLen, 0)
}

func (s *modelRemoteApplicationSuite) TestGetOffererRelationUUIDsForConsumersSingleConsumer(c *tc.C) {
	charmUUID := tc.Must(c, internaluuid.NewUUID).String()
	offerUUID := tc.Must(c, internaluuid.NewUUID).String()
	relationUUID := tc.Must(c, internaluuid.NewUUID).String()
	consumerModelUUID := tc.Must(c, internaluuid.NewUUID).String()
	consumerApplicationUUID := tc.Must(c, internaluuid.NewUUID).String()
	synthApplicationUUID := tc.Must(c, internaluuid.NewUUID).String()

	// Offer resources needed:
	offerApplicationUUID := tc.Must(c, coreapplication.NewUUID)
	offerCharmUUID := tc.Must(c, internaluuid.NewUUID).String()
	// Create an offer in the database.
	s.createOffer(c, offerUUID)
	// Create a charm in the database.
	s.createCharm(c, offerCharmUUID)
	// Create an application in the database.
	s.createApplication(c, offerApplicationUUID, offerCharmUUID, offerUUID)

	charm := charm.Charm{
		ReferenceName: "bar",
		Source:        charm.CMRSource,
		Metadata: charm.Metadata{
			Name:        "foo",
			Description: "remote consumer application",
			Provides:    map[string]charm.Relation{},
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
	err := s.state.AddConsumedRelation(c.Context(), "foo", crossmodelrelation.AddRemoteApplicationConsumerArgs{
		OfferUUID:                   offerUUID,
		RelationUUID:                relationUUID,
		ConsumerModelUUID:           consumerModelUUID,
		ConsumerApplicationUUID:     consumerApplicationUUID,
		ConsumerApplicationEndpoint: "cache",
		SynthApplicationUUID:        synthApplicationUUID,
		CharmUUID:                   charmUUID,
		Charm:                       charm,
		Username:                    "consumer-user",
	})
	c.Assert(err, tc.ErrorIsNil)

	// Get the application_remote_consumer UUID
	var consumerUUID string
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `
SELECT offer_connection_uuid FROM application_remote_consumer WHERE consumer_application_uuid = ?`, consumerApplicationUUID).Scan(&consumerUUID)
	})
	c.Assert(err, tc.ErrorIsNil)

	// Get the offerer relation UUID (synthetic relation created for the consumer)
	var offererRelationUUID string
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `
SELECT r.uuid
FROM   application_remote_consumer AS arc
JOIN   offer_connection AS oc ON oc.uuid = arc.offer_connection_uuid
JOIN   relation AS r ON r.uuid = oc.remote_relation_uuid
WHERE  arc.offer_connection_uuid = ?`, consumerUUID).Scan(&offererRelationUUID)
	})
	c.Assert(err, tc.ErrorIsNil)

	// Test the method
	results, err := s.state.GetOffererRelationUUIDsForConsumers(c.Context(), consumerUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 1)
	c.Check(results[0], tc.Equals, offererRelationUUID)
}

func (s *modelRemoteApplicationSuite) TestGetOffererRelationUUIDsForConsumersMultipleConsumers(c *tc.C) {
	offerUUID0 := tc.Must(c, internaluuid.NewUUID).String()
	offerUUID1 := tc.Must(c, internaluuid.NewUUID).String()

	consumerModelUUID := tc.Must(c, internaluuid.NewUUID).String()

	consumerApplicationUUID0 := tc.Must(c, internaluuid.NewUUID).String()
	consumerApplicationUUID1 := tc.Must(c, internaluuid.NewUUID).String()

	charmUUID0 := tc.Must(c, internaluuid.NewUUID).String()
	charmUUID1 := tc.Must(c, internaluuid.NewUUID).String()

	relationUUID0 := tc.Must(c, internaluuid.NewUUID).String()
	relationUUID1 := tc.Must(c, internaluuid.NewUUID).String()

	synthApplicationUUID0 := tc.Must(c, internaluuid.NewUUID).String()
	synthApplicationUUID1 := tc.Must(c, internaluuid.NewUUID).String()

	// Offer resources needed:
	offerApplicationUUID0 := tc.Must(c, coreapplication.NewUUID)
	offerApplicationUUID1 := tc.Must(c, coreapplication.NewUUID)

	offerCharmUUID0 := tc.Must(c, internaluuid.NewUUID).String()
	offerCharmUUID1 := tc.Must(c, internaluuid.NewUUID).String()

	// Create an offer in the database.
	s.createOffer(c, offerUUID0)
	// Create a charm in the database.
	s.createCharm(c, offerCharmUUID0)
	// Create an application in the database.
	s.createApplication(c, offerApplicationUUID0, offerCharmUUID0, offerUUID0)

	s.createOffer(c, offerUUID1)
	// Create a charm in the database.
	s.createCharm(c, offerCharmUUID1)
	// Create an application in the database.
	s.createApplication(c, offerApplicationUUID1, offerCharmUUID1, offerUUID1)

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
			Peers: map[string]charm.Relation{},
		},
	}

	charm2 := charm.Charm{
		ReferenceName: "baz",
		Source:        charm.CMRSource,
		Metadata: charm.Metadata{
			Name:        "bar",
			Description: "remote consumer application 2",
			Provides:    map[string]charm.Relation{},
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

	err := s.state.AddConsumedRelation(c.Context(), "foo", crossmodelrelation.AddRemoteApplicationConsumerArgs{
		OfferUUID:                   offerUUID0,
		RelationUUID:                relationUUID0,
		ConsumerModelUUID:           consumerModelUUID,
		ConsumerApplicationUUID:     consumerApplicationUUID0,
		ConsumerApplicationEndpoint: "db",
		SynthApplicationUUID:        synthApplicationUUID0,
		CharmUUID:                   charmUUID0,
		Charm:                       charm1,
		Username:                    "consumer-user",
	})
	c.Assert(err, tc.ErrorIsNil)

	// Should succeed since different applications can consume same offer
	err = s.state.AddConsumedRelation(c.Context(), "bar", crossmodelrelation.AddRemoteApplicationConsumerArgs{
		OfferUUID:                   offerUUID1,
		RelationUUID:                relationUUID1,
		ConsumerModelUUID:           consumerModelUUID,
		ConsumerApplicationUUID:     consumerApplicationUUID1,
		ConsumerApplicationEndpoint: "cache",
		SynthApplicationUUID:        synthApplicationUUID1,
		CharmUUID:                   charmUUID1,
		Charm:                       charm2,
		Username:                    "consumer-user",
	})
	c.Assert(err, tc.ErrorIsNil)

	// Get consumer UUIDs and expected relation UUIDs
	var consumerUUID1, consumerUUID2, offererRelationUUID1, offererRelationUUID2 string
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, `
SELECT offer_connection_uuid FROM application_remote_consumer WHERE consumer_application_uuid = ?`, consumerApplicationUUID0).Scan(&consumerUUID1); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, `
SELECT offer_connection_uuid FROM application_remote_consumer WHERE consumer_application_uuid = ?`, consumerApplicationUUID1).Scan(&consumerUUID2); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, `
SELECT r.uuid
FROM   application_remote_consumer AS arc
JOIN   offer_connection AS oc ON oc.uuid = arc.offer_connection_uuid
JOIN   relation AS r ON r.uuid = oc.remote_relation_uuid
WHERE  arc.offer_connection_uuid = ?`, consumerUUID1).Scan(&offererRelationUUID1); err != nil {
			return err
		}
		return tx.QueryRowContext(ctx, `
SELECT r.uuid
FROM   application_remote_consumer AS arc
JOIN   offer_connection AS oc ON oc.uuid = arc.offer_connection_uuid
JOIN   relation AS r ON r.uuid = oc.remote_relation_uuid
WHERE  arc.offer_connection_uuid = ?`, consumerUUID2).Scan(&offererRelationUUID2)
	})
	c.Assert(err, tc.ErrorIsNil)

	// Test with multiple consumer UUIDs
	results, err := s.state.GetOffererRelationUUIDsForConsumers(c.Context(), consumerUUID1, consumerUUID2)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 2)

	res := set.NewStrings(results...).Intersection(set.NewStrings(offererRelationUUID1, offererRelationUUID2))
	c.Check(res.Size(), tc.Equals, 2)
}

func (s *modelRemoteApplicationSuite) TestGetOffererRelationUUIDsForConsumersEmpty(c *tc.C) {
	// Test with no consumer UUIDs provided
	results, err := s.state.GetOffererRelationUUIDsForConsumers(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(results, tc.HasLen, 0)
}

func (s *modelRemoteApplicationSuite) TestGetOffererRelationUUIDsForConsumersNonExistentUUID(c *tc.C) {
	// Test with a UUID that doesn't exist in the database
	nonExistentUUID := tc.Must(c, internaluuid.NewUUID).String()
	results, err := s.state.GetOffererRelationUUIDsForConsumers(c.Context(), nonExistentUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(results, tc.HasLen, 0)
}

func (s *modelRemoteApplicationSuite) TestGetOffererRelationUUIDsForConsumersMixedValidAndInvalid(c *tc.C) {
	charmUUID := tc.Must(c, internaluuid.NewUUID).String()
	offerUUID := tc.Must(c, internaluuid.NewUUID).String()
	relationUUID := tc.Must(c, internaluuid.NewUUID).String()
	consumerModelUUID := tc.Must(c, internaluuid.NewUUID).String()
	consumerApplicationUUID := tc.Must(c, internaluuid.NewUUID).String()
	synthApplicationUUID := tc.Must(c, internaluuid.NewUUID).String()

	// Offer resources needed:
	offerApplicationUUID := tc.Must(c, coreapplication.NewUUID)
	offerCharmUUID := tc.Must(c, internaluuid.NewUUID).String()
	// Create an offer in the database.
	s.createOffer(c, offerUUID)
	// Create a charm in the database.
	s.createCharm(c, offerCharmUUID)
	// Create an application in the database.
	s.createApplication(c, offerApplicationUUID, offerCharmUUID, offerUUID)

	charm := charm.Charm{
		ReferenceName: "bar",
		Source:        charm.CMRSource,
		Metadata: charm.Metadata{
			Name:        "foo",
			Description: "remote consumer application",
			Provides:    map[string]charm.Relation{},
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
	err := s.state.AddConsumedRelation(c.Context(), "foo", crossmodelrelation.AddRemoteApplicationConsumerArgs{
		OfferUUID:                   offerUUID,
		RelationUUID:                relationUUID,
		ConsumerModelUUID:           consumerModelUUID,
		ConsumerApplicationUUID:     consumerApplicationUUID,
		ConsumerApplicationEndpoint: "cache",
		SynthApplicationUUID:        synthApplicationUUID,
		CharmUUID:                   charmUUID,
		Charm:                       charm,
		Username:                    "consumer-user",
	})
	c.Assert(err, tc.ErrorIsNil)

	var consumerUUID, offererRelationUUID string
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, `
SELECT offer_connection_uuid FROM application_remote_consumer WHERE consumer_application_uuid = ?
`, consumerApplicationUUID).Scan(&consumerUUID); err != nil {
			return err
		}
		return tx.QueryRowContext(ctx, `

SELECT r.uuid
FROM   application_remote_consumer AS arc
JOIN   offer_connection AS oc ON oc.uuid = arc.offer_connection_uuid
JOIN   relation AS r ON r.uuid = oc.remote_relation_uuid
WHERE  arc.offer_connection_uuid = ?`, consumerUUID).Scan(&offererRelationUUID)

	})
	c.Assert(err, tc.ErrorIsNil)

	// Test with mix of valid and invalid UUIDs
	invalidUUID := tc.Must(c, internaluuid.NewUUID).String()
	results, err := s.state.GetOffererRelationUUIDsForConsumers(c.Context(), consumerUUID, invalidUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 1)
	c.Check(results[0], tc.Equals, offererRelationUUID)
}

func (s *modelRemoteApplicationSuite) TestGetOffererRelationUUIDsForConsumersDuplicateInput(c *tc.C) {
	// Test that duplicate consumer UUIDs in input result in DISTINCT relation UUIDs (no duplicates in output)
	charmUUID := tc.Must(c, internaluuid.NewUUID).String()
	offerUUID := tc.Must(c, internaluuid.NewUUID).String()
	relationUUID := tc.Must(c, internaluuid.NewUUID).String()
	consumerModelUUID := tc.Must(c, internaluuid.NewUUID).String()
	consumerApplicationUUID := tc.Must(c, internaluuid.NewUUID).String()
	synthApplicationUUID := tc.Must(c, internaluuid.NewUUID).String()

	// Offer resources needed:
	offerApplicationUUID := tc.Must(c, coreapplication.NewUUID)
	offerCharmUUID := tc.Must(c, internaluuid.NewUUID).String()
	// Create an offer in the database.
	s.createOffer(c, offerUUID)
	// Create a charm in the database.
	s.createCharm(c, offerCharmUUID)
	// Create an application in the database.
	s.createApplication(c, offerApplicationUUID, offerCharmUUID, offerUUID)

	charm := charm.Charm{
		ReferenceName: "bar",
		Source:        charm.CMRSource,
		Metadata: charm.Metadata{
			Name:        "foo",
			Description: "remote consumer application",
			Provides:    map[string]charm.Relation{},
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
	err := s.state.AddConsumedRelation(c.Context(), "foo", crossmodelrelation.AddRemoteApplicationConsumerArgs{
		OfferUUID:                   offerUUID,
		RelationUUID:                relationUUID,
		ConsumerModelUUID:           consumerModelUUID,
		ConsumerApplicationUUID:     consumerApplicationUUID,
		ConsumerApplicationEndpoint: "cache",
		SynthApplicationUUID:        synthApplicationUUID,
		CharmUUID:                   charmUUID,
		Charm:                       charm,
		Username:                    "consumer-user",
	})
	c.Assert(err, tc.ErrorIsNil)

	var consumerUUID, offererRelationUUID string
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, `
SELECT offer_connection_uuid FROM application_remote_consumer WHERE consumer_application_uuid = ?`, consumerApplicationUUID).Scan(&consumerUUID); err != nil {
			return err
		}
		return tx.QueryRowContext(ctx, `

SELECT r.uuid
FROM   application_remote_consumer AS arc
JOIN   offer_connection AS oc ON oc.uuid = arc.offer_connection_uuid
JOIN   relation AS r ON r.uuid = oc.remote_relation_uuid
WHERE  arc.offer_connection_uuid = ?`, consumerUUID).Scan(&offererRelationUUID)

	})
	c.Assert(err, tc.ErrorIsNil)

	// Test with duplicate consumer UUIDs - should still only return one relation UUID due to DISTINCT
	results, err := s.state.GetOffererRelationUUIDsForConsumers(c.Context(), consumerUUID, consumerUUID, consumerUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 1)
	c.Check(results[0], tc.Equals, offererRelationUUID)
}

func (s *modelRemoteApplicationSuite) TestGetRemoteConsumerApplicationName(c *tc.C) {
	charmUUID := tc.Must(c, internaluuid.NewUUID).String()
	offerUUID := tc.Must(c, internaluuid.NewUUID).String()
	relationUUID := tc.Must(c, internaluuid.NewUUID).String()
	consumerModelUUID := tc.Must(c, internaluuid.NewUUID).String()
	consumerApplicationUUID := tc.Must(c, internaluuid.NewUUID).String()
	synthApplicationUUID := tc.Must(c, internaluuid.NewUUID).String()

	// Offer resources needed:
	offerApplicationUUID := tc.Must(c, coreapplication.NewUUID)
	offerCharmUUID := tc.Must(c, internaluuid.NewUUID).String()
	// Create an offer in the database.
	s.createOffer(c, offerUUID)
	// Create a charm in the database.
	s.createCharm(c, offerCharmUUID)
	// Create an application in the database.
	s.createApplication(c, offerApplicationUUID, offerCharmUUID, offerUUID)

	charm := charm.Charm{
		ReferenceName: "bar",
		Source:        charm.CMRSource,
		Metadata: charm.Metadata{
			Name:        "foo",
			Description: "remote consumer application",
			Provides:    map[string]charm.Relation{},
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
	err := s.state.AddConsumedRelation(c.Context(), "remote-foo", crossmodelrelation.AddRemoteApplicationConsumerArgs{
		OfferUUID:                   offerUUID,
		RelationUUID:                relationUUID,
		ConsumerModelUUID:           consumerModelUUID,
		ConsumerApplicationUUID:     consumerApplicationUUID,
		ConsumerApplicationEndpoint: "cache",
		SynthApplicationUUID:        synthApplicationUUID,
		CharmUUID:                   charmUUID,
		Charm:                       charm,
		Username:                    "consumer-user",
	})
	c.Assert(err, tc.ErrorIsNil)

	name, err := s.state.GetRemoteConsumerApplicationName(c.Context(), consumerApplicationUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(name, tc.Equals, "remote-foo")
}

func (s *modelRemoteApplicationSuite) TestGetRemoteConsumerApplicationNameNotFound(c *tc.C) {
	consumerApplicationUUID := tc.Must(c, internaluuid.NewUUID).String()
	_, err := s.state.GetRemoteConsumerApplicationName(c.Context(), consumerApplicationUUID)
	c.Assert(err, tc.ErrorIs, crossmodelrelationerrors.RemoteApplicationNotFound)
}

func (s *modelRemoteApplicationSuite) TestEnsureUnitsExist(c *tc.C) {
	applicationUUID := tc.Must(c, coreapplication.NewUUID)
	charmUUID := tc.Must(c, internaluuid.NewUUID).String()
	offerUUID := tc.Must(c, internaluuid.NewUUID).String()

	s.createOffer(c, offerUUID)
	s.createCharm(c, charmUUID)
	s.createApplication(c, applicationUUID, charmUUID, offerUUID)

	err := s.state.EnsureUnitsExist(c.Context(), applicationUUID.String(), []string{"app/0", "app/1", "app/2"})
	c.Assert(err, tc.ErrorIsNil)

	s.assertUnitNames(c, applicationUUID, []string{"app/0", "app/1", "app/2"})
}

func (s *modelRemoteApplicationSuite) TestEnsureUnitsExistIdempotent(c *tc.C) {
	applicationUUID := tc.Must(c, coreapplication.NewUUID)
	charmUUID := tc.Must(c, internaluuid.NewUUID).String()
	offerUUID := tc.Must(c, internaluuid.NewUUID).String()

	s.createOffer(c, offerUUID)
	s.createCharm(c, charmUUID)
	s.createApplication(c, applicationUUID, charmUUID, offerUUID)

	err := s.state.EnsureUnitsExist(c.Context(), applicationUUID.String(), []string{"app/0", "app/1", "app/2"})
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.EnsureUnitsExist(c.Context(), applicationUUID.String(), []string{"app/0", "app/1", "app/2"})
	c.Assert(err, tc.ErrorIsNil)

	s.assertUnitNames(c, applicationUUID, []string{"app/0", "app/1", "app/2"})
}

func (s *modelRemoteApplicationSuite) TestEnsureUnitsExistIdempotentPartial(c *tc.C) {
	applicationUUID := tc.Must(c, coreapplication.NewUUID)
	charmUUID := tc.Must(c, internaluuid.NewUUID).String()
	offerUUID := tc.Must(c, internaluuid.NewUUID).String()

	s.createOffer(c, offerUUID)
	s.createCharm(c, charmUUID)
	s.createApplication(c, applicationUUID, charmUUID, offerUUID)

	err := s.state.EnsureUnitsExist(c.Context(), applicationUUID.String(), []string{"app/0", "app/1", "app/2"})
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.EnsureUnitsExist(c.Context(), applicationUUID.String(), []string{"app/1", "app/2"})
	c.Assert(err, tc.ErrorIsNil)

	s.assertUnitNames(c, applicationUUID, []string{"app/0", "app/1", "app/2"})
}

func (s *modelRemoteApplicationSuite) TestEnsureUnitsExistMissing(c *tc.C) {
	applicationUUID := tc.Must(c, coreapplication.NewUUID)
	charmUUID := tc.Must(c, internaluuid.NewUUID).String()
	offerUUID := tc.Must(c, internaluuid.NewUUID).String()

	s.createOffer(c, offerUUID)
	s.createCharm(c, charmUUID)
	s.createApplication(c, applicationUUID, charmUUID, offerUUID)

	err := s.state.EnsureUnitsExist(c.Context(), applicationUUID.String(), []string{"app/0", "app/1", "app/2"})
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.EnsureUnitsExist(c.Context(), applicationUUID.String(), []string{"app/1", "app/4"})
	c.Assert(err, tc.ErrorIsNil)

	s.assertUnitNames(c, applicationUUID, []string{"app/0", "app/1", "app/2", "app/4"})
}

func (s *modelRemoteApplicationSuite) TestEnsureUnitsExistDying(c *tc.C) {
	applicationUUID := tc.Must(c, coreapplication.NewUUID)
	charmUUID := tc.Must(c, internaluuid.NewUUID).String()
	offerUUID := tc.Must(c, internaluuid.NewUUID).String()

	s.createOffer(c, offerUUID)
	s.createCharm(c, charmUUID)
	s.createApplication(c, applicationUUID, charmUUID, offerUUID)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, e := tx.ExecContext(ctx, `
UPDATE application
SET life_id = 1
WHERE uuid = ?`, applicationUUID)
		return e
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.EnsureUnitsExist(c.Context(), applicationUUID.String(), []string{"app/0", "app/1", "app/2"})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelRemoteApplicationSuite) TestEnsureUnitsExistDead(c *tc.C) {
	applicationUUID := tc.Must(c, coreapplication.NewUUID)
	charmUUID := tc.Must(c, internaluuid.NewUUID).String()
	offerUUID := tc.Must(c, internaluuid.NewUUID).String()

	s.createOffer(c, offerUUID)
	s.createCharm(c, charmUUID)
	s.createApplication(c, applicationUUID, charmUUID, offerUUID)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, e := tx.ExecContext(ctx, `
UPDATE application
SET life_id = 2
WHERE uuid = ?`, applicationUUID)
		return e
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.EnsureUnitsExist(c.Context(), applicationUUID.String(), []string{"app/0", "app/1", "app/2"})
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *modelRemoteApplicationSuite) TestGetOfferingApplicationToken(c *tc.C) {
	// Arrange
	cmrCharmUUID := s.addCMRCharm(c)
	cmrRelation := internalcharm.Relation{
		Name:      "db",
		Role:      internalcharm.RoleRequirer,
		Interface: "db",
		Scope:     internalcharm.ScopeGlobal,
	}
	cmrRelationUUID := s.addCharmRelation(c, cmrCharmUUID, cmrRelation)
	cmrAppUUID := s.addApplication(c, cmrCharmUUID, "cmr-app")
	cmrEndpointUUID := s.addApplicationEndpoint(c, cmrAppUUID, cmrRelationUUID)

	charmUUID := s.addCharm(c)
	s.addCharmMetadata(c, charmUUID, false)
	relation := internalcharm.Relation{
		Name:      "db",
		Role:      internalcharm.RoleProvider,
		Interface: "db",
		Scope:     internalcharm.ScopeGlobal,
	}
	charmRelationUUID := s.addCharmRelation(c, charmUUID, relation)
	appUUID := s.addApplication(c, charmUUID, "local")
	endpointUUID := s.addApplicationEndpoint(c, appUUID, charmRelationUUID)

	relationUUID := s.addRelation(c)
	s.addRelationEndpoint(c, relationUUID.String(), endpointUUID)
	s.addRelationEndpoint(c, relationUUID.String(), cmrEndpointUUID)

	// Act
	appToken, err := s.state.GetOfferingApplicationToken(c.Context(), relationUUID.String())

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(appToken, tc.Equals, appUUID.String())
}

func (s *modelRemoteApplicationSuite) TestGetOfferingApplicationTokenNotRemote(c *tc.C) {
	// Arrange
	otherCharmUUID := s.addCharm(c)
	otherRelation := internalcharm.Relation{
		Name:      "db",
		Role:      internalcharm.RoleRequirer,
		Interface: "db",
		Scope:     internalcharm.ScopeGlobal,
	}
	otherRelationUUID := s.addCharmRelation(c, otherCharmUUID, otherRelation)
	otherAppUUID := s.addApplication(c, otherCharmUUID, "app")
	otherEndpointUUID := s.addApplicationEndpoint(c, otherAppUUID, otherRelationUUID)

	charmUUID := s.addCharm(c)
	s.addCharmMetadata(c, charmUUID, false)
	relation := internalcharm.Relation{
		Name:      "db",
		Role:      internalcharm.RoleProvider,
		Interface: "db",
		Scope:     internalcharm.ScopeGlobal,
	}
	charmRelationUUID := s.addCharmRelation(c, charmUUID, relation)
	appUUID := s.addApplication(c, charmUUID, "local")
	endpointUUID := s.addApplicationEndpoint(c, appUUID, charmRelationUUID)

	relationUUID := s.addRelation(c)
	s.addRelationEndpoint(c, relationUUID.String(), endpointUUID)
	s.addRelationEndpoint(c, relationUUID.String(), otherEndpointUUID)

	// Act
	_, err := s.state.GetOfferingApplicationToken(c.Context(), relationUUID.String())

	// Assert
	c.Assert(err, tc.ErrorIs, crossmodelrelationerrors.RelationNotRemote)
}

func (s *modelRemoteApplicationSuite) TestGetOfferingApplicationTokenNotFound(c *tc.C) {
	// Act
	_, err := s.state.GetOfferingApplicationToken(c.Context(), "bad-uuid")

	// Assert
	c.Assert(err, tc.ErrorIs, relationerrors.RelationNotFound)
}

func (s *modelRemoteApplicationSuite) assertUnitNames(c *tc.C, applicationUUID coreapplication.UUID, expectedNames []string) {
	var names []string
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `
SELECT name
FROM unit
WHERE application_uuid = ?`, applicationUUID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				return err
			}
			names = append(names, name)
		}
		return rows.Err()
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(names, tc.SameContents, expectedNames)
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

func (s *modelRemoteApplicationSuite) createApplication(c *tc.C, applicationUUID coreapplication.UUID, charmUUID string, offerUUID string) {
	s.createApplicationWithLife(c, applicationUUID, charmUUID, offerUUID, life.Alive)
}

func (s *modelRemoteApplicationSuite) createDyingApplication(c *tc.C, applicationUUID coreapplication.UUID, charmUUID string, offerUUID string) {
	s.createApplicationWithLife(c, applicationUUID, charmUUID, offerUUID, life.Dying)
}

func (s *modelRemoteApplicationSuite) createApplicationWithLife(c *tc.C, applicationUUID coreapplication.UUID, charmUUID string, offerUUID string, l life.Life) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		// Create an application record
		_, err := tx.Exec(`
INSERT INTO application (uuid, name, charm_uuid, life_id, space_uuid)
VALUES (?, ?, ?, ?, ?)
`, applicationUUID, applicationUUID, charmUUID, l, network.AlphaSpaceId)
		if err != nil {
			return err
		}

		charmRelationUUID := tc.Must(c, internaluuid.NewUUID).String()
		charmEndpointUUID := tc.Must(c, internaluuid.NewUUID).String()

		// Insert charm_relation and application_endpoint records
		insertCharmRelation := `INSERT INTO charm_relation (uuid, charm_uuid, scope_id, role_id, name) VALUES (?, ?, ?, ?, ?)`
		_, err = tx.ExecContext(ctx, insertCharmRelation, charmRelationUUID, charmUUID, "0", "0", "endpoint0")
		if err != nil {
			return err
		}
		insertEndpoint := `INSERT INTO application_endpoint (uuid, application_uuid, space_uuid, charm_relation_uuid) VALUES (?, ?, ?, ?)`
		_, err = tx.ExecContext(ctx, insertEndpoint, charmEndpointUUID, applicationUUID, network.AlphaSpaceId, charmRelationUUID)
		if err != nil {
			return err
		}
		// Insert an offer endpoint record if it's not empty.
		if offerUUID == "" {
			return nil
		}
		insertOfferEndpoint := `INSERT INTO offer_endpoint (offer_uuid, endpoint_uuid) VALUES (?, ?)`
		_, err = tx.ExecContext(ctx, insertOfferEndpoint, offerUUID, charmEndpointUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelRemoteApplicationSuite) createCharm(c *tc.C, charmUUID string) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		// Create a charm record
		_, err := tx.Exec(`
INSERT INTO charm (uuid, reference_name, source_id)
VALUES (?, ?, 1)
`, charmUUID, charmUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelRemoteApplicationSuite) createCharmRelation(c *tc.C, charmUUID, endpointName string) string {
	charmRelUUID1 := tc.Must(c, internaluuid.NewUUID).String()
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO charm_relation (uuid, charm_uuid, scope_id, role_id, name) VALUES (?, ?, 0, 0, ?)`,
			charmRelUUID1, charmUUID, endpointName)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	return charmRelUUID1
}

func (s *modelRemoteApplicationSuite) assertApplicationRemoteOfferer(c *tc.C, uuid string) {
	var (
		gotLifeID   int
		gotMacaroon string
	)
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `
SELECT aro.life_id, aro.macaroon 
FROM application_remote_offerer AS aro
JOIN application AS a ON aro.application_uuid = a.uuid
WHERE a.uuid=?`, uuid).
			Scan(&gotLifeID, &gotMacaroon)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotLifeID, tc.Equals, 0)
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
		gotUUID            string
		gotID              int
		gotLifID           int
		gotScopeID         int
		gotSuspended       bool
		gotSuspendedReason sql.Null[string]
	)
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `
SELECT uuid, relation_id, life_id, scope_id, suspended, suspended_reason
FROM relation WHERE relation_id=?
`, relationID).
			Scan(&gotUUID, &gotID, &gotLifID, &gotScopeID, &gotSuspended, &gotSuspendedReason)
		if err != nil {
			return err
		}
		return nil
	})
	if c.Check(err, tc.ErrorIsNil) {
		c.Check(gotUUID, tc.Equals, relationUUID)
		c.Check(gotID, tc.Equals, relationID)
		c.Check(gotLifID, tc.Equals, 0)   // life.Alive
		c.Check(gotScopeID, tc.Equals, 0) // scope.Global
		c.Check(gotSuspended, tc.Equals, false)
		c.Check(gotSuspendedReason.V, tc.Equals, "")
	}
}
func (s *modelRemoteApplicationSuite) assertRelationEndpoints(c *tc.C, relationUUID, app1UUID, app2UUID string) {
	appUUIDs := set.NewStrings(app1UUID, app2UUID)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `
SELECT ae.application_uuid
FROM   relation_endpoint AS re
JOIN   application_endpoint AS ae ON re.endpoint_uuid = ae.uuid
WHERE  relation_uuid = ?
`, relationUUID)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()

		for rows.Next() {
			var appUUID string

			if err := rows.Scan(&appUUID); err != nil {
				return err
			}
			if c.Check(appUUIDs.Contains(appUUID), tc.Equals, true) {
				appUUIDs.Remove(appUUID)
			}
		}
		return nil
	})
	c.Check(err, tc.ErrorIsNil)
	c.Check(appUUIDs.IsEmpty(), tc.Equals, true, tc.Commentf("relation_endpoint with app %q, not found", appUUIDs.SortedValues()))
}

func (s *modelRemoteApplicationSuite) assertRelationStatusJoining(c *tc.C, relationUUID string) {
	var statusID int

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `
SELECT relation_status_type_id
FROM   relation_status
WHERE  relation_uuid = ?
`, relationUUID).Scan(&statusID)

		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	expectedRelationID := tc.Must1(c, status.EncodeRelationStatus, status.RelationStatusTypeJoining)
	c.Check(statusID, tc.Equals, expectedRelationID)
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
	remoteAppUUID := tc.Must(c, coreapplication.NewUUID)
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
		ApplicationUUID:       remoteAppUUID.String(),
		CharmUUID:             remoteCharmUUID,
		RemoteApplicationUUID: tc.Must(c, internaluuid.NewUUID).String(),
		OfferUUID:             remoteOfferUUID,
		Charm:                 remoteCharm,
		EncodedMacaroon:       []byte("m"),
	})
	c.Assert(err, tc.ErrorIsNil)

	remoteEndpointUUID := s.getAppEndpointUUID(c, remoteAppUUID, "juju-info")

	// Create a non-offerer local application and one endpoint named "endpoint0".
	localOfferUUID := tc.Must(c, internaluuid.NewUUID).String()
	s.createOffer(c, localOfferUUID)
	localCharmUUID := tc.Must(c, internaluuid.NewUUID).String()
	s.createCharm(c, localCharmUUID)
	localAppUUID := tc.Must(c, coreapplication.NewUUID)
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
	remoteAppUUID := tc.Must(c, coreapplication.NewUUID)
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
		ApplicationUUID:       remoteAppUUID.String(),
		CharmUUID:             remoteCharmUUID,
		RemoteApplicationUUID: tc.Must(c, internaluuid.NewUUID).String(),
		OfferUUID:             remoteOfferUUID,
		Charm:                 remoteCharm,
		EncodedMacaroon:       []byte("m"),
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
	localAppUUID := tc.Must(c, coreapplication.NewUUID)
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
	remoteAppUUID := tc.Must(c, coreapplication.NewUUID)
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
		ApplicationUUID:       remoteAppUUID.String(),
		CharmUUID:             remoteCharmUUID,
		RemoteApplicationUUID: tc.Must(c, internaluuid.NewUUID).String(),
		OfferUUID:             remoteOfferUUID,
		Charm:                 remoteCharm,
		EncodedMacaroon:       []byte("m"),
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
func (s *modelRemoteApplicationSuite) getAppEndpointUUID(c *tc.C, applicationUUID coreapplication.UUID, endpointName string) string {
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

func (s *modelRemoteApplicationSuite) setupRelationStatus(c *tc.C, appName, appName2 string, statusID int) {
	charmUUID := s.addCharm(c)
	s.addCharmMetadata(c, charmUUID, false)
	relation := internalcharm.Relation{
		Name:      "db",
		Role:      internalcharm.RoleProvider,
		Interface: "db",
		Scope:     internalcharm.ScopeGlobal,
	}
	charmRelUUID := s.addCharmRelation(c, charmUUID, relation)

	charmUUID2 := s.addCharm(c)
	s.addCharmMetadata(c, charmUUID2, false)
	relation2 := internalcharm.Relation{
		Name:      "database",
		Role:      internalcharm.RoleRequirer,
		Interface: "db",
		Scope:     internalcharm.ScopeGlobal,
	}
	charmRelUUID2 := s.addCharmRelation(c, charmUUID, relation2)

	appUUID := s.addApplication(c, charmUUID, appName)
	appUUID2 := s.addApplication(c, charmUUID2, appName2)
	localEndpointUUID := s.addApplicationEndpoint(c, appUUID, charmRelUUID)
	localEndpointUUID2 := s.addApplicationEndpoint(c, appUUID2, charmRelUUID2)

	relUUID := s.addRelation(c)

	s.query(c, `
INSERT INTO relation_endpoint (uuid, relation_uuid, endpoint_uuid)
VALUES (?, ?, ?)`, internaluuid.MustNewUUID().String(), relUUID, localEndpointUUID2)
	s.query(c, `
INSERT INTO relation_endpoint (uuid, relation_uuid, endpoint_uuid)
VALUES (?, ?, ?)`, internaluuid.MustNewUUID().String(), relUUID, localEndpointUUID)

	s.query(c, `
INSERT INTO relation_status (relation_uuid, relation_status_type_id, updated_at)
VALUES (?, ?, 0)`, relUUID, statusID)
}

func (s *modelRemoteApplicationSuite) TestIsRelationWithEndpointIdentifiersSuspendedTrue(c *tc.C) {
	s.setupRelationStatus(c, "test", "remote-test", 4)
	isSuspended, err := s.state.IsRelationWithEndpointIdentifiersSuspended(
		c.Context(),
		corerelation.EndpointIdentifier{
			ApplicationName: "test",
			EndpointName:    "db",
		},
		corerelation.EndpointIdentifier{
			ApplicationName: "remote-test",
			EndpointName:    "database",
		},
	)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(isSuspended, tc.IsTrue)
}

func (s *modelRemoteApplicationSuite) TestIsRelationWithEndpointIdentifiersSuspendedFalse(c *tc.C) {
	s.setupRelationStatus(c, "test", "remote-test", 0)
	isSuspended, err := s.state.IsRelationWithEndpointIdentifiersSuspended(
		c.Context(),
		corerelation.EndpointIdentifier{
			ApplicationName: "test",
			EndpointName:    "db",
		},
		corerelation.EndpointIdentifier{
			ApplicationName: "remote-test",
			EndpointName:    "database",
		},
	)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(isSuspended, tc.IsFalse)
}

func (s *modelRemoteApplicationSuite) TestIsRelationWithEndpointIdentifiersSuspendedRelationNotFound(c *tc.C) {
	_, err := s.state.IsRelationWithEndpointIdentifiersSuspended(
		c.Context(),
		corerelation.EndpointIdentifier{
			ApplicationName: "test",
			EndpointName:    "db",
		},
		corerelation.EndpointIdentifier{
			ApplicationName: "remote-test",
			EndpointName:    "database",
		},
	)

	c.Assert(err, tc.ErrorIs, relationerrors.RelationNotFound)
}

func (s *modelRemoteApplicationSuite) TestGetOffererModelUUID(c *tc.C) {
	applicationUUID := tc.Must(c, coreapplication.NewUUID).String()
	charmUUID := tc.Must(c, corecharm.NewID).String()
	remoteAppUUID := tc.Must(c, coreapplication.NewUUID).String()
	offerUUID := tc.Must(c, coreoffer.NewUUID).String()
	offererModelUUID := tc.Must(c, coremodel.NewUUID)

	charm := charm.Charm{
		ReferenceName: "bar",
		Source:        charm.CMRSource,
		Metadata: charm.Metadata{
			Name:        "remote-app",
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
			Requires: map[string]charm.Relation{},
			Peers:    map[string]charm.Relation{},
		},
	}

	err := s.state.AddRemoteApplicationOfferer(c.Context(), "remote-app", crossmodelrelation.AddRemoteApplicationOffererArgs{
		ApplicationUUID:       applicationUUID,
		CharmUUID:             charmUUID,
		RemoteApplicationUUID: remoteAppUUID,
		OfferUUID:             offerUUID,
		Charm:                 charm,
		OffererModelUUID:      offererModelUUID.String(),
		EncodedMacaroon:       []byte("encoded macaroon"),
	})
	c.Assert(err, tc.ErrorIsNil)

	gotModelUUID, err := s.state.GetOffererModelUUID(c.Context(), "remote-app")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotModelUUID, tc.Equals, offererModelUUID)
}

func (s *modelRemoteApplicationSuite) TestGetOffererModelUUIDNotFound(c *tc.C) {
	_, err := s.state.GetOffererModelUUID(c.Context(), "non-existent-app")
	c.Assert(err, tc.ErrorIs, crossmodelrelationerrors.RemoteApplicationNotFound)
}

func (s *modelRemoteApplicationSuite) TestGetOffererModelUUIDNotRemoteOfferer(c *tc.C) {
	applicationUUID := tc.Must(c, coreapplication.NewUUID)
	charmUUID := tc.Must(c, corecharm.NewID).String()
	offerUUID := tc.Must(c, coreoffer.NewUUID).String()

	s.createOffer(c, offerUUID)
	s.createCharm(c, charmUUID)
	s.createApplication(c, applicationUUID, charmUUID, offerUUID)

	_, err := s.state.GetOffererModelUUID(c.Context(), "existing-app")
	c.Assert(err, tc.ErrorIs, crossmodelrelationerrors.RemoteApplicationNotFound)
}

func (s *modelRemoteApplicationSuite) TestCheckIsApplicationSyntheticNot(c *tc.C) {
	applicationUUID := tc.Must(c, coreapplication.NewUUID)
	charmUUID := tc.Must(c, corecharm.NewID).String()
	offerUUID := tc.Must(c, coreoffer.NewUUID).String()

	s.createOffer(c, offerUUID)
	s.createCharm(c, charmUUID)
	s.createApplication(c, applicationUUID, charmUUID, offerUUID)

	isSynthetic, err := s.state.IsApplicationSynthetic(c.Context(), applicationUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(isSynthetic, tc.IsFalse)
}

func (s *modelRemoteApplicationSuite) TestCheckIsApplicationConsumerNotFound(c *tc.C) {
	isSynthetic, err := s.state.IsApplicationSynthetic(c.Context(), "non-existent-app")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(isSynthetic, tc.IsFalse)
}

func (s *modelRemoteApplicationSuite) TestCheckIsApplicationSynthetic(c *tc.C) {
	applicationUUID := tc.Must(c, coreapplication.NewUUID).String()
	charmUUID := tc.Must(c, corecharm.NewID).String()
	remoteAppUUID := tc.Must(c, coreapplication.NewUUID).String()
	offerUUID := tc.Must(c, coreoffer.NewUUID).String()

	charm := charm.Charm{
		ReferenceName: "bar",
		Source:        charm.CMRSource,
		Metadata: charm.Metadata{
			Name:        "remote-app",
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
			Requires: map[string]charm.Relation{},
			Peers:    map[string]charm.Relation{},
		},
	}

	err := s.state.AddRemoteApplicationOfferer(c.Context(), "remote-app", crossmodelrelation.AddRemoteApplicationOffererArgs{
		ApplicationUUID:       applicationUUID,
		CharmUUID:             charmUUID,
		RemoteApplicationUUID: remoteAppUUID,
		OfferUUID:             offerUUID,
		Charm:                 charm,
		OffererModelUUID:      tc.Must(c, internaluuid.NewUUID).String(),
		EncodedMacaroon:       []byte("encoded macaroon"),
	})
	c.Assert(err, tc.ErrorIsNil)

	isSynthetic, err := s.state.IsApplicationSynthetic(c.Context(), "remote-app")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(isSynthetic, tc.IsTrue)
}

func (s *modelRemoteApplicationSuite) TestGetRelationRemoteModelUUIDConsumerSide(c *tc.C) {
	applicationUUID := tc.Must(c, coreapplication.NewUUID).String()
	charmUUID := tc.Must(c, corecharm.NewID).String()
	remoteAppUUID := tc.Must(c, coreapplication.NewUUID).String()
	offerUUID := tc.Must(c, coreoffer.NewUUID).String()
	offererModelUUID := tc.Must(c, coremodel.NewUUID)

	// Create a local application
	localAppUUID := tc.Must(c, coreapplication.NewUUID)
	localCharmUUID := tc.Must(c, corecharm.NewID).String()
	s.createOffer(c, offerUUID)
	s.createCharm(c, localCharmUUID)
	localCharmRelationUUID := s.createCharmRelation(c, localCharmUUID, "server")
	s.createApplication(c, localAppUUID, localCharmUUID, offerUUID)
	localEndpointUUID := s.addApplicationEndpoint(c, localAppUUID, localCharmRelationUUID)

	// Create the remote offerer application (synthetic app in consumer model).
	charm := charm.Charm{
		ReferenceName: "bar",
		Source:        charm.CMRSource,
		Metadata: charm.Metadata{
			Name:        "remote-app",
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
			Requires: map[string]charm.Relation{},
			Peers:    map[string]charm.Relation{},
		},
	}

	err := s.state.AddRemoteApplicationOfferer(c.Context(), "remote-app", crossmodelrelation.AddRemoteApplicationOffererArgs{
		ApplicationUUID:       applicationUUID,
		CharmUUID:             charmUUID,
		RemoteApplicationUUID: remoteAppUUID,
		OfferUUID:             offerUUID,
		Charm:                 charm,
		OffererModelUUID:      offererModelUUID.String(),
		EncodedMacaroon:       []byte("encoded macaroon"),
	})
	c.Assert(err, tc.ErrorIsNil)

	// Create a relation between the local app and the remote offerer.
	relationUUID := tc.Must(c, corerelation.NewUUID)
	remoteCharmRelationUUID := s.createCharmRelation(c, charmUUID, "client")
	remoteEndpointUUID := s.addApplicationEndpoint(c, coreapplication.UUID(applicationUUID), remoteCharmRelationUUID)

	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.Exec(`
INSERT INTO relation (uuid, life_id, relation_id, scope_id)
VALUES (?, 0, 1, 0)
`, relationUUID.String())
		if err != nil {
			return err
		}

		_, err = tx.Exec(`
INSERT INTO relation_endpoint (uuid, relation_uuid, endpoint_uuid)
VALUES (?, ?, ?), (?, ?, ?)
`, tc.Must(c, internaluuid.NewUUID).String(), relationUUID.String(), localEndpointUUID,
			tc.Must(c, internaluuid.NewUUID).String(), relationUUID.String(), remoteEndpointUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	gotModelUUID, err := s.state.GetRelationRemoteModelUUID(c.Context(), relationUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotModelUUID, tc.Equals, offererModelUUID)
}

func (s *modelRemoteApplicationSuite) TestGetRelationRemoteModelUUIDOffererSide(c *tc.C) {
	charmUUID := tc.Must(c, internaluuid.NewUUID).String()
	offerUUID := tc.Must(c, internaluuid.NewUUID).String()
	relationUUID := tc.Must(c, corerelation.NewUUID)
	consumerModelUUID := tc.Must(c, coremodel.NewUUID)
	consumerApplicationUUID := tc.Must(c, internaluuid.NewUUID).String()
	synthApplicationUUID := tc.Must(c, internaluuid.NewUUID).String()

	// Create the local offerer application and offer
	offerApplicationUUID := tc.Must(c, coreapplication.NewUUID)
	offerCharmUUID := tc.Must(c, internaluuid.NewUUID).String()
	s.createOffer(c, offerUUID)
	s.createCharm(c, offerCharmUUID)
	charmRelationUUID := s.createCharmRelation(c, offerCharmUUID, "offer-endpoint")
	s.createApplication(c, offerApplicationUUID, offerCharmUUID, offerUUID)
	s.addApplicationEndpoint(c, offerApplicationUUID, charmRelationUUID)

	// Create the remote consumer application (synthetic app in offerer model).
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
			Requires: map[string]charm.Relation{},
			Peers:    map[string]charm.Relation{},
		},
	}

	err := s.state.AddConsumedRelation(c.Context(), "foo", crossmodelrelation.AddRemoteApplicationConsumerArgs{
		OfferUUID:                   offerUUID,
		OfferEndpointName:           "offer-endpoint",
		RelationUUID:                relationUUID.String(),
		ConsumerModelUUID:           consumerModelUUID.String(),
		ConsumerApplicationUUID:     consumerApplicationUUID,
		ConsumerApplicationEndpoint: "db",
		SynthApplicationUUID:        synthApplicationUUID,
		CharmUUID:                   charmUUID,
		Charm:                       charm,
		Username:                    "consumer-user",
	})
	c.Assert(err, tc.ErrorIsNil)

	gotModelUUID, err := s.state.GetRelationRemoteModelUUID(c.Context(), relationUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotModelUUID, tc.Equals, consumerModelUUID)
}

func (s *modelRemoteApplicationSuite) TestGetRelationRemoteModelUUIDNotFound(c *tc.C) {
	// Test with a non-existent relation UUID
	nonExistentUUID := tc.Must(c, corerelation.NewUUID)
	_, err := s.state.GetRelationRemoteModelUUID(c.Context(), nonExistentUUID)
	c.Assert(err, tc.ErrorIs, relationerrors.RelationNotFound)
}

func (s *modelRemoteApplicationSuite) TestGetRelationRemoteModelUUIDNotCrossModel(c *tc.C) {
	// Create a local application with a peer relation.
	appUUID := tc.Must(c, coreapplication.NewUUID)
	charmUUID := tc.Must(c, corecharm.NewID).String()
	offerUUID := tc.Must(c, coreoffer.NewUUID).String()

	s.createOffer(c, offerUUID)
	s.createCharm(c, charmUUID)

	charmRelationUUID := s.createCharmRelation(c, charmUUID, "peer-relation")

	s.createApplication(c, appUUID, charmUUID, offerUUID)

	endpointUUID := s.addApplicationEndpoint(c, appUUID, charmRelationUUID)

	relationUUID := tc.Must(c, corerelation.NewUUID)
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		// Insert relation
		_, err := tx.Exec(`
INSERT INTO relation (uuid, life_id, relation_id, scope_id)
VALUES (?, 0, 1, 0)
`, relationUUID.String())
		if err != nil {
			return err
		}

		_, err = tx.Exec(`
INSERT INTO relation_endpoint (uuid, relation_uuid, endpoint_uuid)
VALUES (?, ?, ?)
`, tc.Must(c, internaluuid.NewUUID).String(), relationUUID.String(), endpointUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.state.GetRelationRemoteModelUUID(c.Context(), relationUUID)
	c.Assert(err, tc.ErrorIs, crossmodelrelationerrors.RelationNotRemote)
}
