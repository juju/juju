// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/offer"
	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/domain/application/architecture"
	domaincharm "github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/crossmodelrelation"
	crossmodelrelationerrors "github.com/juju/juju/domain/crossmodelrelation/errors"
	domainstatus "github.com/juju/juju/domain/status"
	"github.com/juju/juju/internal/charm"
)

type modelOfferSuite struct {
	baseSuite
}

func TestModelOfferSuite(t *testing.T) {
	tc.Run(t, &modelOfferSuite{})
}

func (s *modelOfferSuite) TestCreateOffer(c *tc.C) {
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

	appName := "test-application"
	appUUID := s.addApplication(c, charmUUID, appName)
	endpointUUID := s.addApplicationEndpoint(c, appUUID, relationUUID)
	endpointUUID2 := s.addApplicationEndpoint(c, appUUID, relationUUID2)

	args := crossmodelrelation.CreateOfferArgs{
		UUID:            tc.Must(c, offer.NewUUID),
		ApplicationName: appName,
		Endpoints:       []string{relation.Name, relation2.Name},
		OfferName:       "test-offer",
	}

	// Act
	err := s.state.CreateOffer(c.Context(), args)

	// Assert
	c.Assert(err, tc.IsNil)
	obtainedOffers := s.readOffers(c)
	c.Check(obtainedOffers, tc.DeepEquals, []nameAndUUID{
		{
			UUID: args.UUID.String(),
			Name: args.OfferName,
		},
	})
	obtainedEndpoints := s.readOfferEndpoints(c)
	c.Check(obtainedEndpoints, tc.SameContents, []offerEndpoint{
		{
			OfferUUID:    args.UUID.String(),
			EndpointUUID: endpointUUID,
		}, {
			OfferUUID:    args.UUID.String(),
			EndpointUUID: endpointUUID2,
		},
	})
}

func (s *modelOfferSuite) TestCreateOfferDyingAplication(c *tc.C) {
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

	appName := "test-application"
	appUUID := s.addApplication(c, charmUUID, appName)
	s.addApplicationEndpoint(c, appUUID, relationUUID)
	s.addApplicationEndpoint(c, appUUID, relationUUID2)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE application SET life_id = 1 WHERE uuid = ?", appUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	args := crossmodelrelation.CreateOfferArgs{
		UUID:            tc.Must(c, offer.NewUUID),
		ApplicationName: appName,
		Endpoints:       []string{relation.Name, relation2.Name},
		OfferName:       "test-offer",
	}

	// Act
	err = s.state.CreateOffer(c.Context(), args)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelOfferSuite) TestCreateOfferDeadAplication(c *tc.C) {
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

	appName := "test-application"
	appUUID := s.addApplication(c, charmUUID, appName)
	s.addApplicationEndpoint(c, appUUID, relationUUID)
	s.addApplicationEndpoint(c, appUUID, relationUUID2)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE application SET life_id = 2 WHERE uuid = ?", appUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	args := crossmodelrelation.CreateOfferArgs{
		UUID:            tc.Must(c, offer.NewUUID),
		ApplicationName: appName,
		Endpoints:       []string{relation.Name, relation2.Name},
		OfferName:       "test-offer",
	}

	// Act
	err = s.state.CreateOffer(c.Context(), args)

	// Assert
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationIsDead)
}

// TestCreateOfferEndpointFail tests that all endpoints are found.
func (s *modelOfferSuite) TestCreateOfferEndpointFail(c *tc.C) {
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

	appName := "test-application"
	appUUID := s.addApplication(c, charmUUID, appName)
	s.addApplicationEndpoint(c, appUUID, relationUUID)

	args := crossmodelrelation.CreateOfferArgs{
		UUID:            tc.Must(c, offer.NewUUID),
		ApplicationName: appName,
		Endpoints:       []string{"fail-me"},
		OfferName:       "test-offer",
	}

	// Act
	err := s.state.CreateOffer(c.Context(), args)

	// Assert
	c.Assert(err, tc.ErrorMatches, `offer "test-offer": "fail-me": endpoint not found`)
	c.Check(s.readOffers(c), tc.HasLen, 0)
	c.Check(s.readOfferEndpoints(c), tc.HasLen, 0)
}

