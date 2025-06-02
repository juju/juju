// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"testing"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	"github.com/juju/tc"

	coreapplication "github.com/juju/juju/core/application"
	applicationtesting "github.com/juju/juju/core/application/testing"
	corecharm "github.com/juju/juju/core/charm"
	charmtesting "github.com/juju/juju/core/charm/testing"
	"github.com/juju/juju/core/network"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

// applicationEndpointStateSuite defines the testing suite for managing
// application endpoint state operations.
//
// It embeds baseSuite and provides constants and state for test scenarios.
type applicationEndpointStateSuite struct {
	baseSuite

	appID     coreapplication.ID
	charmUUID corecharm.ID

	state *State
}

func TestApplicationEndpointStateSuite(t *testing.T) {
	tc.Run(t, &applicationEndpointStateSuite{})
}

// SetUpTest sets up the testing environment by initializing the suite's state
// and arranging the required database context:
//   - One charm
//   - One application
func (s *applicationEndpointStateSuite) SetUpTest(c *tc.C) {
	s.baseSuite.SetUpTest(c)

	s.state = NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	// Arrange suite context, same for all tests:
	s.appID = applicationtesting.GenApplicationUUID(c)
	s.charmUUID = charmtesting.GenCharmID(c)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		var err error
		_, err = tx.ExecContext(ctx, `
INSERT INTO charm (uuid, reference_name, source_id) 
VALUES (?, 'foo', 0)`, s.charmUUID)
		if err != nil {
			return errors.Capture(err)
		}
		_, err = tx.ExecContext(ctx, `
INSERT INTO application (uuid, charm_uuid, name, life_id, space_uuid)
VALUES (?,?,?,0,?)`, s.appID, s.charmUUID, "foo", network.AlphaSpaceId)
		if err != nil {
			return errors.Capture(err)
		}

		return nil
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange suite) Failed to setup test suite: %v", err))
}

// TestUpdateDefaultSpace validates behavior when inserting
// application endpoints without a charm relation.
//
// Ensures no relation endpoints are created and no errors occur during the operation,
// while the default endpoint is correctly set
func (s *applicationEndpointStateSuite) TestUpdateDefaultSpace(c *tc.C) {
	// Arrange: No relation
	db, err := s.state.DB()
	c.Assert(err, tc.ErrorIsNil)
	bindings := map[string]network.SpaceName{
		"": s.addSpaceReturningName(c, "beta"),
	}

	// Act:
	err = db.Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.updateDefaultSpace(c.Context(), tx, s.appID, bindings)
	})

	// Assert: Shouldn't have any relation endpoint, but default should be updated
	c.Assert(err, tc.ErrorIsNil)

	c.Check(s.getApplicationDefaultSpace(c), tc.Equals, "beta")
	c.Check(s.fetchApplicationEndpoints(c), tc.DeepEquals, []applicationEndpoint{})
}

func (s *applicationEndpointStateSuite) TestUpdateDefaultSpaceNoBindings(c *tc.C) {
	// Arrange: No relation
	db, err := s.state.DB()
	c.Assert(err, tc.ErrorIsNil)

	// Act:
	err = db.Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.updateDefaultSpace(c.Context(), tx, s.appID, nil)
	})

	// Assert: default space not updated.
	c.Assert(err, tc.ErrorIsNil)

	c.Check(s.getApplicationDefaultSpace(c), tc.Equals, network.AlphaSpaceName)
}

func (s *applicationEndpointStateSuite) TestUpdateDefaultSpaceNoBingingToDefault(c *tc.C) {
	// Arrange:
	// - two expected relation
	// - two expected extra endpoint
	// - one of both are bound with a specific space (beta)
	db, err := s.state.DB()
	c.Assert(err, tc.ErrorIsNil)
	s.addRelation(c, "default")
	s.addRelation(c, "bound")
	s.addExtraBinding(c, "extra")
	s.addExtraBinding(c, "bound-extra")
	bindings := map[string]network.SpaceName{
		"bound":       s.addSpaceReturningName(c, "beta"),
		"bound-extra": s.addSpaceReturningName(c, "beta-extra"),
	}

	// Act:
	err = db.Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.updateDefaultSpace(c.Context(), tx, s.appID, bindings)
	})

	// Assert: default space not updated.
	c.Assert(err, tc.ErrorIsNil)
	c.Check(s.getApplicationDefaultSpace(c), tc.Equals, network.AlphaSpaceName)
}

