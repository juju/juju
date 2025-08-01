// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"testing"

	"github.com/canonical/sqlair"
	"github.com/juju/tc"

	"github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/network"
	networkerrors "github.com/juju/juju/domain/network/errors"
	"github.com/juju/juju/internal/charm"
)

type spaceDeleteSuite struct {
	linkLayerBaseSuite
}

func TestSpaceDeleteSuite(t *testing.T) {
	tc.Run(t, &spaceDeleteSuite{})
}

// TestDeleteSpaceNotFoundError tests that if we try to call RemoveSpace with
// a non-existent space name, it will return an error.
func (s *spaceDeleteSuite) TestDeleteSpaceNotFoundError(c *tc.C) {
	// Try to remove a space that doesn't exist
	nonExistentSpaceName := network.SpaceName("non-existent-space")
	err := s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.deleteSpace(ctx, tx, nonExistentSpaceName)
	})

	// Verify that an error is returned
	c.Assert(err, tc.ErrorIs, networkerrors.SpaceNotFound)
}

// TestDeleteAlphaSpaceError tests that if we try to call RemoveSpace with
// the alpha space name, it will return an error.
func (s *spaceDeleteSuite) TestDeleteAlphaSpaceError(c *tc.C) {
	// Try to remove the alpha space
	err := s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.deleteSpace(ctx, tx, network.AlphaSpaceName)
	})

	// Verify that an error is returned
	c.Assert(err, tc.ErrorMatches, ".*cannot remove the alpha space.*")
}

// TestDeleteSpaceRemoveConstraints ensures that deleting a space removes all
// related application space constraints.
func (s *spaceDeleteSuite) TestDeleteSpaceRemoveConstraints(c *tc.C) {
	// Arrange
	s.addSpaceWithName(c, "toDelete")
	s.addSpaceWithName(c, "other")

	appUUID1 := s.addApplication(c, s.addCharm(c), network.AlphaSpaceId.String())
	appUUID2 := s.addApplication(c, s.addCharm(c), network.AlphaSpaceId.String())
	s.addApplicationSpaceConstraint(c, appUUID1, "toDelete", false)
	s.addApplicationSpaceConstraint(c, appUUID2, "other", false)

	// Act
	err := s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.deleteSpace(ctx, tx, "toDelete")
	})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	obtained := s.selectDistinctValues(c, "space", "constraint_space")
	c.Assert(obtained, tc.SameContents, []string{"other"})
}

// TestDeleteSpaceResetApplicationBindings ensures that deleting a space resets
// application bindings associated with it to Alpha space
func (s *spaceDeleteSuite) TestDeleteSpaceResetApplicationBindings(c *tc.C) {

	// Arrange
	toDeleteUUID := s.addSpaceWithName(c, "toDelete")
	otherUUID := s.addSpaceWithName(c, "other")

	s.addApplication(c, s.addCharm(c), toDeleteUUID)
	s.addApplication(c, s.addCharm(c), otherUUID)

	// Act
	err := s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.deleteSpace(ctx, tx, "toDelete")
	})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	obtained := s.selectDistinctValues(c, "space_uuid", "application")
	c.Assert(obtained, tc.SameContents, []string{otherUUID, network.AlphaSpaceId.String()})
}

// TestDeleteSpaceRemoveEndpointBindings ensures that deleting a space removes
// associated endpoint bindings correctly.
func (s *spaceDeleteSuite) TestDeleteSpaceRemoveEndpointBindings(c *tc.C) {

	// Arrange
	toDeleteUUID := s.addSpaceWithName(c, "toDelete")
	otherUUID := s.addSpaceWithName(c, "other")

	charmUUID := s.addCharm(c)
	appUUID1 := s.addApplication(c, charmUUID, network.AlphaSpaceId.String())
	appUUID2 := s.addApplication(c, charmUUID, network.AlphaSpaceId.String())
	charmRelationUUID1 := s.addCharmRelation(c, corecharm.ID(charmUUID),
		charm.Relation{Name: "ep1", Role: charm.RoleProvider, Scope: charm.ScopeGlobal})
	charmRelationUUID2 := s.addCharmRelation(c, corecharm.ID(charmUUID),
		charm.Relation{Name: "ep2", Role: charm.RoleProvider, Scope: charm.ScopeGlobal})

	s.addApplicationEndpoint(c, application.ID(appUUID1), charmRelationUUID1, toDeleteUUID)
	s.addApplicationEndpoint(c, application.ID(appUUID2), charmRelationUUID2, otherUUID)

	// Act
	err := s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.deleteSpace(ctx, tx, "toDelete")
	})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	obtained := s.selectDistinctValues(c, "space_uuid", "application_endpoint")
	c.Assert(obtained, tc.SameContents, []string{otherUUID, "" /* empty */})
}

