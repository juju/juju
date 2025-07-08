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
	networktesting "github.com/juju/juju/core/network/testing"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/life"
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

	c.Check(s.getApplicationDefaultSpace(c, s.appID), tc.Equals, network.SpaceName("beta"))
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

	c.Check(s.getApplicationDefaultSpace(c, s.appID), tc.Equals, network.AlphaSpaceName)
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
	c.Check(s.getApplicationDefaultSpace(c, s.appID), tc.Equals, network.AlphaSpaceName)
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
	betaSpace := s.addSpace(c, "beta")
	gammaSpace := s.addSpace(c, "gamma")
	deltaSpace := s.addSpace(c, "delta")
	bindings := map[string]network.SpaceName{
		"":         "gamma",
		"endpoint": "beta",
		"misc":     "beta",
		"extra":    "beta",
	}
	appUUID := s.createIAASApplicationWithEndpointBindings(c, "fee", life.Alive, bindings)
	unitUUID := s.addUnit(c, "fee/0", appUUID).String()

	netNodeUUID := s.addUnitIPWithSpace(c, unitUUID, betaSpace.String(), "192.168.3.42/24", "192.168.3.0/24")
	s.addUnitAnotherIPWithSpace(c, unitUUID, netNodeUUID, gammaSpace.String(), "10.16.42.9/24", "10.16.42.0/24")
	s.addUnitAnotherIPWithSpace(c, unitUUID, netNodeUUID, deltaSpace.String(), "10.0.15.3/24", "10.0.15.0/24")

	// Update misc and extra endpoints to be mapped to the
	// default space for the application.
	updates := map[string]network.SpaceName{
		"misc":  "delta",
		"extra": "delta",
	}
	expected := map[string]network.SpaceUUID{
		"":          gammaSpace,
		"juju-info": gammaSpace,
		"endpoint":  betaSpace,
		"misc":      deltaSpace,
		"extra":     deltaSpace,
	}

	// Act: Bind the endpoints that are already bound to a new space (gamma)
	err := s.state.MergeApplicationEndpointBindings(c.Context(), appUUID, updates, false)

	// Assert:
	c.Assert(err, tc.ErrorIsNil)
	obtained, err := s.state.GetApplicationEndpointBindings(c.Context(), appUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtained, tc.DeepEquals, expected)
}

func (s *applicationEndpointStateSuite) TestMergeApplicationEndpointBindingsDefaultSpace(c *tc.C) {
	// Arrange:
	betaSpace := s.addSpace(c, "beta")
	gammaSpace := s.addSpace(c, "gamma")
	bindings := map[string]network.SpaceName{
		"":         "gamma",
		"endpoint": "beta",
		"misc":     "beta",
		"extra":    "beta",
	}
	appUUID := s.createIAASApplicationWithEndpointBindings(c, "fee", life.Alive, bindings)
	unitUUID := s.addUnit(c, "fee/0", appUUID).String()

	netNodeUUID := s.addUnitIPWithSpace(c, unitUUID, betaSpace.String(), "192.168.3.42/24", "192.168.3.0/24")
	s.addUnitAnotherIPWithSpace(c, unitUUID, netNodeUUID, gammaSpace.String(), "10.16.42.9/24", "10.16.42.0/24")

	// Update misc and extra endpoints to be mapped to the
	// default space for the application.
	updates := map[string]network.SpaceName{
		"misc":  "",
		"extra": "",
	}
	expected := map[string]network.SpaceUUID{
		"":          gammaSpace,
		"juju-info": gammaSpace,
		"endpoint":  betaSpace,
		"misc":      gammaSpace,
		"extra":     gammaSpace,
	}

	// Act: Bind the endpoints that are already bound to a new space (gamma)
	err := s.state.MergeApplicationEndpointBindings(c.Context(), appUUID, updates, false)

	// Assert:
	c.Assert(err, tc.ErrorIsNil)
	obtained, err := s.state.GetApplicationEndpointBindings(c.Context(), appUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtained, tc.DeepEquals, expected)
}

func (s *applicationEndpointStateSuite) TestMergeApplicationEndpointBindingsApplicationNotFound(c *tc.C) {
	// Act:
	err := s.state.MergeApplicationEndpointBindings(c.Context(), "bad-uuid", nil, false)

	// Assert:
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationEndpointStateSuite) TestGetAllEndpointBindingsFreshModel(c *tc.C) {
	// Act:
	bindings, err := s.state.GetAllEndpointBindings(c.Context())

	// Assert: Only the pre-made application should be returned.
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(bindings, tc.DeepEquals, map[string]map[string]string{
		"foo": {
			"": network.AlphaSpaceName.String(),
		},
	})
}

