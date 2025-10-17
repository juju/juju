// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package service

import (
	"context"
	"testing"
	"time"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreapplication "github.com/juju/juju/core/application"
	coreapplicationtesting "github.com/juju/juju/core/application/testing"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/offer"
	corerelation "github.com/juju/juju/core/relation"
	status "github.com/juju/juju/core/status"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/crossmodelrelation"
	crossmodelrelationerrors "github.com/juju/juju/domain/crossmodelrelation/errors"
	"github.com/juju/juju/domain/life"
	domainstatus "github.com/juju/juju/domain/status"
	internalerrors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

type remoteApplicationServiceSuite struct {
	baseSuite
}

func TestRemoteApplicationServiceSuite(t *testing.T) {
	tc.Run(t, &remoteApplicationServiceSuite{})
}

func (s *remoteApplicationServiceSuite) TestAddRemoteApplicationOfferer(c *tc.C) {
	defer s.setupMocks(c).Finish()

	offerUUID := tc.Must(c, offer.NewUUID)
	offererControllerUUID := ptr(tc.Must(c, uuid.NewUUID).String())
	offererModelUUID := tc.Must(c, uuid.NewUUID).String()
	macaroon := newMacaroon(c, "test")

	syntheticCharm := charm.Charm{
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
		ReferenceName: "foo",
		Source:        charm.CMRSource,
	}
	var received crossmodelrelation.AddRemoteApplicationOffererArgs
	s.modelState.EXPECT().AddRemoteApplicationOfferer(gomock.Any(), "foo", gomock.Any()).DoAndReturn(func(_ context.Context, _ string, args crossmodelrelation.AddRemoteApplicationOffererArgs) error {
		received = args
		return nil
	})

	service := s.service(c)

	err := service.AddRemoteApplicationOfferer(c.Context(), "foo", AddRemoteApplicationOffererArgs{
		OfferUUID:             offerUUID,
		OffererControllerUUID: offererControllerUUID,
		OffererModelUUID:      offererModelUUID,
		OfferURL:              tc.Must1(c, crossmodel.ParseOfferURL, "controller:qualifier/model.offername"),
		Endpoints: []charm.Relation{{
			Name:      "db",
			Role:      charm.RoleProvider,
			Interface: "db",
			Limit:     1,
			Scope:     charm.ScopeGlobal,
		}, {
			Name:      "cache",
			Role:      charm.RoleRequirer,
			Interface: "cacher",
			Scope:     charm.ScopeGlobal,
		}},
		Macaroon: macaroon,
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Check(received.RemoteApplicationUUID, tc.IsUUID)
	c.Check(received.ApplicationUUID, tc.IsUUID)
	c.Check(received.CharmUUID, tc.IsUUID)

	received.RemoteApplicationUUID = ""
	received.ApplicationUUID = ""
	received.CharmUUID = ""

	c.Check(received, tc.DeepEquals, crossmodelrelation.AddRemoteApplicationOffererArgs{
		AddRemoteApplicationArgs: crossmodelrelation.AddRemoteApplicationArgs{
			Charm:     syntheticCharm,
			OfferUUID: offerUUID.String(),
		},
		OfferURL:              "controller:qualifier/model.offername",
		OffererControllerUUID: offererControllerUUID,
		OffererModelUUID:      offererModelUUID,
		EncodedMacaroon:       tc.Must(c, macaroon.MarshalJSON),
	})
}

func (s *remoteApplicationServiceSuite) TestAddRemoteApplicationOffererNoEndpoints(c *tc.C) {
	defer s.setupMocks(c).Finish()

	offerUUID := tc.Must(c, offer.NewUUID)

	service := s.service(c)

	err := service.AddRemoteApplicationOfferer(c.Context(), "foo", AddRemoteApplicationOffererArgs{
		OfferUUID: offerUUID,
	})
	c.Assert(err, tc.ErrorIs, errors.NotValid)
}

func (s *remoteApplicationServiceSuite) TestAddRemoteApplicationOffererInvalidApplicationName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	service := s.service(c)

	err := service.AddRemoteApplicationOfferer(c.Context(), "!foo", AddRemoteApplicationOffererArgs{})
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNameNotValid)
}

func (s *remoteApplicationServiceSuite) TestAddRemoteApplicationOffererInvalidOfferUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	service := s.service(c)

	err := service.AddRemoteApplicationOfferer(c.Context(), "foo", AddRemoteApplicationOffererArgs{
		OfferUUID: "!!",
	})
	c.Assert(err, tc.ErrorIs, errors.NotValid)
}

