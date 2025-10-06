// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package service

import (
	"context"
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/errors"
	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/crossmodelrelation"
	"github.com/juju/juju/domain/life"
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

	offerUUID := tc.Must(c, uuid.NewUUID).String()
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
			OfferUUID: offerUUID,
		},
		OffererControllerUUID: offererControllerUUID,
		OffererModelUUID:      offererModelUUID,
		EncodedMacaroon:       tc.Must(c, macaroon.MarshalJSON),
	})
}

func (s *remoteApplicationServiceSuite) TestAddRemoteApplicationOffererNoEndpoints(c *tc.C) {
	defer s.setupMocks(c).Finish()

	offerUUID := tc.Must(c, uuid.NewUUID).String()

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

	offerUUID := tc.Must(c, uuid.NewUUID).String()
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

	offerUUID := tc.Must(c, uuid.NewUUID).String()
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

	offerUUID := tc.Must(c, uuid.NewUUID).String()
	relationUUID := tc.Must(c, uuid.NewUUID).String()

	syntheticCharm := charm.Charm{
		Metadata: charm.Metadata{
			Name:        "remote-test-app",
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
		ReferenceName: "remote-test-app",
		Source:        charm.CMRSource,
	}

	var received crossmodelrelation.AddRemoteApplicationConsumerArgs
	s.modelState.EXPECT().AddRemoteApplicationConsumer(gomock.Any(), "remote-test-app", gomock.Any()).DoAndReturn(func(_ context.Context, _ string, args crossmodelrelation.AddRemoteApplicationConsumerArgs) error {
		received = args
		return nil
	})

	service := s.service(c)

	err := service.AddRemoteApplicationConsumer(c.Context(), AddRemoteApplicationConsumerArgs{
		RemoteApplicationName: "test-app",
		OfferUUID:             offerUUID,
		RelationUUID:          relationUUID,
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
			Charm:     syntheticCharm,
			OfferUUID: offerUUID,
		},
		RelationUUID: relationUUID,
	})
}

func (s *remoteApplicationServiceSuite) TestAddRemoteApplicationConsumerInvalidApplicationName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	service := s.service(c)

	err := service.AddRemoteApplicationConsumer(c.Context(), AddRemoteApplicationConsumerArgs{
		RemoteApplicationName: "!foo",
	})
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNameNotValid)
}

func (s *remoteApplicationServiceSuite) TestAddRemoteApplicationConsumerInvalidOfferUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	service := s.service(c)

	err := service.AddRemoteApplicationConsumer(c.Context(), AddRemoteApplicationConsumerArgs{
		RemoteApplicationName: "foo",
		OfferUUID:             "!!",
	})
	c.Assert(err, tc.ErrorIs, errors.NotValid)
}

func (s *remoteApplicationServiceSuite) TestAddRemoteApplicationConsumerInvalidRelationUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	offerUUID := tc.Must(c, uuid.NewUUID).String()

	service := s.service(c)

	err := service.AddRemoteApplicationConsumer(c.Context(), AddRemoteApplicationConsumerArgs{
		RemoteApplicationName: "foo",
		OfferUUID:             offerUUID,
		RelationUUID:          "!!",
	})
	c.Assert(err, tc.ErrorIs, errors.NotValid)
}

func (s *remoteApplicationServiceSuite) TestAddRemoteApplicationConsumerNoEndpoints(c *tc.C) {
	defer s.setupMocks(c).Finish()

	offerUUID := tc.Must(c, uuid.NewUUID).String()
	relationUUID := tc.Must(c, uuid.NewUUID).String()

	service := s.service(c)

	err := service.AddRemoteApplicationConsumer(c.Context(), AddRemoteApplicationConsumerArgs{
		RemoteApplicationName: "foo",
		OfferUUID:             offerUUID,
		RelationUUID:          relationUUID,
		Endpoints:             []charm.Relation{},
	})
	c.Assert(err, tc.ErrorIs, errors.NotValid)
}

func (s *remoteApplicationServiceSuite) TestAddRemoteApplicationConsumerInvalidEndpointScope(c *tc.C) {
	defer s.setupMocks(c).Finish()

	offerUUID := tc.Must(c, uuid.NewUUID).String()
	relationUUID := tc.Must(c, uuid.NewUUID).String()

	service := s.service(c)

	err := service.AddRemoteApplicationConsumer(c.Context(), AddRemoteApplicationConsumerArgs{
		RemoteApplicationName: "foo",
		OfferUUID:             offerUUID,
		RelationUUID:          relationUUID,
		Endpoints: []charm.Relation{{
			Name:      "db",
			Role:      charm.RoleProvider,
			Interface: "database",
			Limit:     1,
			Scope:     charm.ScopeContainer,
		}},
	})
	c.Assert(err, tc.ErrorIs, errors.NotValid)
}

func (s *remoteApplicationServiceSuite) TestAddRemoteApplicationConsumerStateError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	offerUUID := tc.Must(c, uuid.NewUUID).String()
	relationUUID := tc.Must(c, uuid.NewUUID).String()

	s.modelState.EXPECT().AddRemoteApplicationConsumer(gomock.Any(), "remote-foo", gomock.Any()).Return(internalerrors.Errorf("database error"))

	service := s.service(c)

	err := service.AddRemoteApplicationConsumer(c.Context(), AddRemoteApplicationConsumerArgs{
		RemoteApplicationName: "foo",
		OfferUUID:             offerUUID,
		RelationUUID:          relationUUID,
		Endpoints: []charm.Relation{{
			Name:      "db",
			Role:      charm.RoleProvider,
			Interface: "database",
			Limit:     1,
			Scope:     charm.ScopeGlobal,
		}},
	})
	c.Assert(err, tc.ErrorMatches, "inserting remote application consumer: database error")
}

func (s *remoteApplicationServiceSuite) TestAddRemoteApplicationConsumerMixedEndpoints(c *tc.C) {
	defer s.setupMocks(c).Finish()

	offerUUID := tc.Must(c, uuid.NewUUID).String()
	relationUUID := tc.Must(c, uuid.NewUUID).String()

	syntheticCharm := charm.Charm{
		Metadata: charm.Metadata{
			Name:        "remote-mixed-app",
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
		ReferenceName: "remote-mixed-app",
		Source:        charm.CMRSource,
	}

	var received crossmodelrelation.AddRemoteApplicationConsumerArgs
	s.modelState.EXPECT().AddRemoteApplicationConsumer(gomock.Any(), "remote-mixed-app", gomock.Any()).DoAndReturn(func(_ context.Context, _ string, args crossmodelrelation.AddRemoteApplicationConsumerArgs) error {
		received = args
		return nil
	})

	service := s.service(c)

	err := service.AddRemoteApplicationConsumer(c.Context(), AddRemoteApplicationConsumerArgs{
		RemoteApplicationName: "mixed-app",
		OfferUUID:             offerUUID,
		RelationUUID:          relationUUID,
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
			Charm:     syntheticCharm,
			OfferUUID: offerUUID,
		},
		RelationUUID: relationUUID,
	})
}
