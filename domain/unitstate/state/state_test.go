// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	stdtesting "testing"

	"github.com/canonical/sqlair"
	"github.com/juju/tc"

	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/life"
	removalstate "github.com/juju/juju/domain/removal/state/model"
	"github.com/juju/juju/domain/unitstate"
	"github.com/juju/juju/internal/uuid"
)

type stateSuite struct {
	baseSuite
}

func TestStateSuite(t *stdtesting.T) {
	tc.Run(t, &stateSuite{})
}

func (s *stateSuite) TestSetUnitState(c *tc.C) {
	s.addRelation(c, 1, life.Alive)

	agentState := unitstate.UnitState{
		Name:          s.unitName,
		CharmState:    new(map[string]string{"one-key": "one-value"}),
		UniterState:   new("some-uniter-state-yaml"),
		RelationState: new(map[int]string{1: "one-value"}),
		StorageState:  new("some-storage-state-yaml"),
		SecretState:   new("some-secret-state-yaml"),
	}
	err := s.state.SetUnitState(c.Context(), agentState)
	c.Assert(err, tc.ErrorIsNil)

	expectedAgentState := unitstate.RetrievedUnitState{
		CharmState:    *agentState.CharmState,
		UniterState:   *agentState.UniterState,
		RelationState: *agentState.RelationState,
		StorageState:  *agentState.StorageState,
		SecretState:   *agentState.SecretState,
	}

	state, err := s.state.GetUnitState(c.Context(), s.unitName)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(state, tc.DeepEquals, expectedAgentState)
}

func (s *stateSuite) TestSetUnitStateFiltersMissingRelations(c *tc.C) {
	// Departure hooks still need checkpoints for relations being removed.
	s.addRelation(c, 0, life.Alive)
	s.addRelation(c, 1, life.Dying)
	s.addRelation(c, 2, life.Dead)
	s.query(c, `INSERT INTO unit_state_relation (unit_uuid, "key", value) VALUES (?, ?, ?)`,
		s.unitUUID, "3", "old checkpoint")

	agentState := unitstate.UnitState{
		Name: s.unitName,
		RelationState: new(map[int]string{
			0: "alive checkpoint",
			1: "dying checkpoint",
			2: "dead checkpoint",
			3: "missing checkpoint",
		}),
		CharmState:   new(map[string]string{"key": "charm state"}),
		UniterState:  new("uniter state"),
		StorageState: new("storage state"),
		SecretState:  new("secret state"),
	}
	err := s.state.SetUnitState(c.Context(), agentState)
	c.Assert(err, tc.ErrorIsNil)

	state, err := s.state.GetUnitState(c.Context(), s.unitName)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(state, tc.DeepEquals, unitstate.RetrievedUnitState{
		RelationState: map[int]string{
			0: "alive checkpoint",
			1: "dying checkpoint",
			2: "dead checkpoint",
		},
		CharmState:   *agentState.CharmState,
		UniterState:  *agentState.UniterState,
		StorageState: *agentState.StorageState,
		SecretState:  *agentState.SecretState,
	})
	// Filtering must not change the caller's checkpoint map.
	c.Check(*agentState.RelationState, tc.DeepEquals, map[int]string{
		0: "alive checkpoint",
		1: "dying checkpoint",
		2: "dead checkpoint",
		3: "missing checkpoint",
	})
}

func (s *stateSuite) TestSetUnitStateAfterRelationDeletion(c *tc.C) {
	relationUUID := s.addRelation(c, 42, life.Dying)
	agentState := unitstate.UnitState{
		Name:          s.unitName,
		RelationState: new(map[int]string{42: "checkpoint"}),
	}
	err := s.state.SetUnitState(c.Context(), agentState)
	c.Assert(err, tc.ErrorIsNil)

	removal := removalstate.NewState(s.TxnRunnerFactory(), s.state.logger)
	err = removal.DeleteRelation(c.Context(), relationUUID)
	c.Assert(err, tc.ErrorIsNil)

	state, err := s.state.GetUnitState(c.Context(), s.unitName)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(state.RelationState, tc.HasLen, 0)

	// A late write from the unit must not restore the deleted checkpoint,
	// even when every relation in its snapshot has disappeared.
	agentState.UniterState = new("updated uniter state")
	err = s.state.SetUnitState(c.Context(), agentState)
	c.Assert(err, tc.ErrorIsNil)

	state, err = s.state.GetUnitState(c.Context(), s.unitName)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(state, tc.DeepEquals, unitstate.RetrievedUnitState{
		UniterState: *agentState.UniterState,
	})
}