func (s *applicationEndpointStateSuite) TestGetAllEndpointBindingsDefault(c *tc.C) {
	// Arrange:
	// - create two applications
	//   - by default, they gain 3 endpoints (juju-info, endpoint & misc) & an
	//     extra binding (extra)

	s.createIAASApplication(c, "fii", life.Alive)
	app1rel1Name := "juju-info"
	app1rel2Name := "endpoint"
	app1rel3Name := "misc"

	app1Extra1Name := "extra"

	s.createIAASApplication(c, "bar", life.Alive)
	app2rel1Name := "juju-info"
	app2rel2Name := "endpoint"
	app2rel3Name := "misc"

	app2Extra1Name := "extra"

	// Act:
	allEndpointBindings, err := s.state.GetAllEndpointBindings(c.Context())

	// Assert:
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(allEndpointBindings, tc.DeepEquals, map[string]map[string]string{
		"foo": {
			"": network.AlphaSpaceName.String(),
		},
		"fii": {
			"":             network.AlphaSpaceName.String(),
			app1rel1Name:   network.AlphaSpaceName.String(),
			app1rel2Name:   network.AlphaSpaceName.String(),
			app1rel3Name:   network.AlphaSpaceName.String(),
			app1Extra1Name: network.AlphaSpaceName.String(),
		},
		"bar": {
			"":             network.AlphaSpaceName.String(),
			app2rel1Name:   network.AlphaSpaceName.String(),
			app2rel2Name:   network.AlphaSpaceName.String(),
			app2rel3Name:   network.AlphaSpaceName.String(),
			app2Extra1Name: network.AlphaSpaceName.String(),
		},
	})
}

