// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/charm"
	applicationstate "github.com/juju/juju/domain/application/state"
	"github.com/juju/juju/domain/schema/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type stateSuite struct {
	testing.ModelSuite

	unitUUID string
}

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) SetUpTest(c *gc.C) {
	s.ModelSuite.SetUpTest(c)

	appState := applicationstate.NewApplicationState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	appArg := application.AddApplicationArg{
		Charm: charm.Charm{
			Metadata: charm.Metadata{
				Name: "app",
			},
		},
	}

	unitArg := application.UpsertUnitArg{UnitName: ptr("app/0")}

	ctx := context.Background()
	_, err := appState.CreateApplication(ctx, "app", appArg, unitArg)
	c.Assert(err, jc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "SELECT uuid FROM unit").Scan(&s.unitUUID)
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) TestGetUUIDForName(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()

	var uuid string
	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		var err error
		uuid, err = st.GetUnitUUIDForName(ctx, "app/0")
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(uuid, gc.Equals, s.unitUUID)
}

func (s *stateSuite) TestEnsureUnitStateRecord(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()

	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.EnsureUnitStateRecord(ctx, s.unitUUID)
	})
	c.Assert(err, jc.ErrorIsNil)

	var uuid string
	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "SELECT uuid FROM unit").Scan(&uuid)
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(uuid, gc.Equals, s.unitUUID)

	// Running again makes no change.
	err = st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.EnsureUnitStateRecord(ctx, s.unitUUID)
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "SELECT uuid FROM unit").Scan(&uuid)
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(uuid, gc.Equals, s.unitUUID)
}

func (s *stateSuite) TestUpdateUnitStateUniter(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	expState := "some uniter state YAML"

	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		if err := st.EnsureUnitStateRecord(ctx, s.unitUUID); err != nil {
			return err
		}
		return st.UpdateUnitStateUniter(ctx, s.unitUUID, expState)
	})
	c.Assert(err, jc.ErrorIsNil)

	var gotState string
	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		q := "SELECT uniter_state FROM unit_state where unit_uuid = ?"
		return tx.QueryRowContext(ctx, q, s.unitUUID).Scan(&gotState)
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotState, gc.Equals, expState)
}

func (s *stateSuite) TestUpdateUnitStateStorage(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	expState := "some storage state YAML"

	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		if err := st.EnsureUnitStateRecord(ctx, s.unitUUID); err != nil {
			return err
		}
		return st.UpdateUnitStateStorage(ctx, s.unitUUID, expState)
	})
	c.Assert(err, jc.ErrorIsNil)

	var gotState string
	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		q := "SELECT storage_state FROM unit_state where unit_uuid = ?"
		return tx.QueryRowContext(ctx, q, s.unitUUID).Scan(&gotState)
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotState, gc.Equals, expState)
}

func (s *stateSuite) TestUpdateUnitStateSecret(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	expState := "some secret state YAML"

	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		if err := st.EnsureUnitStateRecord(ctx, s.unitUUID); err != nil {
			return err
		}
		return st.UpdateUnitStateSecret(ctx, s.unitUUID, expState)
	})
	c.Assert(err, jc.ErrorIsNil)

	var gotState string
	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		q := "SELECT secret_state FROM unit_state where unit_uuid = ?"
		return tx.QueryRowContext(ctx, q, s.unitUUID).Scan(&gotState)
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotState, gc.Equals, expState)
}

func (s *stateSuite) TestUpdateUnitStateCharm(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()

	// Set some initial state. This should be overwritten.
	err := s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		q := "INSERT INTO unit_state_charm VALUES (?, 'one-key', 'one-val')"
		_, err := tx.ExecContext(ctx, q, s.unitUUID)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	expState := map[string]string{
		"two-key":   "two-val",
		"three-key": "three-val",
	}

	err = st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.SetUnitStateCharm(ctx, s.unitUUID, expState)
	})
	c.Assert(err, jc.ErrorIsNil)

	gotState := make(map[string]string)
	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		q := "SELECT key, value FROM unit_state_charm WHERE unit_uuid = ?"
		rows, err := tx.QueryContext(ctx, q, s.unitUUID)
		if err != nil {
			return err
		}

		for rows.Next() {
			var k, v string
			if err := rows.Scan(&k, &v); err != nil {
				return err
			}
			gotState[k] = v
		}
		return rows.Close()
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(gotState, gc.DeepEquals, expState)
}

func (s *stateSuite) TestUpdateUnitStateRelation(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()

	// Set some initial state. This should be overwritten.
	err := s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		q := "INSERT INTO unit_state_relation VALUES (?, 'one-key', 'one-val')"
		_, err := tx.ExecContext(ctx, q, s.unitUUID)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	expState := map[string]string{
		"two-key":   "two-val",
		"three-key": "three-val",
	}

	err = st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.SetUnitStateRelation(ctx, s.unitUUID, expState)
	})
	c.Assert(err, jc.ErrorIsNil)

	gotState := make(map[string]string)
	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		q := "SELECT key, value FROM unit_state_relation WHERE unit_uuid = ?"
		rows, err := tx.QueryContext(ctx, q, s.unitUUID)
		if err != nil {
			return err
		}

		for rows.Next() {
			var k, v string
			if err := rows.Scan(&k, &v); err != nil {
				return err
			}
			gotState[k] = v
		}
		return rows.Close()
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(gotState, gc.DeepEquals, expState)
}