func (s *remoteApplicationServiceSuite) TestAddRemoteApplicationOffererInvalidOffererModelUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	offerUUID := tc.Must(c, offer.NewUUID)
	offererModelUUID := "!!"

	service := s.service(c)

	err := service.AddRemoteApplicationOfferer(c.Context(), "foo", AddRemoteApplicationOffererArgs{
		OfferUUID:        offerUUID,
		OffererModelUUID: offererModelUUID,
	})
	c.Assert(err, tc.ErrorIs, errors.NotValid)
}

func (s *remoteApplicationServiceSuite) TestAddRemoteApplicationOffererInvalidRole(c *tc.C) {
	defer s.setupMocks(c).Finish()

	offerUUID := tc.Must(c, offer.NewUUID)
	offererControllerUUID := ptr(tc.Must(c, uuid.NewUUID).String())
	offererModelUUID := tc.Must(c, uuid.NewUUID).String()
	macaroon := newMacaroon(c, "test")

	service := s.service(c)

	err := service.AddRemoteApplicationOfferer(c.Context(), "foo", AddRemoteApplicationOffererArgs{
		OfferUUID:             offerUUID,
		OffererControllerUUID: offererControllerUUID,
		OffererModelUUID:      offererModelUUID,
		Endpoints: []charm.Relation{{
			Name:      "db",
			Role:      charm.RoleProvider,
			Interface: "db",
			Limit:     1,
			Scope:     charm.ScopeContainer,
		}},
		Macaroon: macaroon,
	})
	c.Assert(err, tc.ErrorIs, errors.NotValid)

}

func (s *remoteApplicationServiceSuite) TestGetRemoteApplicationOfferers(c *tc.C) {
	defer s.setupMocks(c).Finish()

	expected := []crossmodelrelation.RemoteApplicationOfferer{{
		ApplicationUUID: "app-uuid",
		ApplicationName: "foo",
		Life:            life.Alive,
		OfferUUID:       "offer-uuid",
		ConsumeVersion:  1,
	}, {
		ApplicationUUID: "app-uuid-2",
		ApplicationName: "bar",
		Life:            life.Dead,
		OfferUUID:       "offer-uuid-2",
		ConsumeVersion:  2,
	}}

	s.modelState.EXPECT().GetRemoteApplicationOfferers(gomock.Any()).Return(expected, nil)

	service := s.service(c)

	actual, err := service.GetRemoteApplicationOfferers(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(actual, tc.DeepEquals, expected)
}

func (s *remoteApplicationServiceSuite) TestGetRemoteApplicationOfferersNoRemoteApplicationOfferers(c *tc.C) {
	defer s.setupMocks(c).Finish()

	expected := []crossmodelrelation.RemoteApplicationOfferer{}

	s.modelState.EXPECT().GetRemoteApplicationOfferers(gomock.Any()).Return(expected, nil)

	service := s.service(c)

	actual, err := service.GetRemoteApplicationOfferers(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(actual, tc.DeepEquals, expected)
}

func (s *remoteApplicationServiceSuite) TestGetRemoteApplicationOfferersError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.modelState.EXPECT().GetRemoteApplicationOfferers(gomock.Any()).Return(nil, internalerrors.Errorf("front fell off"))

	service := s.service(c)

	_, err := service.GetRemoteApplicationOfferers(c.Context())
	c.Assert(err, tc.ErrorMatches, "front fell off")
}

func (s *remoteApplicationServiceSuite) TestSaveMacaroonForRelation(c *tc.C) {
	defer s.setupMocks(c).Finish()

	relationUUID := tc.Must(c, corerelation.NewUUID)
	mac := newMacaroon(c, "test")
	encodedMac := tc.Must(c, mac.MarshalJSON)

	s.modelState.EXPECT().SaveMacaroonForRelation(gomock.Any(), relationUUID.String(), encodedMac).Return(nil)

	service := s.service(c)

	err := service.SaveMacaroonForRelation(c.Context(), relationUUID, mac)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *remoteApplicationServiceSuite) TestSaveMacaroonForRelationInvalidRelationUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	service := s.service(c)

	err := service.SaveMacaroonForRelation(c.Context(), "foo", nil)
	c.Assert(err, tc.ErrorIs, errors.NotValid)
}

