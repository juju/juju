// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"testing"

	"github.com/canonical/sqlair"
	"github.com/juju/tc"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/changestream"
	corerelation "github.com/juju/juju/core/relation"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/deployment/charm"
	domainrelation "github.com/juju/juju/domain/relation"
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

// TestGetUnitsInRelationContainerScope checks that getUnitsInRelation
// populates the principal unit UUID for subordinate units, which the
// co-location filtering of container-scoped relations relies on.
func (s *relatedUnitWatcherSuite) TestGetUnitsInRelationContainerScope(c *tc.C) {
	// Arrange: a container-scoped relation between a principal application
	// with two units and a subordinate application with one unit per
	// principal unit.
	fixture := s.setupContainerScopedRelation(c)

	// Act
	related, err := s.state.getUnitsInRelation(c.Context(), fixture.relationUUID.String())

	// Assert: all units are returned, with the principal UUID populated for
	// subordinate units only.
	c.Assert(err, tc.ErrorIsNil)
	principalByUnit := make(map[string]string, len(related))
	for _, u := range related {
		principalByUnit[u.UnitUUID] = u.PrincipalUUID
	}
	c.Check(principalByUnit, tc.DeepEquals, map[string]string{
		fixture.principal0UUID.String():   "",
		fixture.principal1UUID.String():   "",
		fixture.subordinate0UUID.String(): fixture.principal0UUID.String(),
		fixture.subordinate1UUID.String(): fixture.principal1UUID.String(),
	})
}