func (s *applicationEndpointStateSuite) TestGetAllEndpointBindings(c *tc.C) {
	// Arrange:
	// - add some spaces
	// - create two applications
	//   - by default, they gain 3 endpoints (juju-info, endpoint & misc) & an
	//     extra binding (extra)
	// - bind some of the endpoints to new spaces
	_ = s.addSpace(c, "beta")
	_ = s.addSpace(c, "gamma")
	_ = s.addSpace(c, "delta")

	_ = s.createIAASApplicationWithEndpointBindings(c, "fii", life.Alive,
		map[string]network.SpaceName{
			"endpoint": "beta",
			"misc":     "gamma",
			"extra":    "delta",
		})

	_ = s.createIAASApplicationWithEndpointBindings(c, "bar", life.Alive,
		map[string]network.SpaceName{
			"endpoint": "beta",
			"misc":     "delta",
		})

	// Act:
	allEndpointBindings, err := s.state.GetAllEndpointBindings(c.Context())

	// Assert:
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(allEndpointBindings, tc.DeepEquals, map[string]map[string]string{
		"foo": {
			"": network.AlphaSpaceName.String(),
		},
		"fii": {
			"":          network.AlphaSpaceName.String(),
			"juju-info": network.AlphaSpaceName.String(),
			"endpoint":  "beta",
			"misc":      "gamma",
			"extra":     "delta",
		},
		"bar": {
			"":          network.AlphaSpaceName.String(),
			"juju-info": network.AlphaSpaceName.String(),
			"endpoint":  "beta",
			"misc":      "delta",
			"extra":     network.AlphaSpaceName.String(),
		},
	})
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
	c.Assert(bindings, tc.DeepEquals, map[string]network.SpaceUUID{
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
	c.Assert(bindings, tc.DeepEquals, map[string]network.SpaceUUID{
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

func (s *applicationEndpointStateSuite) TestGetApplicationsBoundToSpaceDefaultBindings(c *tc.C) {
	// Arrange:
	// - some extra applications
	// - an extra space
	s.createIAASApplication(c, "fee", life.Alive)
	s.createIAASApplication(c, "bar", life.Alive)
	s.createCAASApplication(c, "baz", life.Alive)

	// Arrange: create some endpoints
	betaUUID := s.addSpace(c, "beta")

	// Act: Get the applications bound to the spaces
	alphaApps, err := s.state.GetApplicationsBoundToSpace(c.Context(), network.AlphaSpaceId.String())
	c.Assert(err, tc.ErrorIsNil)

	betaApps, err := s.state.GetApplicationsBoundToSpace(c.Context(), betaUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	// Assert: all apps are only bound to alpha
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(alphaApps, tc.SameContents, []string{"fee", "bar", "baz"})
	c.Assert(betaApps, tc.SameContents, []string{})
}

func (s *applicationEndpointStateSuite) TestGetApplicationsBoundToSpace(c *tc.C) {
	// Arrange:
	// - some extra applications
	// - some extra spaces
	// - merge some bindings
	//   - all fee endpoint are bound to beta, and extra are bound to gamma
	//   - a single endpoint on bar is bound to gamma
	betaUUID := s.addSpace(c, "beta")
	gammaUUID := s.addSpace(c, "gamma")

	_ = s.createIAASApplicationWithEndpointBindings(c, "fee", life.Alive,
		map[string]network.SpaceName{
			"juju-info": network.SpaceName("beta"),
			"endpoint":  network.SpaceName("beta"),
			"misc":      network.SpaceName("beta"),
			"extra":     network.SpaceName("gamma"),
		})
	_ = s.createIAASApplicationWithEndpointBindings(c, "bar", life.Alive,
		map[string]network.SpaceName{
			"endpoint": network.SpaceName("gamma"),
		})
	s.createCAASApplication(c, "baz", life.Alive)

	// Act:
	alphaApps, err := s.state.GetApplicationsBoundToSpace(c.Context(), network.AlphaSpaceId.String())
	c.Assert(err, tc.ErrorIsNil)

	betaApps, err := s.state.GetApplicationsBoundToSpace(c.Context(), betaUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	gammaApps, err := s.state.GetApplicationsBoundToSpace(c.Context(), gammaUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	// Assert: all apps are only bound to alpha
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(alphaApps, tc.SameContents, []string{"bar", "baz"})
	c.Assert(betaApps, tc.SameContents, []string{"fee"})
	c.Assert(gammaApps, tc.SameContents, []string{"fee", "bar"})
}

func (s *applicationEndpointStateSuite) TestValidateEndpointBindingsForApplication(c *tc.C) {
	// Arrange:
	betaSpace := s.addSpace(c, "beta").String()

	appUUID := s.createIAASApplicationWithEndpointBindings(c, "fee", life.Alive,
		map[string]network.SpaceName{
			"endpoint": "beta",
			"misc":     "beta",
			"extra":    "beta",
		})
	unitUUID := s.addUnit(c, "fee/0", appUUID).String()

	gammaSpace := s.addSpace(c, "gamma").String()
	netNodeUUID := s.addUnitIPWithSpace(c, unitUUID, betaSpace, "192.168.3.42/24", "192.168.3.0/24")
	s.addUnitAnotherIPWithSpace(c, unitUUID, netNodeUUID, gammaSpace, "10.16.42.9/24", "10.16.42.0/24")

	// Act:
	// Argument includes bindings which are and are not changing
	// to mimic real world input.
	err := s.state.MergeApplicationEndpointBindings(c.Context(), appUUID,
		map[string]network.SpaceName{
			"endpoint": "beta",
			"misc":     "gamma",
			"extra":    "gamma",
		}, false,
	)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *applicationEndpointStateSuite) TestMergeApplicationEndpointBindingsUpdatesAppDefaultSpace(c *tc.C) {
	// Arrange:
	// An application's default space starts as "alpha" if not
	// specified.
	betaSpace := s.addSpace(c, "beta").String()

	appUUID := s.createIAASApplicationWithEndpointBindings(c, "fee", life.Alive,
		map[string]network.SpaceName{
			"endpoint": "beta",
			"misc":     "beta",
			"extra":    "beta",
		})
	unitUUID := s.addUnit(c, "fee/0", appUUID).String()

	deltaSpace := s.addSpace(c, "delta").String()
	netNodeUUID := s.addUnitIPWithSpace(c, unitUUID, betaSpace, "192.168.3.42/24", "192.168.3.0/24")
	s.addUnitAnotherIPWithSpace(c, unitUUID, netNodeUUID, deltaSpace, "10.16.42.9/24", "10.16.42.0/24")

	// Act
	err := s.state.MergeApplicationEndpointBindings(c.Context(), appUUID,
		map[string]network.SpaceName{
			"":      "beta",
			"extra": "delta",
		}, false,
	)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.getApplicationDefaultSpace(c, appUUID), tc.Equals, network.SpaceName("beta"))
}

func (s *applicationEndpointStateSuite) TestValidateEndpointBindingsForApplicationOneUnitNotInSpace(c *tc.C) {
	// Arrange:
	betaSpace := s.addSpace(c, "beta").String()

	appUUID := s.createIAASApplicationWithEndpointBindings(c, "fee", life.Alive,
		map[string]network.SpaceName{
			"endpoint": "beta",
			"misc":     "beta",
			"extra":    "beta",
		})

	unitUUIDOne := s.addUnit(c, "fee/0", appUUID).String()
	gammaSpace := s.addSpace(c, "gamma").String()
	netNodeUUIDOne := s.addUnitIPWithSpace(c, unitUUIDOne, betaSpace, "192.168.3.42/24", "192.168.3.0/24")
	s.addUnitAnotherIPWithSpace(c, unitUUIDOne, netNodeUUIDOne, gammaSpace, "10.0.42.8/24", "10.0.42.0/24")

	unitUUIDTwo := s.addUnit(c, "fee/1", appUUID).String()
	deltaSpace := s.addSpace(c, "delta").String()
	_ = s.addUnitIPWithSpace(c, unitUUIDTwo, deltaSpace, "10.7.3.43/24", "10.7.3.0/24")

	// Act
	err := s.state.MergeApplicationEndpointBindings(c.Context(), appUUID,
		map[string]network.SpaceName{
			"misc":  "gamma",
			"extra": "gamma",
		}, false,
	)

	// Assert
	c.Assert(err, tc.ErrorMatches, `validating endpoint bindings: unit "fee/1" is not in every space: .*`)
}

func (s *applicationEndpointStateSuite) TestValidateEndpointBindingsForApplicationFail(c *tc.C) {
	// Arrange:
	betaSpace := s.addSpace(c, "beta").String()

	appUUID := s.createIAASApplicationWithEndpointBindings(c, "fee", life.Alive,
		map[string]network.SpaceName{
			"endpoint": "beta",
			"misc":     "beta",
			"extra":    "beta",
		})
	unitUUID := s.addUnit(c, "fee/0", appUUID).String()

	gammaSpace := s.addSpace(c, "gamma").String()
	netNodeUUID := s.addUnitIPWithSpace(c, unitUUID, betaSpace, "192.168.3.42/24", "192.168.3.0/24")
	s.addUnitAnotherIPWithSpace(c, unitUUID, netNodeUUID, gammaSpace, "10.0.42.8/24", "10.0.42.0/24")
	_ = s.addSpace(c, "delta")

	// Act:
	// Unit has no ip address in space gamma.
	err := s.state.MergeApplicationEndpointBindings(c.Context(), appUUID,
		map[string]network.SpaceName{"misc": "delta"}, false,
	)

	// Assert
	c.Assert(err, tc.ErrorMatches, `validating endpoint bindings: unit "fee/0" is not in every space: .*`)
}

func (s *applicationEndpointStateSuite) TestValidateEndpointBindingsForApplicationForce(c *tc.C) {
	// Arrange:
	// Setup for failure where using the force flag allows for success.
	betaSpace := s.addSpace(c, "beta").String()

	appUUID := s.createIAASApplicationWithEndpointBindings(c, "fee", life.Alive,
		map[string]network.SpaceName{
			"endpoint": network.SpaceName("beta"),
			"misc":     network.SpaceName("beta"),
			"extra":    network.SpaceName("beta"),
		})
	unitUUID := s.addUnit(c, "fee/0", appUUID).String()

	gammaSpace := s.addSpace(c, "gamma").String()
	netNodeUUID := s.addUnitIPWithSpace(c, unitUUID, betaSpace, "192.168.3.42/24", "192.168.3.0/24")
	s.addUnitAnotherIPWithSpace(c, unitUUID, netNodeUUID, gammaSpace, "10.0.42.8/24", "10.0.42.0/24")
	_ = s.addSpace(c, "delta")

	// Act:
	// Unit has no ip address in space gamma.
	err := s.state.MergeApplicationEndpointBindings(c.Context(), appUUID,
		map[string]network.SpaceName{"misc": "delta"}, true,
	)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *applicationEndpointStateSuite) TestValidateEndpointBindingsForApplicationMissingSpace(c *tc.C) {
	// Act
	err := s.state.MergeApplicationEndpointBindings(c.Context(), s.appID,
		map[string]network.SpaceName{"misc": "delta"}, false,
	)

	// Assert
	c.Assert(err, tc.ErrorIs, applicationerrors.SpaceNotFound)
}

func (s *applicationEndpointStateSuite) TestValidateEndpointBindingsForApplicationMissingEndpoint(c *tc.C) {
	// Arrange
	appUUID := s.createIAASApplication(c, "fee", life.Alive)
	s.addSpace(c, "delta")

	// Act:
	err := s.state.MergeApplicationEndpointBindings(c.Context(), appUUID,
		map[string]network.SpaceName{"not-an-endpoint": "delta"}, false,
	)

	// Assert
	c.Assert(err, tc.ErrorMatches, "validating endpoint bindings: validating endpoints exist: one or more of the provided endpoints .* do not exist")
}

func (s *applicationEndpointStateSuite) TestValidateEndpointBindingsForApplicationUnitNoSpaces(c *tc.C) {
	// Arrange:
	appUUID := s.createIAASApplication(c, "fee", life.Alive)
	_ = s.addUnit(c, "fee/0", appUUID).String()
	s.addSpace(c, "gamma")

	// Act
	err := s.state.MergeApplicationEndpointBindings(c.Context(), appUUID,
		map[string]network.SpaceName{"misc": "gamma"}, false,
	)

	// Assert
	c.Assert(err, tc.ErrorIs, applicationerrors.SpaceNotFound)
}

func (s *applicationEndpointStateSuite) addApplicationEndpoint(c *tc.C, spaceUUID network.SpaceUUID, relationUUID string) string {
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

func (s *applicationEndpointStateSuite) addApplicationExtraEndpoint(c *tc.C, spaceUUID network.SpaceUUID, extraEndpointUUID string) {
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
func (s *applicationEndpointStateSuite) addSpace(c *tc.C, name string) network.SpaceUUID {
	spaceUUID := networktesting.GenSpaceUUID(c)
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO space (uuid, name)
VALUES (?, ?)`, spaceUUID, name)
		return errors.Capture(err)
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) Failed to add a space: %v", err))
	return spaceUUID
}

func (s *applicationEndpointStateSuite) setApplicationDefaultSpace(c *tc.C, spaceUUID network.SpaceUUID) {
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

func (s *applicationEndpointStateSuite) getApplicationDefaultSpace(c *tc.C, appID coreapplication.ID) network.SpaceName {
	var spaceName network.SpaceName
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(`
SELECT s.name
FROM application a
JOIN space s ON s.uuid=a.space_uuid
WHERE a.uuid=?`, appID).Scan(&spaceName)
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Assert) Failed to fetch default space: %v", err))
	return spaceName
}

func (s *applicationEndpointStateSuite) addUnitIPWithSpace(c *tc.C, unitUUID, spaceUUID, ip, subnetCIDR string) string {
	netNodeUUID := uuid.MustNewUUID().String()
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		insertNetNode := `INSERT INTO net_node (uuid) VALUES (?)`
		_, err := tx.ExecContext(ctx, insertNetNode, netNodeUUID)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	s.addUnitAnotherIPWithSpace(c, unitUUID, netNodeUUID, spaceUUID, ip, subnetCIDR)
	return netNodeUUID
}

func (s *applicationEndpointStateSuite) addUnitAnotherIPWithSpace(c *tc.C, unitUUID, netNodeUUID, spaceUUID, ip, subnetCIDR string) {
	ipAddrUUID := uuid.MustNewUUID().String()
	lldUUID := uuid.MustNewUUID().String()
	subnetUUID := uuid.MustNewUUID().String()
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		updateUnit := `UPDATE unit SET net_node_uuid = ? WHERE uuid = ?`
		_, err := tx.ExecContext(ctx, updateUnit, netNodeUUID, unitUUID)
		if err != nil {
			return errors.Errorf("failed to update unit net_node_uuid: %v", err)
		}
		insertLLD := `INSERT INTO link_layer_device (uuid, net_node_uuid, name, mtu, mac_address, device_type_id, virtual_port_type_id) VALUES (?, ?, ?, ?, ?, ?, ?)`
		_, err = tx.ExecContext(ctx, insertLLD, lldUUID, netNodeUUID, "lld-name", 1500, "00:11:22:33:44:55", 0, 0)
		if err != nil {
			return errors.Errorf("failed to insert link_layer_device: %v", err)
		}
		insertSubnet := `INSERT INTO subnet (uuid, cidr, space_uuid) VALUES (?, ?, ?)`
		_, err = tx.ExecContext(ctx, insertSubnet, subnetUUID, subnetCIDR, spaceUUID)
		if err != nil {
			return errors.Errorf("failed to insert subnet: %v", err)
		}
		insertIPAddress := `INSERT INTO ip_address (uuid, device_uuid, address_value, net_node_uuid, type_id, scope_id, origin_id, config_type_id, subnet_uuid) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
		_, err = tx.ExecContext(ctx, insertIPAddress, ipAddrUUID, lldUUID, ip, netNodeUUID, 0, 0, 0, 0, subnetUUID)
		if err != nil {
			return errors.Errorf("failed to insert ip_address: %v", err)
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf(unitUUID+" "+netNodeUUID+" "+spaceUUID+" "+ip+" "+subnetCIDR))
}