func (s *remoteApplicationServiceSuite) TestSaveMacaroonForRelationInvalidMacaroon(c *tc.C) {
	defer s.setupMocks(c).Finish()

	relationUUID := tc.Must(c, corerelation.NewUUID)

	service := s.service(c)

	err := service.SaveMacaroonForRelation(c.Context(), relationUUID, nil)
	c.Assert(err, tc.ErrorIs, errors.NotValid)
}

func (s *remoteApplicationServiceSuite) TestAddRemoteApplicationConsumer(c *tc.C) {
	defer s.setupMocks(c).Finish()

	offerUUID := tc.Must(c, offer.NewUUID)
	relationUUID := tc.Must(c, uuid.NewUUID).String()
	consumerModelUUID := tc.Must(c, uuid.NewUUID).String()

	remoteApplicationUUID := "deadbeef-1bad-500d-9000-4b1d0d06f00d"

	syntheticCharm := charm.Charm{
		Metadata: charm.Metadata{
			Name:        "remote-deadbeef1bad500d90004b1d0d06f00d",
			Description: "remote offerer application",
			Provides: map[string]charm.Relation{
				"db": {
					Name:      "db",
					Role:      charm.RoleProvider,
					Interface: "database",
					Limit:     1,
					Scope:     charm.ScopeGlobal,
				},
			},
			Requires: map[string]charm.Relation{},
			Peers:    map[string]charm.Relation{},
		},
		ReferenceName: "remote-deadbeef1bad500d90004b1d0d06f00d",
		Source:        charm.CMRSource,
	}

	var received crossmodelrelation.AddRemoteApplicationConsumerArgs
	s.modelState.EXPECT().AddRemoteApplicationConsumer(gomock.Any(), "remote-deadbeef1bad500d90004b1d0d06f00d", gomock.Any()).DoAndReturn(func(_ context.Context, _ string, args crossmodelrelation.AddRemoteApplicationConsumerArgs) error {
		received = args
		return nil
	})

	service := s.service(c)

	err := service.AddRemoteApplicationConsumer(c.Context(), AddRemoteApplicationConsumerArgs{
		RemoteApplicationUUID: remoteApplicationUUID,
		OfferUUID:             offerUUID,
		RelationUUID:          relationUUID,
		ConsumerModelUUID:     consumerModelUUID,
		Endpoints: []charm.Relation{{
			Name:      "db",
			Role:      charm.RoleProvider,
			Interface: "database",
			Limit:     1,
			Scope:     charm.ScopeGlobal,
		}},
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Check(received.RemoteApplicationUUID, tc.IsUUID)
	c.Check(received.ApplicationUUID, tc.IsUUID)
	c.Check(received.CharmUUID, tc.IsUUID)

	received.RemoteApplicationUUID = ""
	received.ApplicationUUID = ""
	received.CharmUUID = ""

	c.Check(received, tc.DeepEquals, crossmodelrelation.AddRemoteApplicationConsumerArgs{
		AddRemoteApplicationArgs: crossmodelrelation.AddRemoteApplicationArgs{
			Charm:             syntheticCharm,
			OfferUUID:         offerUUID.String(),
			ConsumerModelUUID: consumerModelUUID,
		},
		RelationUUID: relationUUID,
	})
}

func (s *remoteApplicationServiceSuite) TestAddRemoteApplicationConsumerInvalidRemoteApplicationUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	service := s.service(c)

	err := service.AddRemoteApplicationConsumer(c.Context(), AddRemoteApplicationConsumerArgs{
		RemoteApplicationUUID: "invalid-uuid",
	})
	c.Assert(err, tc.ErrorIs, errors.NotValid)
	c.Assert(err, tc.ErrorMatches, "remote application UUID \"invalid-uuid\" is not a valid UUID")
}

func (s *remoteApplicationServiceSuite) TestAddRemoteApplicationConsumerInvalidOfferUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	remoteApplicationUUID := "deadbeef-1bad-500d-9000-4b1d0d06f00d"

	service := s.service(c)

	err := service.AddRemoteApplicationConsumer(c.Context(), AddRemoteApplicationConsumerArgs{
		RemoteApplicationUUID: remoteApplicationUUID,
		OfferUUID:             "!!",
	})
	c.Assert(err, tc.ErrorIs, errors.NotValid)
	c.Assert(err, tc.ErrorMatches, `.*uuid "!!" not valid`)
}