func (s *modelOfferSuite) TestDeleteFailedOffer(c *tc.C) {
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

	appName := "test-application"
	appUUID := s.addApplication(c, charmUUID, appName)
	appEndpointUUD := s.addApplicationEndpoint(c, appUUID, relationUUID)

	offerName := "test-offer"
	offerUUID := s.addOffer(c, offerName, []string{appEndpointUUD})

	// Act
	err := s.state.DeleteFailedOffer(c.Context(), offerUUID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(s.readOffers(c), tc.HasLen, 0)
	c.Check(s.readOfferEndpoints(c), tc.HasLen, 0)
}

func (s *modelOfferSuite) TestGetOfferDetailsFilterMultiplePartialResult(c *tc.C) {
	// Arrange
	expected := s.setupForGetOfferDetails(c)

	// Act
	results, err := s.state.GetOfferDetails(c.Context(), crossmodelrelation.OfferFilter{
		OfferName: expected[0].OfferName,
		// A charm with this metadata description does not exist,
		// expect only 1 result.
		ApplicationDescription: "failme",
	})

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(results, tc.DeepEquals, expected)
}

// TestGetOfferDetailsNoFilter tests that if no filters are provided, all
// offers are returned.
func (s *modelOfferSuite) TestGetOfferDetailsNoFilter(c *tc.C) {
	// Arrange
	expected := s.setupForGetOfferDetails(c)

	// Act
	results, err := s.state.GetOfferDetails(c.Context(), crossmodelrelation.OfferFilter{})

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(results, tc.DeepEquals, expected)
}

func (s *modelOfferSuite) TestGetOfferDetailsFilterNoResult(c *tc.C) {
	// Arrange
	// Create an offer with one endpoint
	charmUUID := s.addCharm(c)
	description := "testing application"
	s.addCharmMetadataWithDescription(c, charmUUID, description)
	relation := charm.Relation{
		Name:      "db",
		Role:      charm.RoleProvider,
		Interface: "db",
		Scope:     charm.ScopeGlobal,
	}
	relationUUID := s.addCharmRelation(c, charmUUID, relation)

	appName := "test-application"
	appUUID := s.addApplication(c, charmUUID, appName)
	s.addApplicationEndpoint(c, appUUID, relationUUID)

	// Add a second relation
	relation2 := charm.Relation{
		Name:      "log",
		Role:      charm.RoleProvider,
		Interface: "log",
		Scope:     charm.ScopeGlobal,
	}
	relationUUID2 := s.addCharmRelation(c, charmUUID, relation2)
	s.addApplicationEndpoint(c, appUUID, relationUUID2)

	// Act
	results, err := s.state.GetOfferDetails(c.Context(), crossmodelrelation.OfferFilter{})

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(results, tc.HasLen, 0)
}

func (s *modelOfferSuite) TestGetOfferDetailsFilterOfferName(c *tc.C) {
	// Arrange
	expected := s.setupForGetOfferDetails(c)

	// Act
	results, err := s.state.GetOfferDetails(c.Context(), crossmodelrelation.OfferFilter{
		OfferName: expected[0].OfferName,
	})

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(results, tc.DeepEquals, expected)
}

func (s *modelOfferSuite) TestGetOfferDetailsFilterOfferUUID(c *tc.C) {
	// Arrange
	expected := s.setupForGetOfferDetails(c)

	// Act
	results, err := s.state.GetOfferDetails(c.Context(), crossmodelrelation.OfferFilter{
		OfferUUIDs: []string{expected[0].OfferUUID},
	})

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(results, tc.DeepEquals, expected)
}

func (s *modelOfferSuite) TestGetOfferDetailsFilterPartialApplicationName(c *tc.C) {
	// Arrange
	expected := s.setupForGetOfferDetails(c)

	// Act
	results, err := s.state.GetOfferDetails(c.Context(), crossmodelrelation.OfferFilter{
		ApplicationName: "test",
	})

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(results, tc.DeepEquals, expected)
}

func (s *modelOfferSuite) TestGetOfferDetailsFilterPartialApplicationDescription(c *tc.C) {
	// Arrange
	expected := s.setupForGetOfferDetails(c)

	// Act
	results, err := s.state.GetOfferDetails(c.Context(), crossmodelrelation.OfferFilter{
		ApplicationDescription: "app",
	})

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(results, tc.DeepEquals, expected)
}

func (s *modelOfferSuite) TestGetOfferDetailsFilterEndpointName(c *tc.C) {
	// Arrange
	expected := s.setupForGetOfferDetails(c)

	// Act
	results, err := s.state.GetOfferDetails(c.Context(), crossmodelrelation.OfferFilter{
		Endpoints: []crossmodelrelation.EndpointFilterTerm{
			{Name: "db-admin"},
		},
	})

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(results, tc.DeepEquals, expected)
}

func (s *modelOfferSuite) TestGetOfferDetailsFilterEndpointRole(c *tc.C) {
	// Arrange
	expected := s.setupForGetOfferDetails(c)

	// Act
	results, err := s.state.GetOfferDetails(c.Context(), crossmodelrelation.OfferFilter{
		Endpoints: []crossmodelrelation.EndpointFilterTerm{
			{Role: domaincharm.RoleProvider},
		},
	})

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(results, tc.DeepEquals, expected)
}

func (s *modelOfferSuite) TestGetOfferDetailsFilterEndpointInterface(c *tc.C) {
	// Arrange
	expected := s.setupForGetOfferDetails(c)
	s.setupOfferWithInterface(c, "testing")

	// Act
	results, err := s.state.GetOfferDetails(c.Context(), crossmodelrelation.OfferFilter{
		Endpoints: []crossmodelrelation.EndpointFilterTerm{
			{Interface: "db"},
		},
	})

	// Assert
	c.Assert(err, tc.IsNil)
	c.Logf("%+v", results)
	c.Assert(results, tc.DeepEquals, expected)
}

func (s *modelOfferSuite) TestGetOfferDetailsFilterMultiEndpoint(c *tc.C) {
	// Arrange
	expected := s.setupForGetOfferDetails(c)
	expected = append(expected, s.setupOfferWithInterface(c, "db")...)
	c.Check(expected, tc.HasLen, 2)

	// Act
	results, err := s.state.GetOfferDetails(c.Context(), crossmodelrelation.OfferFilter{
		Endpoints: []crossmodelrelation.EndpointFilterTerm{
			{Interface: "db"},
		},
	})

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(results, tc.SameContents, expected)
}

func (s *modelOfferSuite) TestGetOfferUUID(c *tc.C) {
	// Arrange
	// Create an offer with one endpoint
	charmUUID := s.addCharm(c)
	description := "testing application"
	s.addCharmMetadataWithDescription(c, charmUUID, description)
	relation := charm.Relation{
		Name:      "db-admin",
		Role:      charm.RoleProvider,
		Interface: "db",
		Scope:     charm.ScopeGlobal,
	}
	relationUUID := s.addCharmRelation(c, charmUUID, relation)

	appName := "test-application"
	appUUID := s.addApplication(c, charmUUID, appName)
	appEndpointUUID := s.addApplicationEndpoint(c, appUUID, relationUUID)
	offerName := "test-offer"
	offerUUID := s.addOffer(c, offerName, []string{appEndpointUUID})

	// Act
	obtainedOfferUUID, err := s.state.GetOfferUUID(c.Context(), offerName)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtainedOfferUUID, tc.Equals, offerUUID.String())
}