func (s *applicationEndpointStateSuite) TestInsertApplicationEndpointsApplicationNotFound(c *tc.C) {
	// Arrange:
	db, err := s.state.DB()
	c.Assert(err, tc.ErrorIsNil)

	// Act:
	err = db.Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.insertApplicationEndpointBindings(c.Context(), tx, insertApplicationEndpointsParams{
			appID:    "bad-uuid",
			bindings: nil,
		})
	})

	// Assert:
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

// TestInsertApplicationEndpointsNoCharmRelation validates behavior when inserting
// application endpoints without a charm relation.
//
// Ensures no relation endpoints are created and no errors occur during the operation.
func (s *applicationEndpointStateSuite) TestInsertApplicationEndpointsNoCharmRelation(c *tc.C) {
	// Arrange:
	db, err := s.state.DB()
	c.Assert(err, tc.ErrorIsNil)

	// Act: noop, no error
	db.Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.insertApplicationEndpointBindings(c.Context(), tx, insertApplicationEndpointsParams{
			appID:    s.appID,
			bindings: nil,
		})
	})

	// Assert: Shouldn't have any relation endpoint, default space not updated
	c.Assert(err, tc.ErrorIsNil)
	c.Check(s.fetchApplicationEndpoints(c), tc.DeepEquals, []applicationEndpoint{})
}

// TestInsertApplicationNoBindings tests the insertion of application
// endpoints with no bindings
func (s *applicationEndpointStateSuite) TestInsertApplicationNoBindings(c *tc.C) {
	// Arrange: One expected relation, one extra endpoint, no binding
	db, err := s.state.DB()
	c.Assert(err, tc.ErrorIsNil)
	relUUID := s.addRelation(c, "default")
	extraUUID := s.addExtraBinding(c, "extra")

	// Act: Charm relation will create application endpoint bounded to the default space (alpha)
	err = db.Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.insertApplicationEndpointBindings(c.Context(), tx, insertApplicationEndpointsParams{
			appID:    s.appID,
			bindings: nil,
		})
	})

	// Assert: Should have
	//  - an application endpoint without spacename,
	//  - an application extra endpoint without spacename,
	c.Assert(err, tc.ErrorIsNil)
	c.Check(s.fetchApplicationEndpoints(c), tc.DeepEquals, []applicationEndpoint{
		{
			charmRelationUUID: relUUID,
		},
	})
	c.Check(s.fetchApplicationExtraEndpoints(c), tc.DeepEquals, []applicationEndpoint{
		{
			charmRelationUUID: extraUUID,
		},
	})
}

// TestInsertApplicationEndpointDefaultedSpace verifies the insertion of
// application endpoints while setting the default space
func (s *applicationEndpointStateSuite) TestInsertApplicationEndpointDefaultedSpace(c *tc.C) {
	// Arrange:
	// - One expected relation, one expected endpoint
	// - override default space to beta
	db, err := s.state.DB()
	c.Assert(err, tc.ErrorIsNil)
	relUUID := s.addRelation(c, "default")
	extraUUID := s.addExtraBinding(c, "extra")
	bindings := map[string]network.SpaceName{
		"": s.addSpaceReturningName(c, "beta"),
	}

	// Act: Charm relation will create application endpoint bounded to the default space (beta)
	err = db.Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.insertApplicationEndpointBindings(c.Context(), tx, insertApplicationEndpointsParams{
			appID:    s.appID,
			bindings: bindings,
		})
	})

	// Assert: Should have
	//  - an application endpoint without spacename,
	//  - an application extra endpoint without spacename,
	c.Assert(err, tc.ErrorIsNil)
	c.Check(s.fetchApplicationEndpoints(c), tc.DeepEquals, []applicationEndpoint{
		{
			charmRelationUUID: relUUID,
		},
	})
	c.Check(s.fetchApplicationExtraEndpoints(c), tc.DeepEquals, []applicationEndpoint{
		{
			charmRelationUUID: extraUUID,
		},
	})
}

