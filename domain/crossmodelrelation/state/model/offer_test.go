// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"testing"

	"github.com/canonical/sqlair"
	"github.com/juju/tc"

	coreapplication "github.com/juju/juju/core/application"
	coreapplicationtesting "github.com/juju/juju/core/application/testing"
	corecharm "github.com/juju/juju/core/charm"
	corecharmtesting "github.com/juju/juju/core/charm/testing"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/crossmodelrelation/internal"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	internaluuid "github.com/juju/juju/internal/uuid"
)

type modelOfferSuite struct {
	schematesting.ModelSuite
	state *State
}

func TestModelOfferSuite(t *testing.T) {
	tc.Run(t, &modelOfferSuite{})
}

func (s *modelOfferSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)
	s.state = NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
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

// TestCreateEndpointFail tests that all endpoints are found.
func (s *modelOfferSuite) TestCreateEndpointFail(c *tc.C) {
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
	s.addApplicationEndpoint(c, appUUID, relationUUID)

	args := internal.CreateOfferArgs{
		UUID:            internaluuid.MustNewUUID(),
		ApplicationName: appName,
		Endpoints:       []string{relation.Name},
		OfferName:       "test-offer",
	}

	err := s.state.CreateOffer(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(s.readOffers(c), tc.HasLen, 1)
	c.Check(s.readOfferEndpoints(c), tc.HasLen, 1)

	// Act
	err = s.state.DeleteFailedOffer(c.Context(), args.UUID)

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
	endpointUUID := s.addApplicationEndpoint(c, appUUID, relationUUID)

	args := internal.CreateOfferArgs{
		UUID:            internaluuid.MustNewUUID(),
		ApplicationName: appName,
		Endpoints:       []string{relation.Name},
		OfferName:       "test-offer",
	}

	err := s.state.CreateOffer(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)

	// Add a second relation
	relation2 := charm.Relation{
		Name:      "log",
		Role:      charm.RoleProvider,
		Interface: "log",
		Scope:     charm.ScopeGlobal,
	}
	relationUUID2 := s.addCharmRelation(c, charmUUID, relation2)
	endpointUUID2 := s.addApplicationEndpoint(c, appUUID, relationUUID2)

	args.Endpoints = append(args.Endpoints, relation2.Name)

	// Act
	err = s.state.UpdateOffer(c.Context(), args.OfferName, args.Endpoints)

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
	s.addApplicationEndpoint(c, appUUID, relationUUID)

	args := internal.CreateOfferArgs{
		UUID:            internaluuid.MustNewUUID(),
		ApplicationName: appName,
		Endpoints:       []string{relation.Name},
		OfferName:       "test-offer",
	}

	err := s.state.CreateOffer(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)

	// Act
	err = s.state.UpdateOffer(c.Context(), args.OfferName, []string{"failme"})

	// Assert
	c.Assert(err, tc.ErrorMatches, `offer "test-offer": "failme": endpoint not found`)

}

// Txn executes a transactional function within a database context,
// ensuring proper error handling and assertion.
func (s *modelOfferSuite) Txn(c *tc.C, fn func(ctx context.Context, tx *sqlair.TX) error) error {
	db, err := s.state.DB(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	return db.Txn(c.Context(), fn)
}

// query executes a given SQL query with optional arguments within a
// transactional context using the test database.
func (s *modelOfferSuite) query(c *tc.C, query string, args ...any) {

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, query, args...)
		if err != nil {
			return errors.Errorf("%w: query: %s (args: %s)", err, query, args)
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) populate DB: %v",
		errors.ErrorStack(err)))
}

// addApplication adds a new application to the database with the specified
// charm UUID and application name. It returns the application UUID.
func (s *modelOfferSuite) addApplication(c *tc.C, charmUUID corecharm.ID, appName string) coreapplication.ID {
	appUUID := coreapplicationtesting.GenApplicationUUID(c)
	s.query(c, `
INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid) 
VALUES (?, ?, ?, ?, ?)
`, appUUID, appName, 0 /* alive */, charmUUID.String(), network.AlphaSpaceId)
	return appUUID
}

// addApplicationEndpoint inserts a new application endpoint into the database
// with the specified UUIDs. Returns the endpoint uuid.
func (s *modelOfferSuite) addApplicationEndpoint(c *tc.C, applicationUUID coreapplication.ID,
	charmRelationUUID string) string {
	applicationEndpointUUID := internaluuid.MustNewUUID().String()
	s.query(c, `
INSERT INTO application_endpoint (uuid, application_uuid, charm_relation_uuid,space_uuid)
VALUES (?, ?, ?, ?)
`, applicationEndpointUUID, applicationUUID, charmRelationUUID, network.AlphaSpaceId)
	return applicationEndpointUUID
}

// addCharm inserts a new charm into the database and returns the UUID.
func (s *modelOfferSuite) addCharm(c *tc.C) corecharm.ID {
	charmUUID := corecharmtesting.GenCharmID(c)
	// The UUID is also used as the reference_name as there is a unique
	// constraint on the reference_name, revision and source_id.
	s.query(c, `
INSERT INTO charm (uuid, reference_name, architecture_id) 
VALUES (?, ?, 0)
`, charmUUID, charmUUID)
	return charmUUID
}

func (s *modelOfferSuite) addCharmMetadata(c *tc.C, charmUUID corecharm.ID, subordinate bool) {
	s.query(c, `
INSERT INTO charm_metadata (charm_uuid, name, subordinate) 
VALUES (?, ?, ?)
`, charmUUID, charmUUID, subordinate)
}

// addCharmRelation inserts a new charm relation into the database with the
// given UUID and attributes. Returns the relation UUID.
func (s *modelOfferSuite) addCharmRelation(c *tc.C, charmUUID corecharm.ID, r charm.Relation) string {
	charmRelationUUID := internaluuid.MustNewUUID().String()
	s.query(c, `
INSERT INTO charm_relation (uuid, charm_uuid, name, role_id, interface, optional, capacity, scope_id) 
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
`, charmRelationUUID, charmUUID, r.Name, s.encodeRoleID(r.Role), r.Interface, r.Optional, r.Limit, s.encodeScopeID(r.Scope))
	return charmRelationUUID
}

// encodeRoleID returns the ID used in the database for the given charm role. This
// reflects the contents of the charm_relation_role table.
func (s *modelOfferSuite) encodeRoleID(role charm.RelationRole) int {
	return map[charm.RelationRole]int{
		charm.RoleProvider: 0,
		charm.RoleRequirer: 1,
		charm.RolePeer:     2,
	}[role]
}

// encodeScopeID returns the ID used in the database for the given charm scope. This
// reflects the contents of the charm_relation_scope table.
func (s *modelOfferSuite) encodeScopeID(role charm.RelationScope) int {
	return map[charm.RelationScope]int{
		charm.ScopeGlobal:    0,
		charm.ScopeContainer: 1,
	}[role]
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
