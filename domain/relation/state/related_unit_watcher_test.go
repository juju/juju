// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/application"
	corerelation "github.com/juju/juju/core/relation"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/internal/charm"
)

// relatedUnitWatcherSuite is a test suite dedicated to check function used in the
// related unit watcher, which rely on several state method to filter events.
// It extends baseRelationSuite to leverage common setup and utility methods
// for relation-related testing and provides more builder dedicated for this
// specific context.
type relatedUnitWatcherSuite struct {
	baseRelationSuite
}

var _ = gc.Suite(&relatedUnitWatcherSuite{})

func (s *relatedUnitWatcherSuite) SetUpTest(c *gc.C) {
	s.baseRelationSuite.SetUpTest(c)
}

func (s *relatedUnitWatcherSuite) TestGetRelatedEndpointUUIDForUnit(c *gc.C) {
	// Arrange: two application linked by two relation.
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
	app1ProvUUID := s.addApplicationEndpoint(c, appUUID1, charmRelationProvidesUUID)
	app2ReqUUID := s.addApplicationEndpoint(c, appUUID2, charmRelationRequiresUUID)
	app2ProvUUID := s.addApplicationEndpoint(c, appUUID2, charmRelationProvidesUUID)
	otherRelationUUID := s.addRelation(c)
	s.addRelationEndpoint(c, otherRelationUUID, app2ReqUUID)
	s.addRelationEndpoint(c, otherRelationUUID, app1ProvUUID)

	// We create a units on both side, and a relation in which we are interested
	s.addUnit(c, "app1/0", appUUID1, charmUUID)
	s.addUnit(c, "app2/0", appUUID2, charmUUID)
	relationUUID := s.addRelation(c)
	s.addRelationEndpoint(c, relationUUID, app1ReqUUID)
	expectedEndpoint := s.addRelationEndpoint(c, relationUUID, app2ProvUUID)

	// Act
	var gotEndpoint relationEndpoint
	var err error
	err = s.Txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		gotEndpoint, err = s.state.getRelatedRelationEndpointForUnit(ctx, tx, "app1/0", relationUUID)
		return err
	})

	// Assert
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotEndpoint, gc.Equals, relationEndpoint{
		UUID:            corerelation.EndpointUUID(expectedEndpoint),
		ApplicationUUID: appUUID2,
	})
}

func (s *relatedUnitWatcherSuite) TestGetRelatedEndpointUUIDForUnitPeerRelation(c *gc.C) {
	// Arrange: One application, call on a peer relation
	charmUUID := s.addCharm(c)
	charmRelationUUID := s.addCharmRelation(c, charmUUID, charm.Relation{
		Name:  "peer",
		Role:  charm.RolePeer,
		Scope: charm.ScopeGlobal,
	})
	appUUID1 := s.addApplication(c, charmUUID, "app1")

	// We create a unit, and the peer relation
	s.addUnit(c, "app1/0", appUUID1, charmUUID)
	relationUUID := s.addRelation(c)
	s.addRelationEndpoint(c, relationUUID, s.addApplicationEndpoint(c, appUUID1, charmRelationUUID))

	// Act
	var gotEndpointUUID relationEndpoint
	var err error
	err = s.Txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		gotEndpointUUID, err = s.state.getRelatedRelationEndpointForUnit(ctx, tx, "app1/0", relationUUID)
		return err
	})

	// Assert
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotEndpointUUID, gc.Equals, relationEndpoint{})
}

func (s *relatedUnitWatcherSuite) TestGetRelatedUnits(c *gc.C) {
	// Arrange: two application linked by a relation, with few units
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
	s.addRelationEndpoint(c, relationUUID, app1ReqUUID)
	s.addRelationEndpoint(c, relationUUID, app2ProvUUID)

	// create some units
	createUnit := func(appID application.ID) func(name coreunit.Name) getRelatedUnit {
		return func(name coreunit.Name) getRelatedUnit {
			return getRelatedUnit{
				UUID: s.addUnit(c, name, appID, charmUUID),
				Name: name,
			}
		}
	}
	// We should get all unit except the fetched one.
	fetchedUnit := createUnit(appUUID1)("app1/0")
	expectedUnits := append(
		transform.Slice([]coreunit.Name{"app1/1", "app1/2"}, createUnit(appUUID1)),
		transform.Slice([]coreunit.Name{"app2/0", "app2/1", "app2/3", "app2/2"}, createUnit(appUUID2))...)

	// Act
	var gotUnits []getRelatedUnit
	var err error
	err = s.Txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		gotUnits, err = s.state.getRelatedUnits(ctx, tx, fetchedUnit.Name, relationUUID)
		return err
	})

	// Assert
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotUnits, jc.SameContents, expectedUnits)
}

func (s *relatedUnitWatcherSuite) TestGetRelatedUnitsPeerRelation(c *gc.C) {
	// Arrange: two application linked by a relation, with few units
	charmUUID := s.addCharm(c)
	charmRelationUUID := s.addCharmRelation(c, charmUUID, charm.Relation{
		Name:  "peer",
		Role:  charm.RolePeer,
		Scope: charm.ScopeGlobal,
	})
	appUUID1 := s.addApplication(c, charmUUID, "app1")

	// We create a unit, and the peer relation
	relationUUID := s.addRelation(c)
	s.addRelationEndpoint(c, relationUUID, s.addApplicationEndpoint(c, appUUID1, charmRelationUUID))

	// create some units
	createUnit := func(appID application.ID) func(name coreunit.Name) getRelatedUnit {
		return func(name coreunit.Name) getRelatedUnit {
			return getRelatedUnit{
				UUID: s.addUnit(c, name, appID, charmUUID),
				Name: name,
			}
		}
	}
	// We should get all unit except the fetched one.
	fetchedUnit := createUnit(appUUID1)("app1/0")
	expectedUnits := append(
		transform.Slice([]coreunit.Name{"app1/1", "app1/2"}, createUnit(appUUID1)))

	// Act
	var gotUnits []getRelatedUnit
	var err error
	err = s.Txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		gotUnits, err = s.state.getRelatedUnits(ctx, tx, fetchedUnit.Name, relationUUID)
		return err
	})

	// Assert
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotUnits, jc.SameContents, expectedUnits)
}