func (s *remoteApplicationServiceSuite) TestAddRemoteApplicationConsumerInvalidRelationUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	remoteApplicationUUID := "deadbeef-1bad-500d-9000-4b1d0d06f00d"
	offerUUID := tc.Must(c, offer.NewUUID)

	service := s.service(c)

	err := service.AddRemoteApplicationConsumer(c.Context(), AddRemoteApplicationConsumerArgs{
		RemoteApplicationUUID: remoteApplicationUUID,
		OfferUUID:             offerUUID,
		RelationUUID:          "!!",
	})
	c.Assert(err, tc.ErrorIs, errors.NotValid)
	c.Assert(err, tc.ErrorMatches, "relation UUID \"!!\" is not a valid UUID")
}

func (s *remoteApplicationServiceSuite) TestAddRemoteApplicationConsumerNoEndpoints(c *tc.C) {
	defer s.setupMocks(c).Finish()

	remoteApplicationUUID := "deadbeef-1bad-500d-9000-4b1d0d06f00d"
	offerUUID := tc.Must(c, offer.NewUUID)
	relationUUID := tc.Must(c, uuid.NewUUID).String()
	consumerModelUUID := tc.Must(c, uuid.NewUUID).String()

	service := s.service(c)

	err := service.AddRemoteApplicationConsumer(c.Context(), AddRemoteApplicationConsumerArgs{
		RemoteApplicationUUID: remoteApplicationUUID,
		OfferUUID:             offerUUID,
		RelationUUID:          relationUUID,
		ConsumerModelUUID:     consumerModelUUID,
		Endpoints:             []charm.Relation{},
	})
	c.Assert(err, tc.ErrorIs, errors.NotValid)
	c.Assert(err, tc.ErrorMatches, "endpoints cannot be empty")
}

func (s *remoteApplicationServiceSuite) TestAddRemoteApplicationConsumerInvalidEndpointScope(c *tc.C) {
	defer s.setupMocks(c).Finish()

	remoteApplicationUUID := "deadbeef-1bad-500d-9000-4b1d0d06f00d"
	offerUUID := tc.Must(c, offer.NewUUID)
	relationUUID := tc.Must(c, uuid.NewUUID).String()
	consumerModelUUID := tc.Must(c, uuid.NewUUID).String()

	service := s.service(c)

	err := service.AddRemoteApplicationConsumer(c.Context(), AddRemoteApplicationConsumerArgs{
		RemoteApplicationUUID: remoteApplicationUUID,
		OfferUUID:             offerUUID,
		RelationUUID:          relationUUID,
		ConsumerModelUUID:     consumerModelUUID,
		Endpoints: []charm.Relation{{
			Name:      "db",
			Role:      charm.RoleProvider,
			Interface: "database",
			Limit:     1,
			Scope:     charm.ScopeContainer,
		}},
	})
	c.Assert(err, tc.ErrorIs, errors.NotValid)
	c.Assert(err, tc.ErrorMatches, "endpoint \"db\" has non-global scope \"container\"")
}

func (s *remoteApplicationServiceSuite) TestAddRemoteApplicationConsumerInvalidConsumerModel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	remoteApplicationUUID := "deadbeef-1bad-500d-9000-4b1d0d06f00d"
	offerUUID := tc.Must(c, offer.NewUUID)
	relationUUID := tc.Must(c, uuid.NewUUID).String()

	service := s.service(c)

	err := service.AddRemoteApplicationConsumer(c.Context(), AddRemoteApplicationConsumerArgs{
		RemoteApplicationUUID: remoteApplicationUUID,
		OfferUUID:             offerUUID,
		RelationUUID:          relationUUID,
		ConsumerModelUUID:     "!!",
		Endpoints: []charm.Relation{{
			Name:      "db",
			Role:      charm.RoleProvider,
			Interface: "database",
			Limit:     1,
			Scope:     charm.ScopeContainer,
		}},
	})
	c.Assert(err, tc.ErrorIs, errors.NotValid)
	c.Assert(err, tc.ErrorMatches, "consumer model UUID \"!!\" is not a valid UUID")
}