// TestInsertApplicationEndpointBindOneToBeta verifies that an application
// endpoint can be correctly bound to a specific space.
func (s *applicationEndpointStateSuite) TestInsertApplicationEndpointBindOneToBeta(c *tc.C) {
	// Arrange:
	// - two expected relation
	// - two expected extra endpoint
	// - one of both are bound with a specific space (beta)
	db, err := s.state.DB()
	c.Assert(err, tc.ErrorIsNil)
	relUUID := s.addRelation(c, "default")
	boundUUID := s.addRelation(c, "bound")
	extraUUID := s.addExtraBinding(c, "extra")
	boundExtraUUID := s.addExtraBinding(c, "bound-extra")
	bindings := map[string]network.SpaceName{
		"bound":       s.addSpaceReturningName(c, "beta"),
		"bound-extra": s.addSpaceReturningName(c, "beta-extra"),
	}

	// Act: Charm relation will create application endpoint bounded to the specified space (beta)
	err = db.Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.insertApplicationEndpointBindings(c.Context(), tx, insertApplicationEndpointsParams{
			appID:    s.appID,
			bindings: bindings,
		})
	})

	// Assert: Should have
	//  - two application endpoint one without spacename, one bound to beta
	//  - two application extra endpoint one without spacename, one bound to beta-extra
	c.Assert(err, tc.ErrorIsNil)
	c.Check(s.fetchApplicationEndpoints(c), tc.DeepEquals, []applicationEndpoint{
		{
			charmRelationUUID: relUUID,
		},
		{
			charmRelationUUID: boundUUID,
			spaceName:         "beta",
		},
	})
	c.Check(s.fetchApplicationExtraEndpoints(c), tc.DeepEquals, []applicationEndpoint{
		{
			charmRelationUUID: extraUUID,
		},
		{
			charmRelationUUID: boundExtraUUID,
			spaceName:         "beta-extra",
		},
	})
}

// TestInsertApplicationEndpointBindOneToBetaDefaultedGamma tests the insertion
// of application endpoints with space bindings.
func (s *applicationEndpointStateSuite) TestInsertApplicationEndpointBindOneToBetaDefaultedGamma(c *tc.C) {
	// Arrange:
	// - two expected relation and extra endpoint
	// - override default space
	// - bind one relation to a specific space
	// - bind one extra relation to a specific space
	db, err := s.state.DB()
	c.Assert(err, tc.ErrorIsNil)
	relUUID := s.addRelation(c, "default")
	boundUUID := s.addRelation(c, "bound")
	extraUUID := s.addExtraBinding(c, "extra")
	boundExtraUUID := s.addExtraBinding(c, "bound-extra")
	beta := s.addSpaceReturningName(c, "beta")
	bindings := map[string]network.SpaceName{
		"":            s.addSpaceReturningName(c, "gamma"),
		"bound":       beta,
		"bound-extra": beta,
	}

	// Act: Charm relation will create application endpoint bounded to either the defaulted space
	// or the specified one
	err = db.Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.insertApplicationEndpointBindings(c.Context(), tx, insertApplicationEndpointsParams{
			appID:    s.appID,
			bindings: bindings,
		})
	})

	// Assert: Should have
	//  - two application endpoint one without spacename, one bound to beta
	//  - two application extra endpoint one without spacename, one bound to beta
	c.Assert(err, tc.ErrorIsNil)
	c.Check(s.fetchApplicationEndpoints(c), tc.DeepEquals, []applicationEndpoint{
		{
			charmRelationUUID: relUUID,
		},
		{
			charmRelationUUID: boundUUID,
			spaceName:         "beta",
		},
	})

	c.Check(s.fetchApplicationExtraEndpoints(c), tc.DeepEquals, []applicationEndpoint{
		{
			charmRelationUUID: extraUUID,
		},
		{
			charmRelationUUID: boundExtraUUID,
			spaceName:         "beta",
		},
	})
}