// TestDeleteSpaceRemoveExtraBindings ensures that deleting a space removes all
// associated application extra bindings.
func (s *spaceDeleteSuite) TestDeleteSpaceRemoveExtraBindings(c *tc.C) {

	// Arrange
	toDeleteUUID := s.addSpaceWithName(c, "toDelete")
	otherUUID := s.addSpaceWithName(c, "other")

	charmUUID := s.addCharm(c)
	appUUID1 := s.addApplication(c, charmUUID, network.AlphaSpaceId.String())
	appUUID2 := s.addApplication(c, charmUUID, network.AlphaSpaceId.String())
	charmExtraUUID1 := s.addCharmExtraBinding(c, corecharm.ID(charmUUID), "extra1")
	charmExtraUUID2 := s.addCharmExtraBinding(c, corecharm.ID(charmUUID), "extra2")

	s.addApplicationExtraEndpoint(c, application.ID(appUUID1), charmExtraUUID1, toDeleteUUID)
	s.addApplicationExtraEndpoint(c, application.ID(appUUID2), charmExtraUUID2, otherUUID)

	// Act
	err := s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.deleteSpace(ctx, tx, "toDelete")
	})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	obtained := s.selectDistinctValues(c, "space_uuid", "application_extra_endpoint")
	c.Assert(obtained, tc.SameContents, []string{otherUUID, "" /* empty */})
}

// TestDeleteSpaceResetExposedEndpoints verifies that exposed endpoints
// associated with a deleted space are reset correctly.
func (s *spaceDeleteSuite) TestDeleteSpaceResetExposedEndpoints(c *tc.C) {

	// Arrange
	toDeleteUUID := s.addSpaceWithName(c, "toDelete")
	otherUUID := s.addSpaceWithName(c, "other")

	charmUUID := s.addCharm(c)
	appUUID1 := s.addApplication(c, charmUUID, network.AlphaSpaceId.String())
	appUUID2 := s.addApplication(c, charmUUID, network.AlphaSpaceId.String())
	charmRelationUUID1 := s.addCharmRelation(c, corecharm.ID(charmUUID),
		charm.Relation{Name: "ep1", Role: charm.RoleProvider, Scope: charm.ScopeGlobal})
	charmRelationUUID2 := s.addCharmRelation(c, corecharm.ID(charmUUID),
		charm.Relation{Name: "ep2", Role: charm.RoleProvider, Scope: charm.ScopeGlobal})

	epUUID1 := s.addApplicationEndpoint(c, application.ID(appUUID1), charmRelationUUID1, "")
	epUUID2 := s.addApplicationEndpoint(c, application.ID(appUUID2), charmRelationUUID2, "")

	s.addApplicationExposedEndpoint(c, appUUID1, epUUID1, toDeleteUUID)
	s.addApplicationExposedEndpoint(c, appUUID2, epUUID2, otherUUID)

	// Act
	err := s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.deleteSpace(ctx, tx, "toDelete")
	})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	obtained := s.selectDistinctValues(c, "space_uuid", "application_exposed_endpoint_space")
	c.Assert(obtained, tc.SameContents, []string{otherUUID, network.AlphaSpaceId.String()})
}

// TestDeleteSpaceMoveSubnets tests that subnets associated with a deleted space
// are moved to the alpha space.
func (s *spaceDeleteSuite) TestDeleteSpaceMoveSubnets(c *tc.C) {

	// Arrange
	toDeleteUUID := s.addSpaceWithName(c, "toDelete")
	otherUUID := s.addSpaceWithName(c, "other")

	s.addSubnet(c, "192.0.2.0/24", toDeleteUUID)
	s.addSubnet(c, "198.51.100.0/24", otherUUID)

	// Act
	err := s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.deleteSpace(ctx, tx, "toDelete")
	})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	obtained := s.selectDistinctValues(c, "space_uuid", "subnet")
	c.Assert(obtained, tc.SameContents, []string{otherUUID, network.AlphaSpaceId.String()})
}

