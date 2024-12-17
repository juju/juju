// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type stateSuite struct {
	baseSuite
}

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) TestSequence(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	var sequence int
	err := s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		sequence, err = st.sequence(ctx, tx, "test")
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sequence, gc.Equals, 1)
}

func (s *stateSuite) TestSequenceMultipleTimes(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	for i := 0; i < 10; i++ {
		var sequence int
		err := s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
			var err error
			sequence, err = st.sequence(ctx, tx, "test")
			return err
		})
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(sequence, gc.Equals, i+1)
	}
}

func (s *stateSuite) TestSequenceMultipleTimesWithDifferentNames(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	for i := 0; i < 10; i++ {
		for j := 0; j < 5; j++ {
			var sequence int
			err := s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
				var err error
				sequence, err = st.sequence(ctx, tx, fmt.Sprintf("test-%d", j))
				return err
			})
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(sequence, gc.Equals, i+1)
		}
	}
}
