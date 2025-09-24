// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"testing"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	"github.com/juju/tc"

	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/life"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type stateSuite struct {
	baseSuite
}

func TestStateSuite(t *testing.T) {
	tc.Run(t, &stateSuite{})
}

func (s *stateSuite) TestCheckApplicationNameAvailable(c *tc.C) {
	s.createIAASApplication(c, "foo", life.Alive)

	st := NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return st.checkApplicationNameAvailable(ctx, tx, "foo")
	})
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationAlreadyExists)
}

func (s *stateSuite) TestCheckApplicationNameAvailableSyntheticCMRApplication(c *tc.C) {
	id := s.createIAASApplication(c, "foo", life.Alive)

	// Switch the source_id of a charm to a synthetic CMR charm.
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
UPDATE charm SET source_id = 2, architecture_id = NULL WHERE uuid = (
SELECT charm_uuid FROM application WHERE uuid = ?
)`, id)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	err = s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return st.checkApplicationNameAvailable(ctx, tx, "foo")
	})
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationAlreadyExists)
}

func (s *stateSuite) TestCheckApplicationNameAvailableNoApplication(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return st.checkApplicationNameAvailable(ctx, tx, "foo")
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *stateSuite) TestCheckApplication(c *tc.C) {
	id := s.createIAASApplication(c, "foo", life.Alive)

	st := NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return st.checkApplicationNotDead(ctx, tx, id)
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *stateSuite) TestCheckApplicationSyntheticCMRApplication(c *tc.C) {
	id := s.createIAASApplication(c, "foo", life.Alive)

	// Switch the source_id of a charm to a synthetic CMR charm.
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
UPDATE charm SET source_id = 2, architecture_id = NULL WHERE uuid = (
SELECT charm_uuid FROM application WHERE uuid = ?
)`, id)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	err = s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return st.checkApplicationNotDead(ctx, tx, id)
	})
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *stateSuite) TestCheckApplicationExistsNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return st.checkApplicationNotDead(ctx, tx, "foo")
	})
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *stateSuite) TestCheckApplicationDying(c *tc.C) {
	id := s.createIAASApplication(c, "foo", life.Dying)

	st := NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return st.checkApplicationNotDead(ctx, tx, id)
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *stateSuite) TestCheckApplicationExistsDead(c *tc.C) {
	id := s.createIAASApplication(c, "foo", life.Dead)

	st := NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return st.checkApplicationNotDead(ctx, tx, id)
	})
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationIsDead)
}

func (s *stateSuite) TestCheckApplicationExistsAlive(c *tc.C) {
	id := s.createIAASApplication(c, "foo", life.Dying)

	st := NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return st.checkApplicationAlive(ctx, tx, id)
	})
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotAlive)
}

func (s *stateSuite) TestCheckApplicationExistsAliveSyntheticCMRApplication(c *tc.C) {
	id := s.createIAASApplication(c, "foo", life.Dying)

	// Switch the source_id of a charm to a synthetic CMR charm.
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
UPDATE charm SET source_id = 2, architecture_id = NULL WHERE uuid = (
SELECT charm_uuid FROM application WHERE uuid = ?
)`, id)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	err = s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return st.checkApplicationAlive(ctx, tx, id)
	})
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}