func (s *modelOfferSuite) TestGetOfferUUIDNotFound(c *tc.C) {
	// Act
	offerUUID, err := s.state.GetOfferUUID(c.Context(), "failure")

	// Assert
	c.Assert(err, tc.ErrorIs, crossmodelrelationerrors.OfferNotFound)
	c.Assert(offerUUID, tc.Equals, "")
}

func (s *modelOfferSuite) TestGetConsumeDetails(c *tc.C) {
	// Arrange
	// Create an offer with two endpoints
	charmUUID := s.addCharm(c)
	description := "testing application"
	s.addCharmMetadataWithDescription(c, charmUUID, description)
	relation := charm.Relation{
		Name:      "db-admin",
		Role:      charm.RoleProvider,
		Interface: "db",
		Scope:     charm.ScopeGlobal,
		Limit:     4,
	}
	relationUUID := s.addCharmRelation(c, charmUUID, relation)
	relationTwo := charm.Relation{
		Name:      "db",
		Role:      charm.RoleProvider,
		Interface: "other",
		Scope:     charm.ScopeGlobal,
	}
	relationTwoUUID := s.addCharmRelation(c, charmUUID, relationTwo)

	appName := "test-application"
	appUUID := s.addApplication(c, charmUUID, appName)
	appEndpointUUID := s.addApplicationEndpoint(c, appUUID, relationUUID)
	appEndpointTwoUUID := s.addApplicationEndpoint(c, appUUID, relationTwoUUID)
	offerName := "test-offer"
	offerUUID := s.addOffer(c, offerName, []string{appEndpointUUID, appEndpointTwoUUID})

	// Act
	obtained, err := s.state.GetConsumeDetails(c.Context(), offerName)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtained.OfferUUID, tc.Equals, offerUUID.String())
	c.Check(obtained.Endpoints, tc.SameContents, []crossmodelrelation.OfferEndpoint{
		{
			Name:      relation.Name,
			Role:      domaincharm.RoleProvider,
			Interface: relation.Interface,
			Limit:     4,
		}, {
			Name:      relationTwo.Name,
			Role:      domaincharm.RoleProvider,
			Interface: relationTwo.Interface,
		},
	})
}