func (s *remoteApplicationServiceSuite) TestAddRemoteApplicationConsumerStateError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	remoteApplicationUUID := "deadbeef-1bad-500d-9000-4b1d0d06f00d"
	offerUUID := tc.Must(c, offer.NewUUID)
	relationUUID := tc.Must(c, uuid.NewUUID).String()
	consumerModelUUID := tc.Must(c, uuid.NewUUID).String()

	s.modelState.EXPECT().AddRemoteApplicationConsumer(gomock.Any(), "remote-deadbeef1bad500d90004b1d0d06f00d", gomock.Any()).Return(internalerrors.Errorf("boom"))

	service := s.service(c)

	err := service.AddRemoteApplicationConsumer(c.Context(), AddRemoteApplicationConsumerArgs{
		RemoteApplicationUUID: remoteApplicationUUID,
		OfferUUID:             offerUUID,
		RelationUUID:          relationUUID,
		ConsumerModelUUID:     consumerModelUUID,
		Endpoints: []charm.Relation{{
			Name:      "db",
			Role:      charm.RoleProvider,
			Interface: "database",
			Limit:     1,
			Scope:     charm.ScopeGlobal,
		}},
	})
	c.Assert(err, tc.ErrorMatches, "inserting remote application consumer: boom")
}

func (s *remoteApplicationServiceSuite) TestAddRemoteApplicationConsumerMixedEndpoints(c *tc.C) {
	defer s.setupMocks(c).Finish()

	offerUUID := tc.Must(c, offer.NewUUID)
	relationUUID := tc.Must(c, uuid.NewUUID).String()
	remoteApplicationUUID := "deadbeef-1bad-500d-9000-4b1d0d06f00d"
	consumerModelUUID := tc.Must(c, uuid.NewUUID).String()

	syntheticCharm := charm.Charm{
		Metadata: charm.Metadata{
			Name:        "remote-deadbeef1bad500d90004b1d0d06f00d",
			Description: "remote offerer application",
			Provides: map[string]charm.Relation{
				"web": {
					Name:      "web",
					Role:      charm.RoleProvider,
					Interface: "http",
					Limit:     0,
					Scope:     charm.ScopeGlobal,
				},
			},
			Requires: map[string]charm.Relation{
				"db": {
					Name:      "db",
					Role:      charm.RoleRequirer,
					Interface: "database",
					Scope:     charm.ScopeGlobal,
				},
			},
			Peers: map[string]charm.Relation{},
		},
		ReferenceName: "remote-deadbeef1bad500d90004b1d0d06f00d",
		Source:        charm.CMRSource,
	}

	var received crossmodelrelation.AddRemoteApplicationConsumerArgs
	s.modelState.EXPECT().AddRemoteApplicationConsumer(gomock.Any(), "remote-deadbeef1bad500d90004b1d0d06f00d", gomock.Any()).DoAndReturn(func(_ context.Context, _ string, args crossmodelrelation.AddRemoteApplicationConsumerArgs) error {
		received = args
		return nil
	})

	service := s.service(c)

	err := service.AddRemoteApplicationConsumer(c.Context(), AddRemoteApplicationConsumerArgs{
		RemoteApplicationUUID: remoteApplicationUUID,
		OfferUUID:             offerUUID,
		RelationUUID:          relationUUID,
		ConsumerModelUUID:     consumerModelUUID,
		Endpoints: []charm.Relation{{
			Name:      "web",
			Role:      charm.RoleProvider,
			Interface: "http",
			Limit:     0,
			Scope:     charm.ScopeGlobal,
		}, {
			Name:      "db",
			Role:      charm.RoleRequirer,
			Interface: "database",
			Scope:     charm.ScopeGlobal,
		}},
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Check(received.RemoteApplicationUUID, tc.IsUUID)
	c.Check(received.ApplicationUUID, tc.IsUUID)
	c.Check(received.CharmUUID, tc.IsUUID)

	received.RemoteApplicationUUID = ""
	received.ApplicationUUID = ""
	received.CharmUUID = ""

	c.Check(received, tc.DeepEquals, crossmodelrelation.AddRemoteApplicationConsumerArgs{
		AddRemoteApplicationArgs: crossmodelrelation.AddRemoteApplicationArgs{
			Charm:             syntheticCharm,
			OfferUUID:         offerUUID.String(),
			ConsumerModelUUID: consumerModelUUID,
		},
		RelationUUID: relationUUID,
	})
}

func (s *remoteApplicationServiceSuite) TestGetApplicationNameAndUUIDByOfferUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	offerUUID := tc.Must(c, offer.NewUUID)
	appUUID := coreapplicationtesting.GenApplicationUUID(c)

	s.modelState.EXPECT().GetApplicationNameAndUUIDByOfferUUID(gomock.Any(), offerUUID.String()).Return("test-app", appUUID, nil)

	service := s.service(c)

	gotName, gotUUID, err := service.GetApplicationNameAndUUIDByOfferUUID(c.Context(), offerUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotName, tc.Equals, "test-app")
	c.Check(gotUUID, tc.Equals, appUUID)
}

func (s *remoteApplicationServiceSuite) TestGetApplicationNameAndUUIDByOfferUUIDNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	offerUUID := tc.Must(c, offer.NewUUID)

	s.modelState.EXPECT().GetApplicationNameAndUUIDByOfferUUID(gomock.Any(), offerUUID.String()).Return("", coreapplication.UUID(""), crossmodelrelationerrors.OfferNotFound)

	service := s.service(c)

	_, _, err := service.GetApplicationNameAndUUIDByOfferUUID(c.Context(), offerUUID)
	c.Assert(err, tc.ErrorMatches, "offer not found")
	c.Assert(err, tc.ErrorIs, crossmodelrelationerrors.OfferNotFound)
}