// TestInsertApplicationEndpointRestoreDefaultSpace tests that we can bind a
// endpoint to the application's default space.
func (s *applicationEndpointStateSuite) TestInsertApplicationEndpointRestoreDefaultSpace(c *tc.C) {
	// Arrange:
	// - two expected relation
	// - bind one relation to a specific space
	// - bind one extra relation to a specific space
	db, err := s.state.DB()
	c.Assert(err, tc.ErrorIsNil)
	relUUID := s.addRelation(c, "default")
	boundUUID := s.addRelation(c, "bound")
	extraUUID := s.addExtraBinding(c, "extra")
	boundExtraUUID := s.addExtraBinding(c, "bound-extra")
	s.addSpace(c, "beta")
	bindings := map[string]network.SpaceName{
		"bound":       "",
		"bound-extra": "",
	}

	// Act:
	err = db.Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.insertApplicationEndpointBindings(c.Context(), tx, insertApplicationEndpointsParams{
			appID:    s.appID,
			bindings: bindings,
		})
	})
	c.Assert(err, tc.ErrorIsNil)

	// Assert:
	c.Check(s.fetchApplicationEndpoints(c), tc.SameContents, []applicationEndpoint{
		{
			charmRelationUUID: relUUID,
		},
		{
			charmRelationUUID: boundUUID,
		},
	})
	c.Check(s.fetchApplicationExtraEndpoints(c), tc.SameContents, []applicationEndpoint{
		{
			charmRelationUUID: extraUUID,
		},
		{
			charmRelationUUID: boundExtraUUID,
		},
	})
}

// TestInsertApplicationEndpointUnknownSpace verifies the behavior of inserting
// application endpoints with an unknown space.
//
// Ensures that an error is returned
func (s *applicationEndpointStateSuite) TestInsertApplicationEndpointUnknownSpace(c *tc.C) {
	// Arrange:
	// - One expected relation
	// - bind with an unknown space
	db, err := s.state.DB()
	c.Assert(err, tc.ErrorIsNil)
	s.addRelation(c, "default")
	bindings := map[string]network.SpaceName{
		"": "unknown",
	}

	// Act: Charm relation will create application endpoint bounded to the default space (alpha)
	err = db.Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.insertApplicationEndpointBindings(c.Context(), tx, insertApplicationEndpointsParams{
			appID:    s.appID,
			bindings: bindings,
		})
	})

	// Assert: should fail because unknown is not a valid space
	c.Assert(err, tc.ErrorIs, applicationerrors.SpaceNotFound)
}

// TestInsertApplicationEndpointUnknownRelation verifies that inserting an
// application endpoint with an unknown relation fails.
//
// Ensures that an error is returned
func (s *applicationEndpointStateSuite) TestInsertApplicationEndpointUnknownRelation(c *tc.C) {
	// Arrange:
	// - One expected relation
	// - bind an unexpected relation
	db, err := s.state.DB()
	c.Assert(err, tc.ErrorIsNil)
	s.addRelation(c, "default")
	bindings := map[string]network.SpaceName{
		"unknown": "alpha",
	}

	// Act
	err = db.Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.insertApplicationEndpointBindings(c.Context(), tx, insertApplicationEndpointsParams{
			appID:    s.appID,
			bindings: bindings,
		})
	})

	// Assert: should fail because unknown is not a valid relation
	c.Assert(err, tc.ErrorIs, applicationerrors.CharmRelationNotFound)
}