func (s *modelOfferSuite) TestGetConsumeDetailsNotFound(c *tc.C) {
	// Act
	_, err := s.state.GetConsumeDetails(c.Context(), "failure")

	// Assert
	c.Assert(err, tc.ErrorIs, crossmodelrelationerrors.OfferNotFound)
}

func (s *modelOfferSuite) TestGetOfferUUIDByRelationUUID(c *tc.C) {
	relationUUID, offerUUID := s.setupOfferConnection(c)
	obtainedOfferUUID, err := s.state.GetOfferUUIDByRelationUUID(c.Context(), relationUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtainedOfferUUID, tc.Equals, offerUUID)
}

func (s *modelOfferSuite) TestGetOfferUUIDByRelationUUIDNotFound(c *tc.C) {
	_, err := s.state.GetOfferUUIDByRelationUUID(c.Context(), tc.Must(c, corerelation.NewUUID).String())
	c.Assert(err, tc.ErrorIs, crossmodelrelationerrors.OfferNotFound)
}

func (s *modelOfferSuite) TestGetOfferConnections(c *tc.C) {
	// Arrange
	// One relation between a real and synthetic application with
	// relation ingress networks.
	cmrCharmUUID := s.addCMRCharm(c)
	provider := charm.Relation{
		Name:      "db-admin-p",
		Role:      charm.RoleProvider,
		Interface: "db",
		Scope:     charm.ScopeGlobal,
	}
	cmrCharmRelationUUID := s.addCharmRelation(c, cmrCharmUUID, provider)
	cmrAppName := "cmr-test-application"
	cmrAppUUID := s.addApplication(c, cmrCharmUUID, cmrAppName)
	cmrAppEndpointUUD := s.addApplicationEndpoint(c, cmrAppUUID, cmrCharmRelationUUID)

	charmUUID := s.addCharm(c)
	requirer := charm.Relation{
		Name:      "db-admin-r",
		Role:      charm.RoleRequirer,
		Interface: "db",
		Scope:     charm.ScopeGlobal,
	}
	charmRelationUUID := s.addCharmRelation(c, charmUUID, requirer)
	appName := "test-application"
	appUUID := s.addApplication(c, charmUUID, appName)
	appEndpointUUD := s.addApplicationEndpoint(c, appUUID, charmRelationUUID)
	offerUUID := s.addOffer(c, appName, []string{appEndpointUUD})

	relationUUID := s.addOfferConnection(c, offerUUID, domainstatus.RelationStatusTypeJoined)
	s.addRelationEndpoint(c, relationUUID, cmrAppEndpointUUD)
	s.addRelationEndpoint(c, relationUUID, appEndpointUUD)
	s.addRelationIngressNetwork(c, relationUUID, "203.0.113.42/24")
	s.addRelationIngressNetwork(c, relationUUID, "203.0.113.8/24")

	// Act
	obtained, err := s.state.GetOfferConnections(c.Context(), []string{offerUUID.String()})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(len(obtained), tc.Equals, 1)
	obtainedConnections, ok := obtained[offerUUID.String()]
	c.Assert(ok, tc.Equals, true)
	c.Assert(obtainedConnections, tc.HasLen, 1)
	mc := tc.NewMultiChecker()
	mc.AddExpr("_.IngressSubnets", tc.Ignore)
	mc.AddExpr("_.Since", tc.Ignore)
	c.Check(obtainedConnections[0], mc, crossmodelrelation.OfferConnection{
		Username:   "bob",
		RelationId: 0,
		Endpoint:   "db-admin-r",
		Status:     status.Joined,
	})
	if !c.Check(obtainedConnections[0].IngressSubnets, tc.SameContents, []string{"203.0.113.8/24", "203.0.113.42/24"}) {
		s.DumpTable(c, "relation_network_ingress")
	}
}