// TestDeleteSpace verifies that a space can be deleted, ensuring provider ID
// and related records are removed correctly.
func (s *spaceDeleteSuite) TestDeleteSpace(c *tc.C) {
	// Provider id removed, record removed
	// Arrange
	s.addSpaceWithNameAndProvider(c, "toDelete", "provider1")
	otherUUID := s.addSpaceWithNameAndProvider(c, "other", "provider2")

	// Act
	err := s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.deleteSpace(ctx, tx, "toDelete")
	})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	s.checkRowCount(c, "provider_space", 1)
	s.checkRowCount(c, "space", 2)
	providerIds := s.selectDistinctValues(c, "space_uuid", "provider_space")
	c.Assert(providerIds, tc.SameContents, []string{otherUUID})
	spaces := s.selectDistinctValues(c, "name", "space")
	c.Assert(spaces, tc.SameContents, []string{"other", network.AlphaSpaceName.String()})
}

// TestHasModelSpaceConstraintsTrue verifies that the hasModelSpaceConstraint
// method correctly identifies existing constraints.
func (s *spaceDeleteSuite) TestHasModelSpaceConstraintsTrue(c *tc.C) {
	// Arrange
	s.addModelSpaceConstraint(c, network.AlphaSpaceName.String(), true)

	// Act
	var err error
	var result bool
	err = s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		result, err = s.state.hasModelSpaceConstraint(ctx, tx, network.AlphaSpaceName)
		return err
	})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.Equals, true)
}

// TestHasModelSpaceConstraintsFalse verifies that the hasModelSpaceConstraint
// method returns false when no constraints exist for a given space.
func (s *spaceDeleteSuite) TestHasModelSpaceConstraintsFalse(c *tc.C) {
	// Arrange
	s.addSpaceWithName(c, "a-space")
	s.addModelSpaceConstraint(c, "a-space", true)

	// Act
	var err error
	var result bool
	err = s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		result, err = s.state.hasModelSpaceConstraint(ctx, tx, "another-space")
		return err
	})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.Equals, false)
}

// TestGetApplicationConstraintsForSpace verifies the retrieval of application
// constraints tied to a specific space.
func (s *spaceDeleteSuite) TestGetApplicationConstraintsForSpace(c *tc.C) {
	// Arrange
	s.addSpaceWithName(c, "a-space")
	s.addSpaceWithName(c, "another-space")

	appUUID1 := s.addApplication(c, s.addCharm(c), network.AlphaSpaceId.String())
	appUUID2 := s.addApplication(c, s.addCharm(c), network.AlphaSpaceId.String())
	appUUID3 := s.addApplication(c, s.addCharm(c), network.AlphaSpaceId.String())
	s.addApplicationSpaceConstraint(c, appUUID1, "a-space", false)
	s.addApplicationSpaceConstraint(c, appUUID2, "a-space", false)
	s.addApplicationSpaceConstraint(c, appUUID3, "another-space", false)

	// Act
	var err error
	var result []string
	err = s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		result, err = s.state.getApplicationConstraintsForSpace(ctx, tx, "a-space")
		return err
	})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.SameContents, []string{appUUID1, appUUID2})
}

// TestGetApplicationConstraintsForSpaceNone verifies that querying application
// constraints for a non-constrained space returns an empty result.
func (s *spaceDeleteSuite) TestGetApplicationConstraintsForSpaceNone(c *tc.C) {
	// Arrange
	s.addSpaceWithName(c, "a-space")
	s.addSpaceWithName(c, "another-space")

	appUUID1 := s.addApplication(c, s.addCharm(c), network.AlphaSpaceId.String())
	s.addApplicationSpaceConstraint(c, appUUID1, "a-space", false)

	// Act
	var err error
	var result []string
	err = s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		result, err = s.state.getApplicationConstraintsForSpace(ctx, tx, "another-space")
		return err
	})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.SameContents, []string{})
}

