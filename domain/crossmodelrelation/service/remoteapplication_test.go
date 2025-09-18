// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package service

import (
	"testing"

	"github.com/juju/juju/core/errors"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/crossmodelrelation"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"
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
	externalControllerUUID := ptr(tc.Must(c, uuid.NewUUID).String())
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
					Scope:     charm.ScopeContainer,
				},
			},
			Peers: map[string]charm.Relation{},
		},
		ReferenceName: "foo",
		Source:        charm.CMRSource,
	}
	s.modelDBState.EXPECT().AddRemoteApplicationOfferer(gomock.Any(), "foo", crossmodelrelation.AddRemoteApplicationOffererArgs{
		Charm:                  syntheticCharm,
		OfferUUID:              offerUUID,
		ExternalControllerUUID: externalControllerUUID,
		EncodedMacaroon:        tc.Must(c, macaroon.MarshalJSON),
	}).Return(nil)

	service := s.service(c)

	err := service.AddRemoteApplicationOfferer(c.Context(), "foo", AddRemoteApplicationOffererArgs{
		OfferUUID:              offerUUID,
		ExternalControllerUUID: externalControllerUUID,
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
			Scope:     charm.ScopeContainer,
		}},
		Macaroon: macaroon,
	})
	c.Assert(err, tc.ErrorIsNil)
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