func (s *modelOfferSuite) TestGetOfferConnectionsNoIngressNetworks(c *tc.C) {
	// Arrange
	// One relation between a real and synthetic application with
	// no relation ingress networks.
	cmrCharmUUID := s.addCMRCharm(c)
	provider := charm.Relation{
		Name:      "db-admin-p",
		Role:      charm.RoleProvider,
		Interface: "db",
		Scope:     charm.ScopeGlobal,
	}
	cmrCharmRelationUUID := s.addCharmRelation(c, cmrCharmUUID, provider)
	cmrAppName := "cmr-test-application"
	cmrAppUUID := s.addApplication(c, cmrCharmUUID, cmrAppName)
	cmrAppEndpointUUD := s.addApplicationEndpoint(c, cmrAppUUID, cmrCharmRelationUUID)

	charmUUID := s.addCharm(c)
	requirer := charm.Relation{
		Name:      "db-admin-r",
		Role:      charm.RoleRequirer,
		Interface: "db",
		Scope:     charm.ScopeGlobal,
	}
	charmRelationUUID := s.addCharmRelation(c, charmUUID, requirer)
	appName := "test-application"
	appUUID := s.addApplication(c, charmUUID, appName)
	appEndpointUUD := s.addApplicationEndpoint(c, appUUID, charmRelationUUID)
	offerUUID := s.addOffer(c, appName, []string{appEndpointUUD})

	relationUUID := s.addOfferConnection(c, offerUUID, domainstatus.RelationStatusTypeJoined)
	s.addRelationEndpoint(c, relationUUID, cmrAppEndpointUUD)
	s.addRelationEndpoint(c, relationUUID, appEndpointUUD)

	// Act
	obtained, err := s.state.GetOfferConnections(c.Context(), []string{offerUUID.String()})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(len(obtained), tc.Equals, 1)
	obtainedConnections, ok := obtained[offerUUID.String()]
	c.Assert(ok, tc.Equals, true)
	c.Assert(obtainedConnections, tc.HasLen, 1)
	mc := tc.NewMultiChecker()
	mc.AddExpr("_.Since", tc.Ignore)
	c.Check(obtainedConnections[0], mc, crossmodelrelation.OfferConnection{
		Username:   "bob",
		RelationId: 0,
		Endpoint:   "db-admin-r",
		Status:     status.Joined,
	})
}

func (s *modelOfferSuite) TestGetOfferConnectionsNoConnections(c *tc.C) {
	// Arrange
	// One application with an offer, no relations, nor connections.
	charmUUID := s.addCharm(c)
	requirer := charm.Relation{
		Name:      "db-admin-r",
		Role:      charm.RoleRequirer,
		Interface: "db",
		Scope:     charm.ScopeGlobal,
	}
	charmRelationUUID := s.addCharmRelation(c, charmUUID, requirer)
	appName := "test-application"
	appUUID := s.addApplication(c, charmUUID, appName)
	appEndpointUUD := s.addApplicationEndpoint(c, appUUID, charmRelationUUID)
	offerUUID := s.addOffer(c, appName, []string{appEndpointUUD})

	// Act
	obtained, err := s.state.GetOfferConnections(c.Context(), []string{offerUUID.String()})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtained, tc.IsNil)
}