func (s *stateSuite) TestSetUnitStateJustUniterState(c *tc.C) {
	agentState := unitstate.UnitState{
		Name:        s.unitName,
		UniterState: new("some-uniter-state-yaml"),
	}
	s.state.SetUnitState(c.Context(), agentState)

	expectedAgentState := unitstate.RetrievedUnitState{
		UniterState: *agentState.UniterState,
	}

	state, err := s.state.GetUnitState(c.Context(), s.unitName)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(state, tc.DeepEquals, expectedAgentState)
}

func (s *stateSuite) TestGetUnitStateUnitNotFound(c *tc.C) {
	_, err := s.state.GetUnitState(c.Context(), "bad-uuid")
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *stateSuite) TestEnsureUnitStateRecord(c *tc.C) {
	ctx := c.Context()

	err := s.TxnRunner().Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.ensureUnitStateRecord(ctx, tx, entityUUID{UUID: s.unitUUID})
	})
	c.Assert(err, tc.ErrorIsNil)

	s.checkUnitUUID(c, s.unitUUID)

	// Running again makes no change.
	err = s.TxnRunner().Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.ensureUnitStateRecord(ctx, tx, entityUUID{UUID: s.unitUUID})
	})
	c.Assert(err, tc.ErrorIsNil)

	s.checkUnitUUID(c, s.unitUUID)
}

func (s *stateSuite) TestUpdateUnitStateUniter(c *tc.C) {
	ctx := c.Context()
	expState := "some uniter state YAML"

	err := s.TxnRunner().Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := s.state.ensureUnitStateRecord(ctx, tx, entityUUID{UUID: s.unitUUID}); err != nil {
			return err
		}
		return s.state.updateUnitStateUniter(ctx, tx, entityUUID{UUID: s.unitUUID}, expState)
	})
	c.Assert(err, tc.ErrorIsNil)

	var gotState string
	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		q := "SELECT uniter_state FROM unit_state where unit_uuid = ?"
		return tx.QueryRowContext(ctx, q, s.unitUUID).Scan(&gotState)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gotState, tc.Equals, expState)
}

func (s *stateSuite) TestUpdateUnitStateStorage(c *tc.C) {
	ctx := c.Context()
	expState := "some storage state YAML"

	err := s.TxnRunner().Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := s.state.ensureUnitStateRecord(ctx, tx, entityUUID{UUID: s.unitUUID}); err != nil {
			return err
		}
		return s.state.updateUnitStateStorage(ctx, tx, entityUUID{UUID: s.unitUUID}, expState)
	})
	c.Assert(err, tc.ErrorIsNil)

	var gotState string
	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		q := "SELECT storage_state FROM unit_state where unit_uuid = ?"
		return tx.QueryRowContext(ctx, q, s.unitUUID).Scan(&gotState)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gotState, tc.Equals, expState)
}

func (s *stateSuite) TestUpdateUnitStateSecret(c *tc.C) {
	ctx := c.Context()
	expState := "some secret state YAML"

	err := s.TxnRunner().Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := s.state.ensureUnitStateRecord(ctx, tx, entityUUID{UUID: s.unitUUID}); err != nil {
			return err
		}
		return s.state.updateUnitStateSecret(ctx, tx, entityUUID{UUID: s.unitUUID}, expState)
	})
	c.Assert(err, tc.ErrorIsNil)

	var gotState string
	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		q := "SELECT secret_state FROM unit_state where unit_uuid = ?"
		return tx.QueryRowContext(ctx, q, s.unitUUID).Scan(&gotState)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gotState, tc.Equals, expState)
}