// TestGetApplicationBoundToSpace verifies that the correct applications
// are associated with a specific space.
// It ensures only applications bound to the provided space are returned
// and checks for expected functionality.
// The method sets up multiple applications with various bindings,
// executes the querying logic, and validates the results.
func (s *spaceDeleteSuite) TestGetApplicationBoundToSpace(c *tc.C) {
	// Arrange
	spaceUUID := s.addSpaceWithName(c, "a-space")
	anotherSpaceUUID := s.addSpaceWithName(c, "another-space")

	charmUUID := s.addCharm(c)
	charmRelationUUID := s.addCharmRelation(c, corecharm.ID(charmUUID),
		charm.Relation{Name: "ep1", Role: charm.RoleProvider, Scope: charm.ScopeGlobal})
	charmExtraUUID := s.addCharmExtraBinding(c, corecharm.ID(charmUUID), "extra1")
	// default binding
	appUUID1 := s.addApplication(c, charmUUID, spaceUUID)
	// endpoint binding
	appUUID2 := s.addApplication(c, charmUUID, network.AlphaSpaceId.String())
	s.addApplicationEndpoint(c, application.ID(appUUID2), charmRelationUUID, spaceUUID)
	// extra endpoint binding
	appUUID3 := s.addApplication(c, charmUUID, network.AlphaSpaceId.String())
	s.addApplicationExtraEndpoint(c, application.ID(appUUID3), charmExtraUUID, spaceUUID)
	// exposed endpoint binding
	appUUID4 := s.addApplication(c, charmUUID, network.AlphaSpaceId.String())
	epUUID4 := s.addApplicationEndpoint(c, application.ID(appUUID4), charmRelationUUID, "")
	s.addApplicationExposedEndpoint(c, appUUID4, epUUID4, spaceUUID)
	// All bindings (shouldn't be duplicated)
	appUUID5 := s.addApplication(c, charmUUID, spaceUUID)
	epUUID5 := s.addApplicationEndpoint(c, application.ID(appUUID5), charmRelationUUID, spaceUUID)
	s.addApplicationExtraEndpoint(c, application.ID(appUUID5), charmExtraUUID, spaceUUID)
	s.addApplicationExposedEndpoint(c, appUUID5, epUUID5, spaceUUID)

	// No binding (shouldn't be found at all)
	appUUID6 := s.addApplication(c, charmUUID, anotherSpaceUUID)

	// Act
	var err error
	var result []string
	err = s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		result, err = s.state.getApplicationBoundToSpace(ctx, tx, "a-space")
		return err
	})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.SameContents, []string{appUUID1, appUUID2, appUUID3, appUUID4, appUUID5},
		tc.Commentf("infos: %v", map[string]string{
			"appUUID1": appUUID1,
			"appUUID2": appUUID2,
			"appUUID3": appUUID3,
			"appUUID4": appUUID4,
			"appUUID5": appUUID5,
			"appUUID6": appUUID6,
		}))
}

// TestGetApplicationBoundToSpaceNone verifies that there is no error returned
// when no application is bound to a specific space.
func (s *spaceDeleteSuite) TestGetApplicationBoundToSpaceNone(c *tc.C) {
	// Arrange
	spaceUUID := s.addSpaceWithName(c, "a-space")
	s.addSpaceWithName(c, "another-space")

	s.addApplication(c, s.addCharm(c), spaceUUID)

	// Act
	var err error
	var result []string
	err = s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		result, err = s.state.getApplicationConstraintsForSpace(ctx, tx, "another-space")
		return err
	})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.SameContents, []string{})
}

func (s *spaceDeleteSuite) TestRemoveSpaceDryRunWithViolation(c *tc.C) {
	// Arrange
	spaceUUID := s.addSpaceWithName(c, "a-space")

	s.addModelSpaceConstraint(c, "a-space", true)
	appUUID1 := s.addApplication(c, s.addCharm(c), spaceUUID)
	appUUID2 := s.addApplication(c, s.addCharm(c), network.AlphaSpaceId.String())
	s.addApplicationSpaceConstraint(c, appUUID2, "a-space", false)

	// Act
	violations, err := s.state.RemoveSpace(c.Context(), "a-space", false, true)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(violations.IsEmpty(), tc.Equals, false)
	c.Assert(violations.HasModelConstraint, tc.Equals, true)
	c.Assert(violations.ApplicationBindings, tc.SameContents, []string{appUUID1})
	c.Assert(violations.ApplicationConstraints, tc.SameContents, []string{appUUID2})
	// No remove
	c.Assert(s.selectDistinctValues(c, "name", "space"), tc.SameContents, []string{
		"a-space",
		network.AlphaSpaceName.String(),
	})
}

func (s *spaceDeleteSuite) TestRemoveSpaceDryRunNoViolation(c *tc.C) {
	// Arrange
	spaceUUID := s.addSpaceWithName(c, "a-space")
	s.addSpaceWithName(c, "b-space")

	s.addModelSpaceConstraint(c, "a-space", true)
	s.addApplication(c, s.addCharm(c), spaceUUID)
	appUUID2 := s.addApplication(c, s.addCharm(c), network.AlphaSpaceId.String())
	s.addApplicationSpaceConstraint(c, appUUID2, "a-space", false)

	// Act
	violations, err := s.state.RemoveSpace(c.Context(), "b-space", false, true)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(violations.IsEmpty(), tc.Equals, true)
	c.Assert(violations.HasModelConstraint, tc.Equals, false)
	c.Assert(violations.ApplicationBindings, tc.SameContents, []string{})
	c.Assert(violations.ApplicationConstraints, tc.SameContents, []string{})
	// No remove
	c.Assert(s.selectDistinctValues(c, "name", "space"), tc.SameContents, []string{
		"a-space",
		"b-space",
		network.AlphaSpaceName.String(),
	})
}

