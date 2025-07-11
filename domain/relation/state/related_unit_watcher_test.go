// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"testing"

	"github.com/canonical/sqlair"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/charm"
)

// relatedUnitWatcherSuite is a test suite for checking functions used in
// the related unit watcher, which relies on some state methods to filter events.
// It extends baseRelationSuite to lever common setup and utility methods
// for relation-related testing and provides more builders dedicated for this
// specific context.
type relatedUnitWatcherSuite struct {
	baseRelationSuite
}

func TestRelatedUnitWatcherSuite(t *testing.T) {
	tc.Run(t, &relatedUnitWatcherSuite{})
}

func (s *relatedUnitWatcherSuite) SetUpTest(c *tc.C) {
	s.baseRelationSuite.SetUpTest(c)
}

func (s *relatedUnitWatcherSuite) TestGetUnitsInRelation(c *tc.C) {
	// Arrange: two applications linked by a relation, with a few units,
	// one of which is in scope.
	charmUUID := s.addCharm(c)
	charmRelationProvidesUUID := s.addCharmRelation(c, charmUUID, charm.Relation{
		Name:  "provides",
		Role:  charm.RoleProvider,
		Scope: charm.ScopeGlobal,
	})
	charmRelationRequiresUUID := s.addCharmRelation(c, charmUUID, charm.Relation{
		Name:  "requires",
		Role:  charm.RoleRequirer,
		Scope: charm.ScopeGlobal,
	})

	appUUID1 := s.addApplication(c, charmUUID, "app1")
	appUUID2 := s.addApplication(c, charmUUID, "app2")

	app1ReqUUID := s.addApplicationEndpoint(c, appUUID1, charmRelationRequiresUUID)
	app2ProvUUID := s.addApplicationEndpoint(c, appUUID2, charmRelationProvidesUUID)

	relationUUID := s.addRelation(c)
	re1UUID := s.addRelationEndpoint(c, relationUUID, app1ReqUUID)
	re2UUID := s.addRelationEndpoint(c, relationUUID, app2ProvUUID)

	u10UUID := s.addUnit(c, "app1/0", appUUID1, charmUUID)
	u11UUID := s.addUnit(c, "app1/1", appUUID1, charmUUID)
	u20UUID := s.addUnit(c, "app2/0", appUUID2, charmUUID)

	var relationUnitUUID string
	err := s.Txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		relationUnitUUID, err = s.state.insertRelationUnit(c.Context(), tx, relationUUID.String(), u10UUID.String())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	// Act
	related, err := s.state.getUnitsInRelation(c.Context(), relationUUID.String())

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(related, tc.SameContents, []relationUnit{
		{
			RelationEndpointUUID: re1UUID,
			UnitUUID:             u10UUID.String(),
			RelationUnitUUID:     relationUnitUUID,
		},
		{
			RelationEndpointUUID: re1UUID,
			UnitUUID:             u11UUID.String(),
		},
		{
			RelationEndpointUUID: re2UUID,
			UnitUUID:             u20UUID.String(),
		},
	})
}

func (s *relatedUnitWatcherSuite) TestGetRelatedUnitsPeerRelation(c *tc.C) {
	// Arrange: two units in a peer relation, and one from another application,
	// which should be ignored.
	charmUUID := s.addCharm(c)
	charmRelationUUID := s.addCharmRelation(c, charmUUID, charm.Relation{
		Name:  "peer",
		Role:  charm.RolePeer,
		Scope: charm.ScopeGlobal,
	})

	appUUID1 := s.addApplication(c, charmUUID, "app1")
	appUUID2 := s.addApplication(c, charmUUID, "app2")

	relationUUID := s.addRelation(c)
	re1UUID := s.addRelationEndpoint(c, relationUUID, s.addApplicationEndpoint(c, appUUID1, charmRelationUUID))

	u10UUID := s.addUnit(c, "app1/0", appUUID1, charmUUID)
	u11UUID := s.addUnit(c, "app1/1", appUUID1, charmUUID)
	_ = s.addUnit(c, "app2/0", appUUID2, charmUUID)

	// Act
	related, err := s.state.getUnitsInRelation(c.Context(), relationUUID.String())

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(related, tc.SameContents, []relationUnit{
		{
			RelationEndpointUUID: re1UUID,
			UnitUUID:             u10UUID.String(),
		},
		{
			RelationEndpointUUID: re1UUID,
			UnitUUID:             u11UUID.String(),
		},
	})
}

func (s *relatedUnitWatcherSuite) TestGetRelatedAppEndpoints(c *tc.C) {
	// Arrange: two applications linked by a relation, with a few endpoints,
	// one of which is in scope.
	charmUUID := s.addCharm(c)
	charmRelationProvidesUUID := s.addCharmRelation(c, charmUUID, charm.Relation{
		Name:  "provides",
		Role:  charm.RoleProvider,
		Scope: charm.ScopeGlobal,
	})
	charmRelationRequiresUUID := s.addCharmRelation(c, charmUUID, charm.Relation{
		Name:  "requires",
		Role:  charm.RoleRequirer,
		Scope: charm.ScopeGlobal,
	})

	appUUID1 := s.addApplication(c, charmUUID, "app1")
	appUUID2 := s.addApplication(c, charmUUID, "app2")

	app1ReqUUID := s.addApplicationEndpoint(c, appUUID1, charmRelationRequiresUUID)
	app2ProvUUID := s.addApplicationEndpoint(c, appUUID2, charmRelationProvidesUUID)

	relationUUID := s.addRelation(c)
	re1UUID := s.addRelationEndpoint(c, relationUUID, app1ReqUUID)
	re2UUID := s.addRelationEndpoint(c, relationUUID, app2ProvUUID)

	// Act
	appByEndpoint, err := s.state.getRelationAppEndpoints(c.Context(), relationUUID.String())

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(appByEndpoint[re1UUID], tc.Equals, appUUID1.String())
	c.Check(appByEndpoint[re2UUID], tc.Equals, appUUID2.String())
}
