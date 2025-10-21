// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package service

import (
	"context"
	"strings"
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/offer"
	corerelation "github.com/juju/juju/core/relation"
	coreremoteapplication "github.com/juju/juju/core/remoteapplication"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/crossmodelrelation"
	crossmodelrelationerrors "github.com/juju/juju/domain/crossmodelrelation/errors"
	"github.com/juju/juju/domain/life"
	internalcharm "github.com/juju/juju/internal/charm"
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

func (s *remoteApplicationServiceSuite) TestGetRemoteApplicationOffererByApplicationName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appName := "test-app"
	uuid := tc.Must(c, coreremoteapplication.NewUUID)

	s.modelState.EXPECT().GetRemoteApplicationOffererByApplicationName(gomock.Any(), appName).Return(uuid.String(), nil)

	service := s.service(c)

	result, err := service.GetRemoteApplicationOffererByApplicationName(c.Context(), appName)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.Equals, uuid)
}

func (s *remoteApplicationServiceSuite) TestGetRemoteApplicationOffererByApplicationNameInvalidUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appName := "test-app"
	uuid := "invalid-uuid"

	service := s.service(c)

	s.modelState.EXPECT().GetRemoteApplicationOffererByApplicationName(gomock.Any(), appName).Return(uuid, nil)

	_, err := service.GetRemoteApplicationOffererByApplicationName(c.Context(), appName)
	c.Assert(err, tc.ErrorMatches, `.*id "invalid-uuid" not valid`)
	c.Assert(err, tc.ErrorIs, errors.NotValid)
}

func (s *remoteApplicationServiceSuite) TestGetRemoteApplicationOffererByApplicationNameStateError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appName := "test-app"

	s.modelState.EXPECT().GetRemoteApplicationOffererByApplicationName(gomock.Any(), appName).Return("", internalerrors.Errorf("boom"))

	service := s.service(c)

	_, err := service.GetRemoteApplicationOffererByApplicationName(c.Context(), appName)
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *remoteApplicationServiceSuite) TestGetMacaroonForRelation(c *tc.C) {
	defer s.setupMocks(c).Finish()

	relationUUID := tc.Must(c, corerelation.NewUUID)
	mac := newMacaroon(c, "test")

	s.modelState.EXPECT().GetMacaroonForRelation(gomock.Any(), relationUUID.String()).Return(mac, nil)

	service := s.service(c)

	got, err := service.GetMacaroonForRelation(c.Context(), relationUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.DeepEquals, mac)
}

