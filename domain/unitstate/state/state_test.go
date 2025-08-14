// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	stdtesting "testing"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	"github.com/juju/tc"

	modeltesting "github.com/juju/juju/core/model/testing"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	applicationstate "github.com/juju/juju/domain/application/state"
	"github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/domain/unitstate"
	unitstateerrors "github.com/juju/juju/domain/unitstate/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	coretesting "github.com/juju/juju/internal/testing"
)

type stateSuite struct {
	testing.ModelSuite

	unitUUID coreunit.UUID
	unitName coreunit.Name
}

func TestStateSuite(t *stdtesting.T) {
	tc.Run(t, &stateSuite{})
}

func (s *stateSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)

	modelUUID := modeltesting.GenModelUUID(c)
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO model (uuid, controller_uuid, name, qualifier, type, cloud, cloud_type)
			VALUES (?, ?, "test", "prod", "iaas", "test-model", "ec2")
		`, modelUUID.String(), coretesting.ControllerTag.Id())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	appState := applicationstate.NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	appArg := application.AddIAASApplicationArg{
		BaseAddApplicationArg: application.BaseAddApplicationArg{
			Charm: charm.Charm{
				Metadata: charm.Metadata{
					Name: "app",
				},
				Manifest: charm.Manifest{
					Bases: []charm.Base{{
						Name:          "ubuntu",
						Channel:       charm.Channel{Risk: charm.RiskStable},
						Architectures: []string{"amd64"},
					}},
				},
				ReferenceName: "app",
				Source:        charm.LocalSource,
				Architecture:  architecture.AMD64,
			},
		},
	}

	s.unitName = coreunit.GenName(c, "app/0")
	unitArgs := []application.AddIAASUnitArg{{}}

	ctx := c.Context()
	_, _, err = appState.CreateIAASApplication(ctx, "app", appArg, unitArgs)
	c.Assert(err, tc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "SELECT uuid FROM unit").Scan(&s.unitUUID)
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *stateSuite) TestSetUnitState(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	agentState := unitstate.UnitState{
		Name:          s.unitName,
		CharmState:    ptr(map[string]string{"one-key": "one-value"}),
		UniterState:   ptr("some-uniter-state-yaml"),
		RelationState: ptr(map[int]string{1: "one-value"}),
		StorageState:  ptr("some-storage-state-yaml"),
		SecretState:   ptr("some-secret-state-yaml"),
	}
	st.SetUnitState(c.Context(), agentState)

	expectedAgentState := unitstate.RetrievedUnitState{
		CharmState:    *agentState.CharmState,
		UniterState:   *agentState.UniterState,
		RelationState: *agentState.RelationState,
		StorageState:  *agentState.StorageState,
		SecretState:   *agentState.SecretState,
	}

	state, err := st.GetUnitState(c.Context(), s.unitName)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(state, tc.DeepEquals, expectedAgentState)
}

func (s *stateSuite) TestSetUnitStateJustUniterState(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	agentState := unitstate.UnitState{
		Name:        s.unitName,
		UniterState: ptr("some-uniter-state-yaml"),
	}
	st.SetUnitState(c.Context(), agentState)

	expectedAgentState := unitstate.RetrievedUnitState{
		UniterState: *agentState.UniterState,
	}

	state, err := st.GetUnitState(c.Context(), s.unitName)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(state, tc.DeepEquals, expectedAgentState)
}

func (s *stateSuite) TestGetUnitStateUnitNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	_, err := st.GetUnitState(c.Context(), "bad-uuid")
	c.Assert(err, tc.ErrorIs, unitstateerrors.UnitNotFound)
}

func (s *stateSuite) TestEnsureUnitStateRecord(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()

	err := s.TxnRunner().Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return st.ensureUnitStateRecord(ctx, tx, unitUUID{UUID: s.unitUUID})
	})
	c.Assert(err, tc.ErrorIsNil)

	var uuid coreunit.UUID
	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "SELECT uuid FROM unit").Scan(&uuid)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(uuid, tc.Equals, s.unitUUID)

	// Running again makes no change.
	err = s.TxnRunner().Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return st.ensureUnitStateRecord(ctx, tx, unitUUID{UUID: s.unitUUID})
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "SELECT uuid FROM unit").Scan(&uuid)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(uuid, tc.Equals, s.unitUUID)
}

func (s *stateSuite) TestUpdateUnitStateUniter(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()
	expState := "some uniter state YAML"

	err := s.TxnRunner().Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := st.ensureUnitStateRecord(ctx, tx, unitUUID{UUID: s.unitUUID}); err != nil {
			return err
		}
		return st.updateUnitStateUniter(ctx, tx, unitUUID{UUID: s.unitUUID}, expState)
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
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()
	expState := "some storage state YAML"

	err := s.TxnRunner().Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := st.ensureUnitStateRecord(ctx, tx, unitUUID{UUID: s.unitUUID}); err != nil {
			return err
		}
		return st.updateUnitStateStorage(ctx, tx, unitUUID{UUID: s.unitUUID}, expState)
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
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()
	expState := "some secret state YAML"

	err := s.TxnRunner().Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := st.ensureUnitStateRecord(ctx, tx, unitUUID{UUID: s.unitUUID}); err != nil {
			return err
		}
		return st.updateUnitStateSecret(ctx, tx, unitUUID{UUID: s.unitUUID}, expState)
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
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()

	// Set some initial state. This should be overwritten.
	err := s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		q := "INSERT INTO unit_state_charm VALUES (?, 'one-key', 'one-val')"
		_, err := tx.ExecContext(ctx, q, s.unitUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	expState := map[string]string{
		"two-key":   "two-val",
		"three-key": "three-val",
	}

	err = s.TxnRunner().Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return st.setUnitStateCharm(ctx, tx, unitUUID{UUID: s.unitUUID}, expState)
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
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()

	// Set some initial state. This should be overwritten.
	err := s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		q := "INSERT INTO unit_state_relation VALUES (?, 1, 'one-val')"
		_, err := tx.ExecContext(ctx, q, s.unitUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	expState := map[int]string{
		2: "two-val",
		3: "three-val",
	}

	err = s.TxnRunner().Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return st.setUnitStateRelation(ctx, tx, unitUUID{UUID: s.unitUUID}, expState)
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
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()

	// Set some initial state. This should be overwritten.
	err := s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		q := "INSERT INTO unit_state_relation VALUES (?, 1, 'one-val')"
		_, err := tx.ExecContext(ctx, q, s.unitUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.TxnRunner().Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return st.setUnitStateRelation(ctx, tx, unitUUID{UUID: s.unitUUID}, map[int]string{})
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

func ptr[T any](v T) *T {
	return &v
}