func (s *modelOfferSuite) TestTransformToOfferConnectionMap(c *tc.C) {
	// Arrange
	// 2 offers, 1 with 2 connections, 1 with 1 connection.
	// 3 relations, 2 on one offer.
	// Ingress networks for 2 of the relations.
	offerConnections := []offerConnectionDetail{
		{
			OfferUUID:    "offerOne",
			EndpointName: "endpointOne",
			RelationUUID: "relationOne",
			RelationID:   1,
			Status:       "joined",
			Username:     "bob",
		}, {
			OfferUUID:    "offerOne",
			EndpointName: "endpointTwo",
			RelationUUID: "relationTwo",
			RelationID:   2,
			Status:       "joined",
			Username:     "bob",
		}, {
			OfferUUID:    "offerTwo",
			EndpointName: "endpointThree",
			RelationUUID: "relationThree",
			RelationID:   3,
			Status:       "joined",
			Username:     "bob",
		},
	}
	ingressNetworks := []relationNetworkIngress{
		{
			RelationUUID: "relationOne",
			CIDR:         "203.0.113.1/24",
		}, {
			RelationUUID: "relationThree",
			CIDR:         "203.0.113.3/24",
		}, {
			RelationUUID: "relationThree",
			CIDR:         "203.0.113.33/24",
		},
	}

	// Act
	obtained := transformToOfferConnectionMap(offerConnections, ingressNetworks)

	// Assert
	c.Check(obtained, tc.HasLen, 2)
	offerOne, ok := obtained["offerOne"]
	c.Check(ok, tc.Equals, true)

	mc := tc.NewMultiChecker()
	mc.AddExpr("_.Since", tc.Ignore)
	c.Check(offerOne, tc.UnorderedMatch[[]crossmodelrelation.OfferConnection](mc), []crossmodelrelation.OfferConnection{
		{
			Endpoint:   "endpointTwo",
			RelationId: 2,
			Status:     status.Joined,
			Username:   "bob",
		}, {
			Endpoint:       "endpointOne",
			RelationId:     1,
			Status:         status.Joined,
			Username:       "bob",
			IngressSubnets: []string{"203.0.113.1/24"},
		},
	})

	offerTwo, ok := obtained["offerTwo"]
	c.Check(ok, tc.Equals, true)
	mc.AddExpr("_.IngressSubnets", tc.SameContents, []string{"203.0.113.3/24", "203.0.113.33/24"})
	c.Check(offerTwo, tc.UnorderedMatch[[]crossmodelrelation.OfferConnection](mc), []crossmodelrelation.OfferConnection{
		{
			Endpoint:   "endpointThree",
			RelationId: 3,
			Status:     status.Joined,
			Username:   "bob",
		},
	})
}