func (s *applicationEndpointStateSuite) TestMergeApplicationEndpointBindings(c *tc.C) {
	// Arrange:
	// - Two expected relation
	// - Two expected extra endpoint
	// - One of both are bound with a specific space (beta)
	db, err := s.state.DB()
	c.Assert(err, tc.ErrorIsNil)
	relUUID := s.addRelation(c, "default")
	boundUUID := s.addRelation(c, "bound")
	extraUUID := s.addExtraBinding(c, "extra")
	boundExtraUUID := s.addExtraBinding(c, "bound-extra")
	beta := s.addSpaceReturningName(c, "beta")
	gamma := s.addSpaceReturningName(c, "gamma")
	bindings := map[string]network.SpaceName{
		"bound":       beta,
		"bound-extra": beta,
	}

	err = db.Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.insertApplicationEndpointBindings(c.Context(), tx, insertApplicationEndpointsParams{
			appID:    s.appID,
			bindings: bindings,
		})
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(s.fetchApplicationEndpoints(c), tc.DeepEquals, []applicationEndpoint{
		{
			charmRelationUUID: relUUID,
		},
		{
			charmRelationUUID: boundUUID,
			spaceName:         "beta",
		},
	})
	c.Check(s.fetchApplicationExtraEndpoints(c), tc.DeepEquals, []applicationEndpoint{
		{
			charmRelationUUID: extraUUID,
		},
		{
			charmRelationUUID: boundExtraUUID,
			spaceName:         "beta",
		},
	})

	// Act: Bind the endpoints that are already bound to a new space (gamma)
	bindings = map[string]network.SpaceName{
		"bound":       gamma,
		"bound-extra": gamma,
	}
	err = s.state.MergeApplicationEndpointBindings(c.Context(), s.appID, bindings)

	// Assert: Should have
	//  - two application endpoint one without spacename, one bound to gamma
	//  - two application extra endpoint one without spacename, one bound to gamma
	c.Assert(err, tc.ErrorIsNil)
	c.Check(s.fetchApplicationEndpoints(c), tc.DeepEquals, []applicationEndpoint{
		{
			charmRelationUUID: relUUID,
		},
		{
			charmRelationUUID: boundUUID,
			spaceName:         "gamma",
		},
	})

	c.Check(s.fetchApplicationExtraEndpoints(c), tc.DeepEquals, []applicationEndpoint{
		{
			charmRelationUUID: extraUUID,
		},
		{
			charmRelationUUID: boundExtraUUID,
			spaceName:         "gamma",
		},
	})
}

func (s *applicationEndpointStateSuite) TestMergeApplicationEndpointBindingsDefaultSpace(c *tc.C) {
	// Arrange:
	// - Two expected relation
	// - Two expected extra endpoint
	// - One of both are bound with a specific space (beta)
	db, err := s.state.DB()
	c.Assert(err, tc.ErrorIsNil)
	relUUID := s.addRelation(c, "default")
	boundUUID := s.addRelation(c, "bound")
	extraUUID := s.addExtraBinding(c, "extra")
	boundExtraUUID := s.addExtraBinding(c, "bound-extra")
	beta := s.addSpaceReturningName(c, "beta")
	bindings := map[string]network.SpaceName{
		"bound":       beta,
		"bound-extra": beta,
	}

	err = db.Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.insertApplicationEndpointBindings(c.Context(), tx, insertApplicationEndpointsParams{
			appID:    s.appID,
			bindings: bindings,
		})
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(s.fetchApplicationEndpoints(c), tc.DeepEquals, []applicationEndpoint{
		{
			charmRelationUUID: relUUID,
		},
		{
			charmRelationUUID: boundUUID,
			spaceName:         "beta",
		},
	})
	c.Check(s.fetchApplicationExtraEndpoints(c), tc.DeepEquals, []applicationEndpoint{
		{
			charmRelationUUID: extraUUID,
		},
		{
			charmRelationUUID: boundExtraUUID,
			spaceName:         "beta",
		},
	})

	// Act: Bind the endpoints that are already bound to a new space (gamma)
	bindings = map[string]network.SpaceName{
		"bound":       "",
		"bound-extra": "",
	}
	err = s.state.MergeApplicationEndpointBindings(c.Context(), s.appID, bindings)

	// Assert: Should have
	//  - two application endpoint one without spacename, one bound to gamma
	//  - two application extra endpoint one without spacename, one bound to gamma
	c.Assert(err, tc.ErrorIsNil)
	c.Check(s.fetchApplicationEndpoints(c), tc.SameContents, []applicationEndpoint{
		{
			charmRelationUUID: relUUID,
		},
		{
			charmRelationUUID: boundUUID,
		},
	})

	c.Check(s.fetchApplicationExtraEndpoints(c), tc.SameContents, []applicationEndpoint{
		{
			charmRelationUUID: extraUUID,
		},
		{
			charmRelationUUID: boundExtraUUID,
		},
	})
}

