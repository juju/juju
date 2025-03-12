// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/life"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type stateSuite struct {
	baseSuite
}

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) TestCheckApplicationNameAvailable(c *gc.C) {
	s.createApplication(c, "foo", life.Alive)

	st := NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	err := s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		return st.checkApplicationNameAvailable(ctx, tx, "foo")
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationAlreadyExists)
}

func (s *stateSuite) TestCheckApplicationNameAvailableNoApplication(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	err := s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		return st.checkApplicationNameAvailable(ctx, tx, "foo")
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) TestCheckApplication(c *gc.C) {
	id := s.createApplication(c, "foo", life.Alive)

	st := NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	err := s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		return st.checkApplicationNotDead(ctx, tx, id)
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) TestCheckApplicationExistsNotFound(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	err := s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		return st.checkApplicationNotDead(ctx, tx, "foo")
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *stateSuite) TestCheckApplicationDying(c *gc.C) {
	id := s.createApplication(c, "foo", life.Dying)

	st := NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	err := s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		return st.checkApplicationNotDead(ctx, tx, id)
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) TestCheckApplicationExistsDead(c *gc.C) {
	id := s.createApplication(c, "foo", life.Dead)

	st := NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	err := s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		return st.checkApplicationNotDead(ctx, tx, id)
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationIsDead)
}

func (s *stateSuite) TestCheckApplicationExistsAlive(c *gc.C) {
	id := s.createApplication(c, "foo", life.Dying)

	st := NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	err := s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		return st.checkApplicationAlive(ctx, tx, id)
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotAlive)
}