// TestInitialWatchRelatedUnitsContainerScopedSubordinateView checks that a
// subordinate unit only sees its own principal unit in the initial event of
// the related units watcher for a container-scoped relation.
func (s *relatedUnitWatcherSuite) TestInitialWatchRelatedUnitsContainerScopedSubordinateView(c *tc.C) {
	// Arrange: a container-scoped relation between a principal application
	// with two units and a subordinate application with one unit per
	// principal unit. The watcher is created for subordinate/1.
	fixture := s.setupContainerScopedRelation(c)

	// Act
	_, initialQuery, _, err := s.state.InitialWatchRelatedUnits(
		c.Context(), fixture.subordinate1UUID.String(), fixture.relationUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	events, err := initialQuery(c.Context(), s.TxnRunner())

	// Assert: only the remote application and the co-located principal unit
	// are returned, not the principal unit on the other machine.
	c.Assert(err, tc.ErrorIsNil)
	c.Check(events, tc.SameContents, []string{
		domainrelation.EncodeApplicationUUID(fixture.principalAppUUID.String()),
		domainrelation.EncodeUnitUUID(fixture.principal1UUID.String()),
	})
}

// TestInitialWatchRelatedUnitsContainerScopedPrincipalView checks that a
// principal unit only sees its own subordinate units in the initial event of
// the related units watcher for a container-scoped relation.
func (s *relatedUnitWatcherSuite) TestInitialWatchRelatedUnitsContainerScopedPrincipalView(c *tc.C) {
	// Arrange: a container-scoped relation between a principal application
	// with two units and a subordinate application with one unit per
	// principal unit. The watcher is created for principal/0.
	fixture := s.setupContainerScopedRelation(c)

	// Act
	_, initialQuery, _, err := s.state.InitialWatchRelatedUnits(
		c.Context(), fixture.principal0UUID.String(), fixture.relationUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	events, err := initialQuery(c.Context(), s.TxnRunner())

	// Assert: only the remote application and the co-located subordinate
	// unit are returned, not the subordinate unit of the other principal.
	c.Assert(err, tc.ErrorIsNil)
	c.Check(events, tc.SameContents, []string{
		domainrelation.EncodeApplicationUUID(fixture.subordinateAppUUID.String()),
		domainrelation.EncodeUnitUUID(fixture.subordinate0UUID.String()),
	})
}

// TestInitialWatchRelatedUnitsContainerScopedMapper checks that the related
// units watcher mapper discards events from units that are not co-located
// with the watching unit in a container-scoped relation.
func (s *relatedUnitWatcherSuite) TestInitialWatchRelatedUnitsContainerScopedMapper(c *tc.C) {
	// Arrange: a container-scoped relation between a principal application
	// with two units and a subordinate application with one unit per
	// principal unit. Both principal units have entered scope. The watcher
	// is created for subordinate/1.
	fixture := s.setupContainerScopedRelation(c)
	relUnitP0UUID := s.addRelationUnit(c, fixture.principal0UUID, fixture.principalEndpointUUID)
	relUnitP1UUID := s.addRelationUnit(c, fixture.principal1UUID, fixture.principalEndpointUUID)

	_, _, mapper, err := s.state.InitialWatchRelatedUnits(
		c.Context(), fixture.subordinate1UUID.String(), fixture.relationUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	// Act
	out, err := mapper(c.Context(), []changestream.ChangeEvent{
		// Entering scope of the other principal unit: not co-located.
		changeEvent{namespace: "relation_unit", changed: fixture.principal0UUID.String()},
		// Entering scope of the co-located principal unit.
		changeEvent{namespace: "relation_unit", changed: fixture.principal1UUID.String()},
		// Entering scope of a unit on the watching unit's side.
		changeEvent{namespace: "relation_unit", changed: fixture.subordinate0UUID.String()},
		// Settings change for the other principal unit: not co-located.
		changeEvent{namespace: "relation_unit_settings_hash", changed: relUnitP0UUID.String()},
		// Settings change for the co-located principal unit.
		changeEvent{namespace: "relation_unit_settings_hash", changed: relUnitP1UUID.String()},
		// Application settings change for the remote application.
		changeEvent{namespace: "relation_application_settings_hash", changed: fixture.principalEndpointUUID},
	})

	// Assert: only events from the co-located principal unit and the remote
	// application are mapped.
	c.Assert(err, tc.ErrorIsNil)
	c.Check(out, tc.SameContents, []string{
		domainrelation.EncodeUnitUUID(fixture.principal1UUID.String()),
		domainrelation.EncodeUnitUUID(fixture.principal1UUID.String()),
		domainrelation.EncodeApplicationUUID(fixture.principalAppUUID.String()),
	})
}

// TestInitialWatchRelatedUnitsGlobalScopedSubordinateView checks that no
// co-location filtering applies for globally-scoped relations: a subordinate
// unit sees every unit of the remote application.
func (s *relatedUnitWatcherSuite) TestInitialWatchRelatedUnitsGlobalScopedSubordinateView(c *tc.C) {
	// Arrange: the same applications as in the container-scoped fixture, but
	// linked by a globally-scoped relation. The watcher is created for
	// subordinate/1.
	fixture := s.setupContainerScopedRelation(c)
	globalRelationUUID := s.addRelation(c)
	s.addRelationEndpoint(c, globalRelationUUID, fixture.principalAppEndpointUUID)
	s.addRelationEndpoint(c, globalRelationUUID, fixture.subordinateAppEndpointUUID)

	// Act
	_, initialQuery, _, err := s.state.InitialWatchRelatedUnits(
		c.Context(), fixture.subordinate1UUID.String(), globalRelationUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	events, err := initialQuery(c.Context(), s.TxnRunner())

	// Assert: both principal units are returned.
	c.Assert(err, tc.ErrorIsNil)
	c.Check(events, tc.SameContents, []string{
		domainrelation.EncodeApplicationUUID(fixture.principalAppUUID.String()),
		domainrelation.EncodeUnitUUID(fixture.principal0UUID.String()),
		domainrelation.EncodeUnitUUID(fixture.principal1UUID.String()),
	})
}

// changeEvent is a minimal changestream.ChangeEvent implementation for
// exercising watcher mappers.
type changeEvent struct {
	namespace string
	changed   string
}

func (e changeEvent) Type() changestream.ChangeType { return changestream.Changed }
func (e changeEvent) Namespace() string             { return e.namespace }
func (e changeEvent) Changed() string               { return e.changed }

// containerScopedRelation holds the identifiers of a container-scoped
// relation fixture between a principal and a subordinate application.
type containerScopedRelation struct {
	relationUUID               corerelation.UUID
	principalAppUUID           coreapplication.UUID
	subordinateAppUUID         coreapplication.UUID
	principalAppEndpointUUID   string
	subordinateAppEndpointUUID string
	principalEndpointUUID      string
	principal0UUID             coreunit.UUID
	principal1UUID             coreunit.UUID
	subordinate0UUID           coreunit.UUID
	subordinate1UUID           coreunit.UUID
}

// setupContainerScopedRelation arranges a container-scoped relation between
// a principal application with two units and a subordinate application with
// one subordinate unit per principal unit.
func (s *relatedUnitWatcherSuite) setupContainerScopedRelation(c *tc.C) containerScopedRelation {
	charmUUID := s.addCharm(c)
	charmRelationProvidesUUID := s.addCharmRelation(c, charmUUID, charm.Relation{
		Name:  "provides",
		Role:  charm.RoleProvider,
		Scope: charm.ScopeGlobal,
	})
	subordinateCharmUUID := s.addCharm(c)
	s.addCharmMetadata(c, subordinateCharmUUID, true)
	charmRelationRequiresUUID := s.addCharmRelation(c, subordinateCharmUUID, charm.Relation{
		Name:  "requires",
		Role:  charm.RoleRequirer,
		Scope: charm.ScopeContainer,
	})

	principalAppUUID := s.addApplication(c, charmUUID, "principal")
	subordinateAppUUID := s.addApplication(c, subordinateCharmUUID, "subordinate")

	principalAppEndpointUUID := s.addApplicationEndpoint(c, principalAppUUID, charmRelationProvidesUUID)
	subordinateAppEndpointUUID := s.addApplicationEndpoint(c, subordinateAppUUID, charmRelationRequiresUUID)

	relationUUID := s.addRelationWithScope(c, charm.ScopeContainer)
	principalEndpointUUID := s.addRelationEndpoint(c, relationUUID, principalAppEndpointUUID)
	s.addRelationEndpoint(c, relationUUID, subordinateAppEndpointUUID)

	principal0UUID := s.addUnit(c, "principal/0", principalAppUUID, charmUUID)
	principal1UUID := s.addUnit(c, "principal/1", principalAppUUID, charmUUID)
	subordinate0UUID := s.addUnit(c, "subordinate/0", subordinateAppUUID, subordinateCharmUUID)
	subordinate1UUID := s.addUnit(c, "subordinate/1", subordinateAppUUID, subordinateCharmUUID)
	s.setUnitSubordinate(c, subordinate0UUID, principal0UUID)
	s.setUnitSubordinate(c, subordinate1UUID, principal1UUID)

	return containerScopedRelation{
		relationUUID:               relationUUID,
		principalAppUUID:           principalAppUUID,
		subordinateAppUUID:         subordinateAppUUID,
		principalAppEndpointUUID:   principalAppEndpointUUID,
		subordinateAppEndpointUUID: subordinateAppEndpointUUID,
		principalEndpointUUID:      principalEndpointUUID,
		principal0UUID:             principal0UUID,
		principal1UUID:             principal1UUID,
		subordinate0UUID:           subordinate0UUID,
		subordinate1UUID:           subordinate1UUID,
	}
}