// setupForGetOfferDetails
func (s *modelOfferSuite) setupForGetOfferDetails(c *tc.C) []*crossmodelrelation.OfferDetail {
	// Create an offer with one endpoint
	charmUUID := s.addCharm(c)
	description := "testing application"
	s.addCharmMetadataWithDescription(c, charmUUID, description)
	relation := charm.Relation{
		Name:      "db-admin",
		Role:      charm.RoleProvider,
		Interface: "db",
		Scope:     charm.ScopeGlobal,
	}
	relationUUID := s.addCharmRelation(c, charmUUID, relation)

	appName := "test-application"
	appUUID := s.addApplication(c, charmUUID, appName)
	appEndpointUUD1 := s.addApplicationEndpoint(c, appUUID, relationUUID)

	// Add a second relation
	relation2 := charm.Relation{
		Name:      "log",
		Role:      charm.RoleRequirer,
		Interface: "log",
		Scope:     charm.ScopeGlobal,
	}
	relationUUID2 := s.addCharmRelation(c, charmUUID, relation2)
	s.addApplicationEndpoint(c, appUUID, relationUUID2)

	offerName := "test-offer"
	offerUUID := s.addOffer(c, offerName, []string{appEndpointUUD1})

	s.addOfferConnection(c, offerUUID, domainstatus.RelationStatusTypeJoined)
	s.addOfferConnection(c, offerUUID, domainstatus.RelationStatusTypeJoining)

	return []*crossmodelrelation.OfferDetail{
		{
			OfferUUID:              offerUUID.String(),
			OfferName:              offerName,
			ApplicationName:        appName,
			ApplicationDescription: description,
			CharmLocator: domaincharm.CharmLocator{
				Name:         charmUUID.String(),
				Revision:     42,
				Source:       domaincharm.CharmHubSource,
				Architecture: architecture.AMD64,
			},
			Endpoints: []crossmodelrelation.OfferEndpoint{
				{
					Name:      relation.Name,
					Role:      domaincharm.RoleProvider,
					Interface: relation.Interface,
				},
			},
			TotalConnections:       2,
			TotalActiveConnections: 1,
		},
	}
}

// setupForGetOfferDetails
func (s *modelOfferSuite) setupOfferWithInterface(c *tc.C, interfaceName string) []*crossmodelrelation.OfferDetail {
	// Create an offer with one endpoint
	charmUUID := s.addCharm(c)
	description := "second testing application"
	s.addCharmMetadataWithDescription(c, charmUUID, description)
	relation := charm.Relation{
		Name:      "second-admin",
		Role:      charm.RoleProvider,
		Interface: interfaceName,
		Scope:     charm.ScopeGlobal,
	}
	relationUUID := s.addCharmRelation(c, charmUUID, relation)

	appName := "second-application"
	appUUID := s.addApplication(c, charmUUID, appName)
	appEndpointUUD := s.addApplicationEndpoint(c, appUUID, relationUUID)

	offerUUID := s.addOffer(c, appName, []string{appEndpointUUD})

	return []*crossmodelrelation.OfferDetail{
		{
			OfferUUID:              offerUUID.String(),
			OfferName:              appName,
			ApplicationName:        appName,
			ApplicationDescription: description,
			CharmLocator: domaincharm.CharmLocator{
				Name:         charmUUID.String(),
				Revision:     42,
				Source:       domaincharm.CharmHubSource,
				Architecture: architecture.AMD64,
			},
			Endpoints: []crossmodelrelation.OfferEndpoint{
				{
					Name:      relation.Name,
					Role:      domaincharm.RoleProvider,
					Interface: relation.Interface,
				},
			},
		},
	}
}

func (s *modelOfferSuite) setupOfferConnection(c *tc.C) (string, string) {
	// Create an offer with one endpoint
	charmUUID := s.addCharm(c)
	description := "testing application"
	s.addCharmMetadataWithDescription(c, charmUUID, description)
	relation := charm.Relation{
		Name:      "db-admin",
		Role:      charm.RoleProvider,
		Interface: "db",
		Scope:     charm.ScopeGlobal,
	}
	relationUUID := s.addCharmRelation(c, charmUUID, relation)

	appName := "test-application"
	appUUID := s.addApplication(c, charmUUID, appName)
	appEndpointUUD1 := s.addApplicationEndpoint(c, appUUID, relationUUID)

	// Add a second relation
	relation2 := charm.Relation{
		Name:      "log",
		Role:      charm.RoleRequirer,
		Interface: "log",
		Scope:     charm.ScopeGlobal,
	}
	relationUUID2 := s.addCharmRelation(c, charmUUID, relation2)
	s.addApplicationEndpoint(c, appUUID, relationUUID2)

	offerName := "test-offer"
	offerUUID := s.addOffer(c, offerName, []string{appEndpointUUD1})

	crossModelRelUUID := s.addOfferConnection(c, offerUUID, domainstatus.RelationStatusTypeJoined)
	s.addOfferConnection(c, offerUUID, domainstatus.RelationStatusTypeJoining)

	return crossModelRelUUID, offerUUID.String()
}
