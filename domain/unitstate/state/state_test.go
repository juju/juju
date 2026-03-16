// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	stdtesting "testing"

	"github.com/canonical/sqlair"
	"github.com/juju/tc"

	"github.com/juju/juju/domain/unitstate"
	unitstateerrors "github.com/juju/juju/domain/unitstate/errors"
)

type stateSuite struct {
	baseSuite
}

func TestStateSuite(t *stdtesting.T) {
	tc.Run(t, &stateSuite{})
}

func (s *stateSuite) TestSetUnitState(c *tc.C) {
	agentState := unitstate.UnitState{
		Name:          s.unitName,
		CharmState:    new(map[string]string{"one-key": "one-value"}),
		UniterState:   new("some-uniter-state-yaml"),
		RelationState: new(map[int]string{1: "one-value"}),
		StorageState:  new("some-storage-state-yaml"),
		SecretState:   new("some-secret-state-yaml"),
	}
	s.state.SetUnitState(c.Context(), agentState)

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
	c.Assert(err, tc.ErrorIs, unitstateerrors.UnitNotFound)
}

func (s *stateSuite) TestEnsureUnitStateRecord(c *tc.C) {
	ctx := c.Context()

	err := s.TxnRunner().Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.ensureUnitStateRecord(ctx, tx, unitUUID{UUID: s.unitUUID})
	})
	c.Assert(err, tc.ErrorIsNil)

	s.checkUnitUUID(c, s.unitUUID)

	// Running again makes no change.
	err = s.TxnRunner().Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.ensureUnitStateRecord(ctx, tx, unitUUID{UUID: s.unitUUID})
	})
	c.Assert(err, tc.ErrorIsNil)

	s.checkUnitUUID(c, s.unitUUID)
}

func (s *stateSuite) TestUpdateUnitStateUniter(c *tc.C) {
	ctx := c.Context()
	expState := "some uniter state YAML"

	err := s.TxnRunner().Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := s.state.ensureUnitStateRecord(ctx, tx, unitUUID{UUID: s.unitUUID}); err != nil {
			return err
		}
		return s.state.updateUnitStateUniter(ctx, tx, unitUUID{UUID: s.unitUUID}, expState)
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
		if err := s.state.ensureUnitStateRecord(ctx, tx, unitUUID{UUID: s.unitUUID}); err != nil {
			return err
		}
		return s.state.updateUnitStateStorage(ctx, tx, unitUUID{UUID: s.unitUUID}, expState)
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
		if err := s.state.ensureUnitStateRecord(ctx, tx, unitUUID{UUID: s.unitUUID}); err != nil {
			return err
		}
		return s.state.updateUnitStateSecret(ctx, tx, unitUUID{UUID: s.unitUUID}, expState)
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
		return s.state.setUnitStateCharm(ctx, tx, unitUUID{UUID: s.unitUUID}, expState)
	})
	c.Assert(err, tc.ErrorIsNil)

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

func (s *stateSuite) TestUpdateUnitStateRelation(c *tc.C) {
	ctx := c.Context()

	// Set some initial state. This should be overwritten.
	s.addUnitStateCharm(c, 1, "one-val")

	expState := map[int]string{
		2: "two-val",
		3: "three-val",
	}

	err := s.TxnRunner().Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.setUnitStateRelation(ctx, tx, unitUUID{UUID: s.unitUUID}, expState)
	})
	c.Assert(err, tc.ErrorIsNil)

	gotState := make(map[int]string)
	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
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

	// Set some initial state. This should be overwritten.
	s.addUnitStateCharm(c, 1, "one-val")

	err := s.TxnRunner().Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.setUnitStateRelation(ctx, tx, unitUUID{UUID: s.unitUUID}, map[int]string{})
	})
	c.Assert(err, tc.ErrorIsNil)

	var rowCount int
	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
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
