// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/domain/application/architecture"
	domaincharm "github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/crossmodelrelation"
	crossmodelrelationerrors "github.com/juju/juju/domain/crossmodelrelation/errors"
	"github.com/juju/juju/domain/crossmodelrelation/internal"
	"github.com/juju/juju/internal/charm"
	internaluuid "github.com/juju/juju/internal/uuid"
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

	args := internal.CreateOfferArgs{
		UUID:            internaluuid.MustNewUUID(),
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

	args := internal.CreateOfferArgs{
		UUID:            internaluuid.MustNewUUID(),
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
	formattedUUID, _ := internaluuid.UUIDFromString(offerUUID)

	// Act
	err := s.state.DeleteFailedOffer(c.Context(), formattedUUID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(s.readOffers(c), tc.HasLen, 0)
	c.Check(s.readOfferEndpoints(c), tc.HasLen, 0)
}

func (s *modelOfferSuite) TestUpdateOffer(c *tc.C) {
	// Arrange:
	// Create an offer with one endpoint
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

	// Add a second relation
	relation2 := charm.Relation{
		Name:      "log",
		Role:      charm.RoleProvider,
		Interface: "log",
		Scope:     charm.ScopeGlobal,
	}
	relationUUID2 := s.addCharmRelation(c, charmUUID, relation2)
	appEndpointUUD2 := s.addApplicationEndpoint(c, appUUID, relationUUID2)

	// Act
	err := s.state.UpdateOffer(c.Context(), offerName, []string{relation.Name, relation2.Name})

	// Assert
	c.Assert(err, tc.IsNil)
	obtainedOffers := s.readOffers(c)
	c.Check(obtainedOffers, tc.DeepEquals, []nameAndUUID{
		{
			UUID: offerUUID,
			Name: offerName,
		},
	})
	obtainedEndpoints := s.readOfferEndpoints(c)
	c.Check(obtainedEndpoints, tc.SameContents, []offerEndpoint{
		{
			OfferUUID:    offerUUID,
			EndpointUUID: appEndpointUUD,
		}, {
			OfferUUID:    offerUUID,
			EndpointUUID: appEndpointUUD2,
		},
	})
}

func (s *modelOfferSuite) TestUpdateOfferDoesNotExist(c *tc.C) {
	// Act
	err := s.state.UpdateOffer(c.Context(), "offername", []string{"db"})

	// Assert
	c.Assert(err, tc.ErrorMatches, `"offername": offer not found`)
}

func (s *modelOfferSuite) TestUpdateOfferEndpointFail(c *tc.C) {
	// Arrange:
	// Create an offer with one endpoint
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
	s.addOffer(c, offerName, []string{appEndpointUUD})

	// Act
	err := s.state.UpdateOffer(c.Context(), offerName, []string{"failme"})

	// Assert
	c.Assert(err, tc.ErrorMatches, `offer "test-offer": "failme": endpoint not found`)

}

func (s *modelOfferSuite) TestGetOfferDetailsFilterMultiplePartialResult(c *tc.C) {
	// Arrange
	expected := s.setupForGetOfferDetails(c)

	// Act
	results, err := s.state.GetOfferDetails(c.Context(), internal.OfferFilter{
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
	results, err := s.state.GetOfferDetails(c.Context(), internal.OfferFilter{})

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
	results, err := s.state.GetOfferDetails(c.Context(), internal.OfferFilter{})

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(results, tc.HasLen, 0)
}

func (s *modelOfferSuite) TestGetOfferDetailsFilterOfferName(c *tc.C) {
	// Arrange
	expected := s.setupForGetOfferDetails(c)

	// Act
	results, err := s.state.GetOfferDetails(c.Context(), internal.OfferFilter{
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
	results, err := s.state.GetOfferDetails(c.Context(), internal.OfferFilter{
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
	results, err := s.state.GetOfferDetails(c.Context(), internal.OfferFilter{
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
	results, err := s.state.GetOfferDetails(c.Context(), internal.OfferFilter{
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
	results, err := s.state.GetOfferDetails(c.Context(), internal.OfferFilter{
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
	results, err := s.state.GetOfferDetails(c.Context(), internal.OfferFilter{
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
	results, err := s.state.GetOfferDetails(c.Context(), internal.OfferFilter{
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
	results, err := s.state.GetOfferDetails(c.Context(), internal.OfferFilter{
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
	c.Assert(obtainedOfferUUID, tc.Equals, offerUUID)
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

	return []*crossmodelrelation.OfferDetail{
		{
			OfferUUID:              offerUUID,
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
			OfferUUID:              offerUUID,
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

func (s *modelOfferSuite) readOffers(c *tc.C) []nameAndUUID {
	rows, err := s.DB().QueryContext(c.Context(), `SELECT * FROM offer`)
	c.Assert(err, tc.IsNil)
	defer func() { _ = rows.Close() }()
	foundOffers := []nameAndUUID{}
	for rows.Next() {
		var found nameAndUUID
		err = rows.Scan(&found.UUID, &found.Name)
		c.Assert(err, tc.IsNil)
		foundOffers = append(foundOffers, found)
	}
	return foundOffers
}

func (s *modelOfferSuite) readOfferEndpoints(c *tc.C) []offerEndpoint {
	rows, err := s.DB().QueryContext(c.Context(), `SELECT * FROM offer_endpoint`)
	c.Assert(err, tc.IsNil)
	defer func() { _ = rows.Close() }()
	foundOfferEndpoints := []offerEndpoint{}
	for rows.Next() {
		var found offerEndpoint
		err = rows.Scan(&found.OfferUUID, &found.EndpointUUID)
		c.Assert(err, tc.IsNil)
		foundOfferEndpoints = append(foundOfferEndpoints, found)
	}
	return foundOfferEndpoints
}
