// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/offer"
	relationtesting "github.com/juju/juju/core/relation/testing"
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
	appEndpointUUD := s.addApplicationEndpoint(c, appUUID, relationUUID)
	offerName := "test-offer"
	offerUUID := s.addOffer(c, offerName, []string{appEndpointUUD})

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

func (s *modelOfferSuite) TestGetOfferUUIDByRelationUUID(c *tc.C) {
	relationUUID, offerUUID := s.setupOfferConnection(c)
	obtainedOfferUUID, err := s.state.GetOfferUUIDByRelationUUID(c.Context(), relationUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtainedOfferUUID, tc.Equals, offerUUID)
}

func (s *modelOfferSuite) TestGetOfferUUIDByRelationUUIDNotFound(c *tc.C) {
	_, err := s.state.GetOfferUUIDByRelationUUID(c.Context(), relationtesting.GenRelationUUID(c).String())
	c.Assert(err, tc.ErrorIs, crossmodelrelationerrors.OfferNotFound)
}
