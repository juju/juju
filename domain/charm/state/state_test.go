// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	charmtesting "github.com/juju/juju/core/charm/testing"
	charmerrors "github.com/juju/juju/domain/charm/errors"
	"github.com/juju/juju/internal/changestream/testing"
)

type stateSuite struct {
	testing.ModelSuite
}

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) SetUpTest(c *gc.C) {
	s.ModelSuite.SetUpTest(c)
}

func (s *stateSuite) TestGetCharmIDByRevision(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	id := charmtesting.GenCharmID(c)

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO charm (uuid, name) VALUES (?, 'foo')`, id.String())
		c.Assert(err, jc.ErrorIsNil)

		_, err = tx.ExecContext(ctx, `INSERT INTO charm_origin (charm_uuid, revision) VALUES (?, 1)`, id.String())
		c.Assert(err, jc.ErrorIsNil)

		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	charmID, err := st.GetCharmIDByRevision(context.Background(), "foo", 1)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(charmID, gc.Equals, id)
}

func (s *stateSuite) TestIsControllerCharmWithNoCharm(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	id := charmtesting.GenCharmID(c)

	_, err := st.IsControllerCharm(context.Background(), id)
	c.Assert(err, jc.ErrorIs, charmerrors.NotFound)
}

func (s *stateSuite) TestIsControllerCharmWithControllerCharm(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	id := charmtesting.GenCharmID(c)

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO charm (uuid, name) VALUES (?, 'juju-controller')`, id.String())
		c.Assert(err, jc.ErrorIsNil)
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	result, err := st.IsControllerCharm(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.IsTrue)
}

func (s *stateSuite) TestIsControllerCharmWithNoControllerCharm(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	id := charmtesting.GenCharmID(c)

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO charm (uuid, name) VALUES (?, 'ubuntu')`, id.String())
		c.Assert(err, jc.ErrorIsNil)
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	result, err := st.IsControllerCharm(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.IsFalse)
}

func (s *stateSuite) TestIsSubordinateCharmWithNoCharm(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	id := charmtesting.GenCharmID(c)

	_, err := st.IsSubordinateCharm(context.Background(), id)
	c.Assert(err, jc.ErrorIs, charmerrors.NotFound)
}

func (s *stateSuite) TestIsSubordinateCharmWithSubordinateCharm(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	id := charmtesting.GenCharmID(c)

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO charm (uuid, subordinate) VALUES (?, true)`, id.String())
		c.Assert(err, jc.ErrorIsNil)
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	result, err := st.IsSubordinateCharm(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.IsTrue)
}

func (s *stateSuite) TestIsSubordinateCharmWithNoSubordinateCharm(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	id := charmtesting.GenCharmID(c)

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO charm (uuid, subordinate) VALUES (?, false)`, id.String())
		c.Assert(err, jc.ErrorIsNil)
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	result, err := st.IsSubordinateCharm(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.IsFalse)
}

func (s *stateSuite) TestSupportsContainersWithNoCharm(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	id := charmtesting.GenCharmID(c)

	_, err := st.SupportsContainers(context.Background(), id)
	c.Assert(err, jc.ErrorIs, charmerrors.NotFound)
}

func (s *stateSuite) TestSupportsContainersWithContainers(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	id := charmtesting.GenCharmID(c)

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO charm (uuid, name) VALUES (?, 'ubuntu')`, id.String())
		c.Assert(err, jc.ErrorIsNil)

		_, err = tx.ExecContext(ctx, `INSERT INTO charm_container (charm_uuid, name) VALUES (?, 'ubuntu@22.04')`, id.String())
		c.Assert(err, jc.ErrorIsNil)

		_, err = tx.ExecContext(ctx, `INSERT INTO charm_container (charm_uuid, name) VALUES (?, 'ubuntu@20.04')`, id.String())
		c.Assert(err, jc.ErrorIsNil)
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	result, err := st.SupportsContainers(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.IsTrue)
}

func (s *stateSuite) TestSupportsContainersWithNoContainers(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	id := charmtesting.GenCharmID(c)

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO charm (uuid, subordinate) VALUES (?, 'ubuntu')`, id.String())
		c.Assert(err, jc.ErrorIsNil)
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	result, err := st.SupportsContainers(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.IsFalse)
}

func (s *stateSuite) TestIsCharmAvailableWithNoCharm(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	id := charmtesting.GenCharmID(c)

	_, err := st.IsCharmAvailable(context.Background(), id)
	c.Assert(err, jc.ErrorIs, charmerrors.NotFound)
}

func (s *stateSuite) TestIsCharmAvailableWithAvailable(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	id := charmtesting.GenCharmID(c)

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO charm (uuid, name) VALUES (?, 'ubuntu')`, id.String())
		c.Assert(err, jc.ErrorIsNil)

		_, err = tx.ExecContext(ctx, `INSERT INTO charm_state (charm_uuid, available) VALUES (?, true)`, id.String())
		c.Assert(err, jc.ErrorIsNil)
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	result, err := st.IsCharmAvailable(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.IsTrue)
}

func (s *stateSuite) TestIsCharmAvailableWithNotAvailable(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	id := charmtesting.GenCharmID(c)

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO charm (uuid, subordinate) VALUES (?, 'ubuntu')`, id.String())
		c.Assert(err, jc.ErrorIsNil)

		_, err = tx.ExecContext(ctx, `INSERT INTO charm_state (charm_uuid, available) VALUES (?, false)`, id.String())
		c.Assert(err, jc.ErrorIsNil)
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	result, err := st.IsCharmAvailable(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.IsFalse)
}

func (s *stateSuite) TestSetCharmAvailableWithNoCharm(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	id := charmtesting.GenCharmID(c)

	err := st.SetCharmAvailable(context.Background(), id)
	c.Assert(err, jc.ErrorIs, charmerrors.NotFound)
}

func (s *stateSuite) TestSetCharmAvailable(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	id := charmtesting.GenCharmID(c)

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO charm (uuid, name) VALUES (?, 'ubuntu')`, id.String())
		c.Assert(err, jc.ErrorIsNil)

		_, err = tx.ExecContext(ctx, `INSERT INTO charm_state (charm_uuid, available) VALUES (?, false)`, id.String())
		c.Assert(err, jc.ErrorIsNil)
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	result, err := st.IsCharmAvailable(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.IsFalse)

	err = st.SetCharmAvailable(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)

	result, err = st.IsCharmAvailable(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.IsTrue)
}

func (s *stateSuite) TestReserveCharmRevisionWithNoCharm(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	id := charmtesting.GenCharmID(c)

	_, err := st.ReserveCharmRevision(context.Background(), id, 1)
	c.Assert(err, jc.ErrorIs, charmerrors.NotFound)
}

func (s *stateSuite) TestReserveCharmRevision(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	id := charmtesting.GenCharmID(c)

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO charm (uuid, name, run_as_id) VALUES (?, 'ubuntu', 0)`, id.String())
		c.Assert(err, jc.ErrorIsNil)

		_, err = tx.ExecContext(ctx, `INSERT INTO charm_state (charm_uuid, available) VALUES (?, false)`, id.String())
		c.Assert(err, jc.ErrorIsNil)
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	newID, err := st.ReserveCharmRevision(context.Background(), id, 1)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(newID, gc.Not(gc.DeepEquals), id)

	// Ensure that the new charm is usable, although should not be available.
	result, err := st.IsCharmAvailable(context.Background(), newID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.IsFalse)
}