func (s *applicationEndpointStateSuite) TestMergeApplicationEndpointBindingsUpdatesAppDefaultSpace(c *tc.C) {
	// Arrange: A non-alpha space
	beta := s.addSpaceReturningName(c, "beta")

	// Act:
	err := s.state.MergeApplicationEndpointBindings(c.Context(), s.appID, map[string]network.SpaceName{
		"": beta,
	})

	// Assert: the application's default space is updated
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.getApplicationDefaultSpace(c), tc.Equals, "beta")
}

func (s *applicationEndpointStateSuite) TestMergeApplicationEndpointBindingsApplicationNotFound(c *tc.C) {
	// Act:
	err := s.state.MergeApplicationEndpointBindings(c.Context(), "bad-uuid", nil)

	// Assert: default space not updated.
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationEndpointStateSuite) TestGetEndpointBindings(c *tc.C) {
	// Arrange: create two application endpoints
	relationName1 := "charmRelation1"
	relationName2 := "charmRelation2"
	relationUUID1 := s.addRelation(c, relationName1)
	relationUUID2 := s.addRelation(c, relationName2)
	spaceUUID1 := s.addSpace(c, "space1")
	spaceUUID2 := s.addSpace(c, "space2")
	s.addApplicationEndpoint(c, spaceUUID1, relationUUID1)
	s.addApplicationEndpoint(c, spaceUUID2, relationUUID2)

	// Arrange application extra endpoints.
	extraName1 := "extra1"
	extraName2 := "extra2"
	extraBindingUUID1 := s.addExtraBinding(c, extraName1)
	extraBindingUUID2 := s.addExtraBinding(c, extraName2)
	spaceUUID3 := s.addSpace(c, "space3")
	spaceUUID4 := s.addSpace(c, "space4")
	s.addApplicationExtraEndpoint(c, spaceUUID3, extraBindingUUID1)
	s.addApplicationExtraEndpoint(c, spaceUUID4, extraBindingUUID2)

	// Arrange: Set the application default space.
	spaceUUID5 := s.addSpace(c, "space5")
	s.setApplicationDefaultSpace(c, spaceUUID5)

	// Act:
	bindings, err := s.state.GetApplicationEndpointBindings(c.Context(), s.appID)

	// Assert:
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(bindings, tc.DeepEquals, map[string]string{
		relationName1: spaceUUID1,
		relationName2: spaceUUID2,
		extraName1:    spaceUUID3,
		extraName2:    spaceUUID4,
		"":            spaceUUID5,
	})
}

// TestGetEndpointBindingsReturnsUnset checks that endpoints with an unset
// space_uuid are included.
func (s *applicationEndpointStateSuite) TestGetEndpointBindingsReturnsUnset(c *tc.C) {
	// Arrange: create two application endpoints
	relationName1 := "charmRelation1"
	relationUUID1 := s.addRelation(c, relationName1)
	s.addApplicationEndpointNullSpace(c, relationUUID1)

	// Arrange application extra endpoints.
	extraName1 := "extra1"
	extraBindingUUID1 := s.addExtraBinding(c, extraName1)
	s.addApplicationExtraEndpointNullSpace(c, extraBindingUUID1)

	// Act:
	bindings, err := s.state.GetApplicationEndpointBindings(c.Context(), s.appID)

	// Assert:
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(bindings, tc.DeepEquals, map[string]string{
		"":            network.AlphaSpaceId,
		relationName1: network.AlphaSpaceId,
		extraName1:    network.AlphaSpaceId,
	})
}

// TestGetEndpointBindingsOnlyDefault checks that when no application endpoints
// are set, the default application bindings is still returned. This is always
// set.
func (s *applicationEndpointStateSuite) TestGetEndpointBindingsOnlyDefault(c *tc.C) {
	// Arrange: Set the application default space.
	spaceUUID := s.addSpace(c, "space")
	s.setApplicationDefaultSpace(c, spaceUUID)

	// Act:
	bindings, err := s.state.GetApplicationEndpointBindings(c.Context(), s.appID)

	// Assert:
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(bindings, tc.HasLen, 1)
	c.Assert(bindings[""], tc.Equals, spaceUUID)
}

