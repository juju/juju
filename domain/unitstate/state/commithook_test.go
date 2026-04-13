// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"testing"

	"github.com/canonical/sqlair"
	"github.com/juju/tc"

	"github.com/juju/juju/core/network"
	corerelation "github.com/juju/juju/core/relation"
	coreunit "github.com/juju/juju/core/unit"
	coreunittesting "github.com/juju/juju/core/unit/testing"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/deployment/charm"
	domainrelation "github.com/juju/juju/domain/relation"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	"github.com/juju/juju/domain/unitstate/internal"
	"github.com/juju/juju/internal/uuid"
)

type commitHookSuite struct {
	commitHookBaseSuite
}

func TestCommitHookSuite(t *testing.T) {
	tc.Run(t, &commitHookSuite{})
}

func (s *commitHookSuite) TestCommitHookChanges(c *tc.C) {
	// Arrange
	arg := internal.CommitHookChangesArg{
		UnitUUID:           coreunit.UUID(s.unitUUID),
		UpdateNetworkInfo:  true,
		RelationSettings:   nil,
		OpenPorts:          nil,
		ClosePorts:         nil,
		CharmState:         nil,
		SecretCreates:      nil,
		TrackLatestSecrets: nil,
		SecretUpdates:      nil,
		SecretGrants:       nil,
		SecretRevokes:      nil,
		SecretDeletes:      nil,
	}

	// Act
	err := s.state.CommitHookChanges(c.Context(), arg)

	// Assert
	c.Assert(err, tc.IsNil)
}

func (s *commitHookSuite) TestUpdateCharmState(c *tc.C) {
	ctx := c.Context()

	// Arrange
	// Set some initial state. This should be overwritten.
	s.addUnitStateCharm(c, "one-key", "one-val")

	expState := map[string]string{
		"two-key":   "two-val",
		"three-key": "three-val",
	}

	// Act
	err := s.TxnRunner().Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		unit := entityUUID{UUID: s.unitUUID}
		return s.state.updateCharmState(ctx, tx, unit, &expState)
	})
	c.Assert(err, tc.ErrorIsNil)

	// Assert
	gotState := make(map[string]string)
	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		q := "SELECT key, value FROM unit_state_charm WHERE unit_uuid = ?"
		rows, err := tx.QueryContext(ctx, q, s.unitUUID)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()

		for rows.Next() {
			var k, v string
			if err := rows.Scan(&k, &v); err != nil {
				return err
			}
			gotState[k] = v
		}
		return rows.Err()
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Check(gotState, tc.DeepEquals, expState)
}

func (s *commitHookSuite) TestUpdateCharmStateEmpty(c *tc.C) {
	ctx := c.Context()

	// Act - use a bad unit uuid to ensure the test fails if setUnitStateCharm
	// is called.
	err := s.TxnRunner().Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		unit := entityUUID{UUID: "bad-unit-uuid"}
		return s.state.updateCharmState(ctx, tx, unit, nil)
	})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *commitHookSuite) TestCommitHookRelationSettings(c *tc.C) {
	// Arrange: Add relation with one endpoint.
	endpoint1 := domainrelation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RoleProvider,
			Interface: "database",
			Scope:     charm.ScopeContainer,
		},
	}
	charmRelationUUID1 := s.addCharmRelation(c, s.fakeCharmUUID1, endpoint1.Relation)
	applicationEndpointUUID1 := s.addApplicationEndpoint(c, s.fakeApplicationUUID1, charmRelationUUID1)
	relationUUID := s.addRelation(c)
	relationEndpointUUID1 := s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID1)

	// Arrange: Add a unit to the relation.
	unitName := coreunittesting.GenNewName(c, "app/7")
	unitUUID := s.addUnit(c, unitName, s.fakeApplicationUUID1, s.fakeCharmUUID1)
	relationUnitUUID := s.addRelationUnit(c, unitUUID, relationEndpointUUID1)

	// Arrange: setup the method input
	appSettings := map[string]string{
		"key2": "value2",
		"key3": "value3",
	}
	unitSettings := map[string]string{
		"key1": "value1",
		"key3": "value3",
	}
	arg := internal.CommitHookChangesArg{
		UnitUUID: unitUUID,
		RelationSettings: []internal.RelationSettings{{
			RelationUUID:   relationUUID,
			ApplicationSet: appSettings,
			UnitSet:        unitSettings,
		}},
	}

	// Act
	err := s.state.CommitHookChanges(c.Context(), arg)

	// Assert
	c.Assert(err, tc.ErrorIsNil)

	foundAppSettings := s.getRelationApplicationSettings(c, relationEndpointUUID1)
	c.Check(foundAppSettings, tc.DeepEquals, appSettings)
	foundUnitSettings := s.getRelationUnitSettings(c, relationUnitUUID)
	c.Check(foundUnitSettings, tc.DeepEquals, unitSettings)
}

func (s *commitHookSuite) TestGetUnitUUIDByName(c *tc.C) {
	// Arrange
	spaceUUID := s.addSpace(c)

	charmUUID := s.addCharm(c)
	appUUID := s.addApplication(c, charmUUID, "testname", spaceUUID)
	unitName := coreunit.Name("testname/0")
	expectedUUID := s.addUnit(c, unitName, appUUID, charmUUID)

	// Act
	unitUUID, err := s.state.GetUnitUUIDByName(c.Context(), unitName)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(unitUUID, tc.Equals, expectedUUID)
}

func (s *commitHookSuite) TestGetUnitUUIDByNameNotFound(c *tc.C) {
	_, err := s.state.GetUnitUUIDByName(c.Context(), "foo")
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *commitHookSuite) TestEnsureCommitHookChangesUUIDsUnitNotFound(c *tc.C) {
	// Arrange
	arg := internal.CommitHookChangesArg{}

	// Act
	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.ensureCommitHookChangesUUIDs(ctx, tx, arg)
	})

	// Assert
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *commitHookSuite) TestEnsureCommitHookChangesRelationsNotFound(c *tc.C) {
	// Arrange: add a unit
	charmUUID := s.addCharm(c)
	appUUID := s.addApplication(c, charmUUID, "testname", network.AlphaSpaceId.String())
	unitName := coreunit.Name("testname/0")
	unitUUID := s.addUnit(c, unitName, appUUID, charmUUID)

	// Arrange: setup the method input with a non-existent relation uuid
	arg := internal.CommitHookChangesArg{
		UnitUUID: unitUUID,
		RelationSettings: []internal.RelationSettings{{
			RelationUUID: tc.Must(c, corerelation.NewUUID),
		}},
	}

	// Act
	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.ensureCommitHookChangesUUIDs(ctx, tx, arg)
	})

	// Assert
	c.Assert(err, tc.ErrorIs, relationerrors.RelationNotFound)
}

func (s *commitHookSuite) addSpace(c *tc.C) string {
	spaceUUID := uuid.MustNewUUID().String()
	s.query(c, `INSERT INTO space (uuid, name) VALUES (?, ?)`,
		spaceUUID, spaceUUID)
	return spaceUUID
}