func (s *remoteApplicationServiceSuite) TestGetMacaroonForRelationInvalidRelationUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	service := s.service(c)

	_, err := service.GetMacaroonForRelation(c.Context(), "foo")
	c.Assert(err, tc.ErrorIs, errors.NotValid)
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
	applicationUUID := tc.Must(c, coreapplication.NewUUID).String()

	name := "remote-" + strings.ReplaceAll(applicationUUID, "-", "")

	syntheticCharm := charm.Charm{
		Metadata: charm.Metadata{
			Name:        name,
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
		ReferenceName: name,
		Source:        charm.CMRSource,
	}

	var received crossmodelrelation.AddRemoteApplicationConsumerArgs
	s.modelState.EXPECT().
		AddRemoteApplicationConsumer(gomock.Any(), name, gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, args crossmodelrelation.AddRemoteApplicationConsumerArgs) error {
			received = args
			return nil
		})

	service := s.service(c)

	err := service.AddRemoteApplicationConsumer(c.Context(), AddRemoteApplicationConsumerArgs{
		RemoteApplicationUUID: applicationUUID,
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

	c.Check(received.CharmUUID, tc.IsUUID)
	received.CharmUUID = ""

	c.Check(received, tc.DeepEquals, crossmodelrelation.AddRemoteApplicationConsumerArgs{
		AddRemoteApplicationArgs: crossmodelrelation.AddRemoteApplicationArgs{
			ApplicationUUID:       applicationUUID,
			RemoteApplicationUUID: applicationUUID,
			Charm:                 syntheticCharm,
			OfferUUID:             offerUUID.String(),
			ConsumerModelUUID:     consumerModelUUID,
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
	appUUID := tc.Must(c, coreapplication.NewUUID)

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

func (s *remoteApplicationServiceSuite) TestGetOfferingApplicationToken(c *tc.C) {
	defer s.setupMocks(c).Finish()

	relationUUID := tc.Must(c, corerelation.NewUUID)
	appUUID := tc.Must(c, coreapplication.NewUUID)

	s.modelState.EXPECT().GetOfferingApplicationToken(gomock.Any(), relationUUID.String()).Return(appUUID.String(), nil)
	service := s.service(c)

	obtainedToken, err := service.GetOfferingApplicationToken(c.Context(), relationUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtainedToken, tc.Equals, appUUID)
}

func (s *remoteApplicationServiceSuite) TestGetOfferingApplicationTokenInvalidUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service(c).GetOfferingApplicationToken(c.Context(), "bad-uuid")
	c.Assert(err, tc.ErrorIs, errors.NotValid)
}

func (s *remoteApplicationServiceSuite) TestGetOfferingApplicationTokenError(c *tc.C) {
	defer s.setupMocks(c).Finish()
	relationUUID := tc.Must(c, corerelation.NewUUID)

	s.modelState.EXPECT().GetOfferingApplicationToken(gomock.Any(), relationUUID.String()).Return("", internalerrors.Errorf("front fell off"))

	service := s.service(c)

	_, err := service.GetOfferingApplicationToken(c.Context(), relationUUID)
	c.Assert(err, tc.ErrorMatches, "front fell off")
}

func (s *remoteApplicationServiceSuite) TestEnsureUnitsExist(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := tc.Must(c, coreapplication.NewUUID)
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

	appUUID := tc.Must(c, coreapplication.NewUUID)
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

	appUUID := tc.Must(c, coreapplication.NewUUID)
	units := []unit.Name{
		unit.Name("remote-app/0"),
		unit.Name("remote-app/1"),
		unit.Name("!!"),
	}

	service := s.service(c)

	err := service.EnsureUnitsExist(c.Context(), appUUID, units)
	c.Assert(err, tc.ErrorIs, unit.InvalidUnitName)
}

func (s *remoteApplicationServiceSuite) TestIsRelationWithEndpointIdentifiersSuspended(c *tc.C) {
	defer s.setupMocks(c).Finish()

	key := tc.Must1(c, corerelation.NewKeyFromString, "mediawiki:server mysql:database")
	eids := key.EndpointIdentifiers()

	s.modelState.EXPECT().IsRelationWithEndpointIdentifiersSuspended(gomock.Any(), eids[0], eids[1]).Return(true, nil)

	service := s.service(c)

	isValid, err := service.IsCrossModelRelationValidForApplication(c.Context(), key, "mediawiki")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(isValid, tc.IsFalse)
}

func (s *remoteApplicationServiceSuite) TestIsRelationWithEndpointIdentifiersSuspendedIncorrectApplication(c *tc.C) {
	defer s.setupMocks(c).Finish()

	key := tc.Must1(c, corerelation.NewKeyFromString, "mediawiki:server mysql:database")

	service := s.service(c)

	_, err := service.IsCrossModelRelationValidForApplication(c.Context(), key, "wordpress")
	c.Assert(err, tc.ErrorIs, errors.NotValid)
}

func (s *remoteApplicationServiceSuite) TestIsRelationCrossModelRegularRelation(c *tc.C) {
	defer s.setupMocks(c).Finish()

	relationKey, err := corerelation.NewKey([]corerelation.EndpointIdentifier{
		{
			ApplicationName: "app1",
			EndpointName:    "endpoint1",
			Role:            internalcharm.RoleRequirer,
		},
		{
			ApplicationName: "app2",
			EndpointName:    "endpoint2",
			Role:            internalcharm.RoleProvider,
		},
	})
	c.Assert(err, tc.IsNil)

	s.modelState.EXPECT().IsRelationCrossModel(gomock.Any(), relationKey).Return(false, nil)

	service := s.service(c)

	isCMR, err := service.IsRelationCrossModel(c.Context(), relationKey)
	c.Assert(err, tc.IsNil)
	c.Assert(isCMR, tc.Equals, false)
}

func (s *remoteApplicationServiceSuite) TestIsRelationCrossModelCrossModelRelation(c *tc.C) {
	defer s.setupMocks(c).Finish()

	relationKey, err := corerelation.NewKey([]corerelation.EndpointIdentifier{
		{
			ApplicationName: "app1",
			EndpointName:    "endpoint1",
			Role:            internalcharm.RoleRequirer,
		},
		{
			ApplicationName: "app2",
			EndpointName:    "endpoint2",
			Role:            internalcharm.RoleProvider,
		},
	})
	c.Assert(err, tc.IsNil)

	s.modelState.EXPECT().IsRelationCrossModel(gomock.Any(), relationKey).Return(true, nil)

	service := s.service(c)

	isCMR, err := service.IsRelationCrossModel(c.Context(), relationKey)
	c.Assert(err, tc.IsNil)
	c.Assert(isCMR, tc.Equals, true)
}

func (s *remoteApplicationServiceSuite) TestIsRelationCrossModelPeerRelation(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Peer relations are always local, never cross-model
	relationKey, err := corerelation.NewKey([]corerelation.EndpointIdentifier{
		{
			ApplicationName: "app1",
			EndpointName:    "peers",
			Role:            internalcharm.RolePeer,
		},
	})
	c.Assert(err, tc.IsNil)

	// The state layer should not be called for peer relations
	service := s.service(c)

	isCMR, err := service.IsRelationCrossModel(c.Context(), relationKey)
	c.Assert(err, tc.IsNil)
	c.Assert(isCMR, tc.Equals, false)
}

func (s *remoteApplicationServiceSuite) TestIsRelationCrossModelInvalidKey(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Create an invalid relation key
	relationKey := corerelation.Key{}

	service := s.service(c)

	_, err := service.IsRelationCrossModel(c.Context(), relationKey)
	c.Assert(err, tc.ErrorIs, errors.NotValid)
}

func (s *remoteApplicationServiceSuite) TestIsRelationCrossModelStateError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	relationKey, err := corerelation.NewKey([]corerelation.EndpointIdentifier{
		{
			ApplicationName: "app1",
			EndpointName:    "endpoint1",
			Role:            internalcharm.RoleRequirer,
		},
		{
			ApplicationName: "app2",
			EndpointName:    "endpoint2",
			Role:            internalcharm.RoleProvider,
		},
	})
	c.Assert(err, tc.IsNil)

	expectedErr := internalerrors.Errorf("state error")
	s.modelState.EXPECT().IsRelationCrossModel(gomock.Any(), relationKey).Return(false, expectedErr)

	service := s.service(c)

	_, err = service.IsRelationCrossModel(c.Context(), relationKey)
	c.Assert(err, tc.ErrorIs, expectedErr)
}