func (s *applicationEndpointStateSuite) TestGetEndpointBindingsApplicationNotFound(c *tc.C) {
	// Act:
	_, err := s.state.GetApplicationEndpointBindings(c.Context(), "bad-uuid")

	// Assert:
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationEndpointStateSuite) TestGetApplicationEndpointNames(c *tc.C) {
	// Arrange: create two application endpoints
	s.addRelation(c, "foo")
	s.addRelation(c, "bar")

	// Act:
	eps, err := s.state.GetApplicationEndpointNames(c.Context(), s.appID)

	// Assert:
	c.Assert(err, tc.ErrorIsNil)
	c.Check(eps, tc.SameContents, []string{"foo", "bar"})
}

func (s *applicationEndpointStateSuite) TestGetApplicationEndpointNamesApplicationNoEndpoints(c *tc.C) {
	// Act:
	eps, err := s.state.GetApplicationEndpointNames(c.Context(), s.appID)

	// Assert:
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(eps, tc.HasLen, 0)
}

func (s *applicationEndpointStateSuite) TestGetApplicationEndpointNamesApplicationNotFound(c *tc.C) {
	// Act:
	_, err := s.state.GetApplicationEndpointNames(c.Context(), "bad-uuid")

	// Assert:
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationEndpointStateSuite) addApplicationEndpoint(c *tc.C, spaceUUID, relationUUID string) string {
	endpointUUID := uuid.MustNewUUID().String()
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO application_endpoint (uuid, application_uuid, space_uuid, charm_relation_uuid)
VALUES (?,?,?,?)`, endpointUUID, s.appID, spaceUUID, relationUUID)
		return errors.Capture(err)
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) Failed to add application endpoint: %v", err))
	return endpointUUID
}

func (s *applicationEndpointStateSuite) addApplicationExtraEndpoint(c *tc.C, spaceUUID, extraEndpointUUID string) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO application_extra_endpoint (application_uuid, space_uuid, charm_extra_binding_uuid)
VALUES (?,?,?)`, s.appID, spaceUUID, extraEndpointUUID)
		return errors.Capture(err)
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) Failed to add application extra endpoint: %v", err))
}

func (s *applicationEndpointStateSuite) addApplicationEndpointNullSpace(c *tc.C, relationUUID string) string {
	endpointUUID := uuid.MustNewUUID().String()
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO application_endpoint (uuid, application_uuid, space_uuid, charm_relation_uuid)
VALUES (?,?,?,?)`, endpointUUID, s.appID, nil, relationUUID)
		return errors.Capture(err)
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) Failed to add application endpoint: %v", err))
	return endpointUUID
}

func (s *applicationEndpointStateSuite) addApplicationExtraEndpointNullSpace(c *tc.C, extraEndpointUUID string) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO application_extra_endpoint (application_uuid, space_uuid, charm_extra_binding_uuid)
VALUES (?,?,?)`, s.appID, nil, extraEndpointUUID)
		return errors.Capture(err)
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) Failed to add application extra endpoint: %v", err))
}

// applicationEndpoint represents an association between a charm relation and a
// specific network space. It is used to fetch the state in order to verify what
// has been created
type applicationEndpoint struct {
	charmRelationUUID string
	spaceName         string
}