func (s *remoteApplicationServiceSuite) TestGetApplicationNameAndUUIDByOfferUUIDInvalidUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	service := s.service(c)

	_, _, err := service.GetApplicationNameAndUUIDByOfferUUID(c.Context(), "invalid-uuid")
	c.Assert(err, tc.ErrorMatches, `.*uuid "invalid-uuid" not valid`)
	c.Assert(err, tc.ErrorIs, errors.NotValid)
}

func (s *remoteApplicationServiceSuite) TestGetApplicationNameAndUUIDByOfferUUIDStateError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	offerUUID := tc.Must(c, offer.NewUUID)

	s.modelState.EXPECT().GetApplicationNameAndUUIDByOfferUUID(gomock.Any(), offerUUID.String()).Return("", coreapplication.UUID(""), internalerrors.Errorf("boom"))

	service := s.service(c)

	_, _, err := service.GetApplicationNameAndUUIDByOfferUUID(c.Context(), offerUUID)
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *remoteApplicationServiceSuite) TestGetRemoteApplicationConsumers(c *tc.C) {
	defer s.setupMocks(c).Finish()

	expected := []crossmodelrelation.RemoteApplicationConsumer{{
		ApplicationName: "remote-app-1",
		Life:            life.Alive,
		OfferUUID:       "offer-uuid-1",
		ConsumeVersion:  0,
	}, {
		ApplicationName: "remote-app-2",
		Life:            life.Dying,
		OfferUUID:       "offer-uuid-2",
		ConsumeVersion:  3,
	}}

	s.modelState.EXPECT().GetRemoteApplicationConsumers(gomock.Any()).Return(expected, nil)

	service := s.service(c)

	actual, err := service.GetRemoteApplicationConsumers(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(actual, tc.DeepEquals, expected)
}

func (s *remoteApplicationServiceSuite) TestGetRemoteApplicationConsumersEmpty(c *tc.C) {
	defer s.setupMocks(c).Finish()

	expected := []crossmodelrelation.RemoteApplicationConsumer{}

	s.modelState.EXPECT().GetRemoteApplicationConsumers(gomock.Any()).Return(expected, nil)

	service := s.service(c)

	actual, err := service.GetRemoteApplicationConsumers(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(actual, tc.DeepEquals, expected)
}

func (s *remoteApplicationServiceSuite) TestGetRemoteApplicationConsumersError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.modelState.EXPECT().GetRemoteApplicationConsumers(gomock.Any()).Return(nil, internalerrors.Errorf("front fell off"))

	service := s.service(c)

	_, err := service.GetRemoteApplicationConsumers(c.Context())
	c.Assert(err, tc.ErrorMatches, "front fell off")
}

func (s *remoteApplicationServiceSuite) TestEnsureUnitsExist(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := coreapplicationtesting.GenApplicationUUID(c)
	units := []unit.Name{
		unit.Name("remote-app/0"),
		unit.Name("remote-app/1"),
		unit.Name("remote-app/2"),
	}

	s.modelState.EXPECT().EnsureUnitsExist(gomock.Any(), appUUID.String(), []string{
		"remote-app/0",
		"remote-app/1",
		"remote-app/2",
	}).Return(nil)

	service := s.service(c)

	err := service.EnsureUnitsExist(c.Context(), appUUID, units)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *remoteApplicationServiceSuite) TestEnsureUnitsExistNoUnits(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := coreapplicationtesting.GenApplicationUUID(c)
	units := []unit.Name{}

	service := s.service(c)

	err := service.EnsureUnitsExist(c.Context(), appUUID, units)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *remoteApplicationServiceSuite) TestEnsureUnitsExistInvalidAppUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	units := []unit.Name{
		unit.Name("remote-app/0"),
		unit.Name("remote-app/1"),
		unit.Name("remote-app/2"),
	}

	service := s.service(c)

	err := service.EnsureUnitsExist(c.Context(), coreapplication.UUID("!!!"), units)
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationUUIDNotValid)
}

