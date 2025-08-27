// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/domain/crossmodelrelation"
	"github.com/juju/juju/internal/charm"
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
	relation := charm.Relation{
		Name:      "db",
		Role:      charm.RoleProvider,
		Interface: "db",
		Scope:     charm.ScopeGlobal,
	}
	relationUUID := s.addCharmRelation(c, charmUUID, relation)
	relation2 := charm.Relation{
		Name:      "log",
		Role:      charm.RoleProvider,
		Interface: "log",
		Scope:     charm.ScopeGlobal,
	}
	relationUUID2 := s.addCharmRelation(c, charmUUID, relation2)
	relation3 := charm.Relation{
		Name:      "public",
		Role:      charm.RoleProvider,
		Interface: "public",
		Scope:     charm.ScopeGlobal,
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