// fetchApplicationEndpoints retrieves a list of application endpoints from
// the database based on the application UUID.
//
// Returns a slice of applicationEndpoint containing charmRelationUUID and
// spaceName for each endpoint.
func (s *applicationEndpointStateSuite) fetchApplicationEndpoints(c *tc.C) []applicationEndpoint {
	nilEmpty := func(s *string) string {
		if s == nil {
			return ""
		}
		return *s
	}
	var endpoints []applicationEndpoint
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.Query(`
SELECT ae.charm_relation_uuid, s.name
FROM application_endpoint ae
LEFT JOIN space s ON s.uuid=ae.space_uuid
WHERE ae.application_uuid=?
ORDER BY s.name`, s.appID)
		defer func() { _ = rows.Close() }()
		if err != nil {
			return errors.Capture(err)
		}
		for rows.Next() {
			var uuid string
			var name *string
			if err := rows.Scan(&uuid, &name); err != nil {
				return errors.Capture(err)
			}
			endpoints = append(endpoints, applicationEndpoint{
				charmRelationUUID: uuid,
				spaceName:         nilEmpty(name),
			})
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Assert) Failed to fetch endpoints: %v", err))
	return endpoints
}

func (s *applicationEndpointStateSuite) fetchApplicationExtraEndpoints(c *tc.C) []applicationEndpoint {
	var endpoints []applicationEndpoint
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.Query(`
SELECT ae.charm_extra_binding_uuid, s.name
FROM application_extra_endpoint ae
LEFT JOIN space s ON s.uuid=ae.space_uuid
WHERE ae.application_uuid=?
ORDER BY s.name`, s.appID)
		defer func() { _ = rows.Close() }()
		if err != nil {
			return errors.Capture(err)
		}
		for rows.Next() {
			var uuid string
			var name *string
			if err := rows.Scan(&uuid, &name); err != nil {
				return errors.Capture(err)
			}
			endpoints = append(endpoints, applicationEndpoint{
				charmRelationUUID: uuid,
				spaceName:         deptr(name),
			})
		}
		return rows.Err()
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Assert) Failed to fetch extra endpoints: %v", err))
	return endpoints
}

// addSpaceReturningName ensures a space with the given name exists in the database,
// creating it if necessary, and returns its name.
func (s *applicationEndpointStateSuite) addSpaceReturningName(c *tc.C, name string) network.SpaceName {
	s.addSpace(c, name)
	return network.SpaceName(name)
}

// addSpace ensures a space with the given name exists in the database,
// creating it if necessary, and returns its name.
func (s *applicationEndpointStateSuite) addSpace(c *tc.C, name string) string {
	spaceUUID := uuid.MustNewUUID().String()
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO space (uuid, name)
VALUES (?, ?)`, spaceUUID, name)
		return errors.Capture(err)
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) Failed to add a space: %v", err))
	return spaceUUID
}

func (s *applicationEndpointStateSuite) setApplicationDefaultSpace(c *tc.C, spaceUUID string) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
UPDATE application
SET    space_uuid = ? 
WHERE  uuid = ?`, spaceUUID, s.appID)
		return errors.Capture(err)
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) Failed to set application default space: %v", err))
}

// addRelation inserts a new charm relation into the database and returns its generated UUID.
// It asserts that the operation succeeds and fails the test if an error occurs.
func (s *applicationEndpointStateSuite) addRelation(c *tc.C, name string) string {
	relUUID := uuid.MustNewUUID().String()
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO charm_relation (uuid, charm_uuid, scope_id, role_id, name)
VALUES (?,?,0,0,?)`, relUUID, s.charmUUID, name)
		return errors.Capture(err)
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) Failed to add charm relation: %v", err))
	return relUUID
}

// addExtraBinding adds a new extra binding to the charm_extra_binding table
// and returns its generated UUID.
// It asserts that the operation succeeds and fails the test if an error occurs.
func (s *applicationEndpointStateSuite) addExtraBinding(c *tc.C, name string) string {
	bindingUUID := uuid.MustNewUUID().String()
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO charm_extra_binding (uuid, charm_uuid, name) 
VALUES (?,?,?)`, bindingUUID, s.charmUUID, name)
		return errors.Capture(err)
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) Failed to add charm extra binding: %v", err))
	return bindingUUID
}

func (s *applicationEndpointStateSuite) getApplicationDefaultSpace(c *tc.C) string {
	var spaceName string
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(`
SELECT s.name
FROM application a
JOIN space s ON s.uuid=a.space_uuid
WHERE a.uuid=?`, s.appID).Scan(&spaceName)
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Assert) Failed to fetch default space: %v", err))
	return spaceName
}