func (s *remoteApplicationServiceSuite) TestEnsureUnitsExistInvalidUnitNames(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := coreapplicationtesting.GenApplicationUUID(c)
	units := []unit.Name{
		unit.Name("remote-app/0"),
		unit.Name("remote-app/1"),
		unit.Name("!!"),
	}

	service := s.service(c)

	err := service.EnsureUnitsExist(c.Context(), appUUID, units)
	c.Assert(err, tc.ErrorIs, unit.InvalidUnitName)
}

func (s *remoteApplicationServiceSuite) TestSetRemoteApplicationOffererStatus(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()
	appUUID := tc.Must(c, coreapplication.NewID)

	s.modelState.EXPECT().
		SetRemoteApplicationOffererStatus(gomock.Any(), appUUID.String(), domainstatus.StatusInfo[domainstatus.WorkloadStatusType]{
			Status:  domainstatus.WorkloadStatusError,
			Message: "failed successfully",
			Data:    []byte(`{"foo":"bar"}`),
			Since:   ptr(now),
		}).Return(nil)

	service := s.service(c)

	err := service.SetRemoteApplicationOffererStatus(c.Context(), appUUID, status.StatusInfo{
		Status:  status.Error,
		Message: "failed successfully",
		Data: map[string]any{
			"foo": "bar",
		},
		Since: ptr(now),
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *remoteApplicationServiceSuite) TestSetRemoteApplicationOffererStatusInvalidAppUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()

	service := s.service(c)

	err := service.SetRemoteApplicationOffererStatus(c.Context(), "!!!", status.StatusInfo{
		Status:  status.Error,
		Message: "failed successfully",
		Data: map[string]any{
			"foo": "bar",
		},
		Since: ptr(now),
	})
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationUUIDNotValid)
}

func (s *remoteApplicationServiceSuite) TestSetRemoteApplicationOffererStatusNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()
	appUUID := tc.Must(c, coreapplication.NewID)

	s.modelState.EXPECT().
		SetRemoteApplicationOffererStatus(gomock.Any(), appUUID.String(), domainstatus.StatusInfo[domainstatus.WorkloadStatusType]{
			Status:  domainstatus.WorkloadStatusError,
			Message: "failed successfully",
			Data:    []byte(`{"foo":"bar"}`),
			Since:   ptr(now),
		}).Return(crossmodelrelationerrors.RemoteApplicationNotFound)

	service := s.service(c)

	err := service.SetRemoteApplicationOffererStatus(c.Context(), appUUID, status.StatusInfo{
		Status:  status.Error,
		Message: "failed successfully",
		Data: map[string]any{
			"foo": "bar",
		},
		Since: ptr(now),
	})
	c.Assert(err, tc.ErrorIs, crossmodelrelationerrors.RemoteApplicationNotFound)
}

func (s *remoteApplicationServiceSuite) TestSetRemoteApplicationOffererStatusInvalidStatus(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()
	appUUID := tc.Must(c, coreapplication.NewID)

	service := s.service(c)

	err := service.SetRemoteApplicationOffererStatus(c.Context(), appUUID, status.StatusInfo{
		Status:  status.Aborted,
		Message: "failed successfully",
		Data: map[string]any{
			"foo": "bar",
		},
		Since: ptr(now),
	})
	c.Assert(err, tc.ErrorMatches, `.*unknown workload status "aborted"`)
}