func (s *spaceDeleteSuite) TestRemoveSpaceWithViolation(c *tc.C) {
	// Arrange
	spaceUUID := s.addSpaceWithName(c, "a-space")

	s.addModelSpaceConstraint(c, "a-space", true)
	appUUID1 := s.addApplication(c, s.addCharm(c), spaceUUID)
	appUUID2 := s.addApplication(c, s.addCharm(c), network.AlphaSpaceId.String())
	s.addApplicationSpaceConstraint(c, appUUID2, "a-space", false)

	// Act
	violations, err := s.state.RemoveSpace(c.Context(), "a-space", false, false)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(violations.IsEmpty(), tc.Equals, false)
	c.Assert(violations.HasModelConstraint, tc.Equals, true)
	c.Assert(violations.ApplicationBindings, tc.SameContents, []string{appUUID1})
	c.Assert(violations.ApplicationConstraints, tc.SameContents, []string{appUUID2})
	// No remove
	c.Assert(s.selectDistinctValues(c, "name", "space"), tc.SameContents, []string{
		"a-space",
		network.AlphaSpaceName.String(),
	})
}

func (s *spaceDeleteSuite) TestRemoveSpaceNoViolation(c *tc.C) {
	// Arrange
	spaceUUID := s.addSpaceWithName(c, "a-space")
	s.addSpaceWithName(c, "b-space")

	s.addModelSpaceConstraint(c, "a-space", true)
	s.addApplication(c, s.addCharm(c), spaceUUID)
	appUUID2 := s.addApplication(c, s.addCharm(c), network.AlphaSpaceId.String())
	s.addApplicationSpaceConstraint(c, appUUID2, "a-space", false)

	// Act
	violations, err := s.state.RemoveSpace(c.Context(), "b-space", false, false)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(violations.IsEmpty(), tc.Equals, true)
	c.Assert(violations.HasModelConstraint, tc.Equals, false)
	c.Assert(violations.ApplicationBindings, tc.SameContents, []string{})
	c.Assert(violations.ApplicationConstraints, tc.SameContents, []string{})
	// No remove
	c.Assert(s.selectDistinctValues(c, "name", "space"), tc.SameContents, []string{
		"a-space",
		// b-space removed
		network.AlphaSpaceName.String(),
	})
}

func (s *spaceDeleteSuite) TestRemoveSpaceForcedWithViolation(c *tc.C) {
	// Arrange
	spaceUUID := s.addSpaceWithName(c, "a-space")

	s.addModelSpaceConstraint(c, "a-space", true)
	appUUID1 := s.addApplication(c, s.addCharm(c), spaceUUID)
	appUUID2 := s.addApplication(c, s.addCharm(c), network.AlphaSpaceId.String())
	s.addApplicationSpaceConstraint(c, appUUID2, "a-space", false)

	// Act
	violations, err := s.state.RemoveSpace(c.Context(), "a-space", true, false)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(violations.IsEmpty(), tc.Equals, false)
	c.Assert(violations.HasModelConstraint, tc.Equals, true)
	c.Assert(violations.ApplicationBindings, tc.SameContents, []string{appUUID1})
	c.Assert(violations.ApplicationConstraints, tc.SameContents, []string{appUUID2})
	// No remove
	c.Assert(s.selectDistinctValues(c, "name", "space"), tc.SameContents, []string{
		// a-space removed
		network.AlphaSpaceName.String(),
	})
}

func (s *spaceDeleteSuite) TestRemoveSpaceForcedNoViolation(c *tc.C) {
	// Arrange
	spaceUUID := s.addSpaceWithName(c, "a-space")
	s.addSpaceWithName(c, "b-space")

	s.addModelSpaceConstraint(c, "a-space", true)
	s.addApplication(c, s.addCharm(c), spaceUUID)
	appUUID2 := s.addApplication(c, s.addCharm(c), network.AlphaSpaceId.String())
	s.addApplicationSpaceConstraint(c, appUUID2, "a-space", false)

	// Act
	violations, err := s.state.RemoveSpace(c.Context(), "b-space", true, false)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(violations.IsEmpty(), tc.Equals, true)
	c.Assert(violations.HasModelConstraint, tc.Equals, false)
	c.Assert(violations.ApplicationBindings, tc.SameContents, []string{})
	c.Assert(violations.ApplicationConstraints, tc.SameContents, []string{})
	// No remove
	c.Assert(s.selectDistinctValues(c, "name", "space"), tc.SameContents, []string{
		"a-space",
		// b-space removed
		network.AlphaSpaceName.String(),
	})
}