func (s *stateSuite) TestUpdateUnitStateCharm(c *tc.C) {
	ctx := c.Context()

	// Set some initial state. This should be overwritten.
	s.addUnitStateCharm(c, "one-key", "one-val")

	expState := map[string]string{
		"two-key":   "two-val",
		"three-key": "three-val",
	}

	err := s.TxnRunner().Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.setUnitStateCharm(ctx, tx, entityUUID{UUID: s.unitUUID}, expState)
	})
	c.Assert(err, tc.ErrorIsNil)

	gotState := make(map[string]string)
	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		gotState = map[string]string{}

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

func (s *stateSuite) TestUpdateUnitStateCharmEmptyMap(c *tc.C) {
	ctx := c.Context()

	// Set some initial state. This should be deleted when we set empty state.
	s.addUnitStateCharm(c, "one-key", "one-val")

	err := s.TxnRunner().Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.setUnitStateCharm(ctx, tx, entityUUID{UUID: s.unitUUID}, map[string]string{})
	})
	c.Assert(err, tc.ErrorIsNil)

	var rowCount int
	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		rowCount = 0

		q := "SELECT key, value FROM unit_state_charm WHERE unit_uuid = ?"
		rows, err := tx.QueryContext(ctx, q, s.unitUUID)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()

		for rows.Next() {
			rowCount++
		}
		return rows.Err()
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Check(rowCount, tc.DeepEquals, 0)
}

func (s *stateSuite) TestUpdateUnitStateRelation(c *tc.C) {
	ctx := c.Context()
	s.addRelation(c, 1, life.Alive)
	s.addRelation(c, 2, life.Alive)
	s.addRelation(c, 3, life.Alive)

	// Set some initial state. This should be overwritten.
	s.query(c, `INSERT INTO unit_state_relation (unit_uuid, "key", value) VALUES (?, ?, ?)`,
		s.unitUUID, "1", "one-val")

	expState := map[int]string{
		2: "two-val",
		3: "three-val",
	}

	err := s.TxnRunner().Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.setUnitStateRelation(ctx, tx, entityUUID{UUID: s.unitUUID}, expState)
	})
	c.Assert(err, tc.ErrorIsNil)

	gotState := make(map[int]string)
	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		gotState = map[int]string{}

		q := "SELECT key, value FROM unit_state_relation WHERE unit_uuid = ?"
		rows, err := tx.QueryContext(ctx, q, s.unitUUID)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()

		for rows.Next() {
			var k int
			var v string
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

func (s *stateSuite) TestUpdateUnitStateRelationEmptyMap(c *tc.C) {
	ctx := c.Context()
	s.addRelation(c, 1, life.Alive)

	// Set some initial state. This should be deleted.
	s.query(c, `INSERT INTO unit_state_relation (unit_uuid, "key", value) VALUES (?, ?, ?)`,
		s.unitUUID, "1", "one-val")

	err := s.TxnRunner().Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.setUnitStateRelation(ctx, tx, entityUUID{UUID: s.unitUUID}, map[int]string{})
	})
	c.Assert(err, tc.ErrorIsNil)

	var rowCount int
	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		rowCount = 0

		q := "SELECT key, value FROM unit_state_relation WHERE unit_uuid = ?"
		rows, err := tx.QueryContext(ctx, q, s.unitUUID)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()

		for rows.Next() {
			rowCount++
		}
		return rows.Err()
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Check(rowCount, tc.DeepEquals, 0)
}

func (s *stateSuite) addRelation(c *tc.C, relationID int, relationLife life.Life) string {
	relationUUID := tc.Must(c, uuid.NewUUID).String()
	s.query(c, `INSERT INTO relation (uuid, life_id, relation_id, scope_id) VALUES (?, ?, ?, 0)`,
		relationUUID, relationLife, relationID)
	return relationUUID
}
