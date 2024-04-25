// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	schematesting "github.com/juju/juju/domain/schema/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type stateSuite struct {
	schematesting.ControllerSuite

	state *State
}

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) SetUpTest(c *gc.C) {
	s.ControllerSuite.SetUpTest(c)

	s.state = NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
}

func (s *stateSuite) TestGetFlagNotFound(c *gc.C) {
	value, err := s.state.GetFlag(context.Background(), "foo")
	c.Assert(err, jc.ErrorIs, errors.NotFound)
	c.Assert(value, jc.IsFalse)
}

func (s *stateSuite) TestGetFlagFound(c *gc.C) {
	err := s.state.SetFlag(context.Background(), "foo", true, "foo set to true")
	c.Assert(err, jc.ErrorIsNil)

	value, err := s.state.GetFlag(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(value, jc.IsTrue)
}

func (s *stateSuite) TestSetFlag(c *gc.C) {
	err := s.state.SetFlag(context.Background(), "foo", true, "foo set to true")
	c.Assert(err, jc.ErrorIsNil)

	var (
		value       bool
		description string
	)
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT value, description FROM flag WHERE name = 'foo'").Scan(&value, &description)
		if err != nil {
			return errors.Trace(err)
		}
		if !value {
			return errors.Errorf("unexpected value: %v", value)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(value, jc.IsTrue)
	c.Assert(description, gc.Equals, "foo set to true")
}

func (s *stateSuite) TestSetFlagAlreadyFound(c *gc.C) {
	err := s.state.SetFlag(context.Background(), "foo", true, "foo set to true")
	c.Assert(err, jc.ErrorIsNil)

	value, err := s.state.GetFlag(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(value, jc.IsTrue)

	err = s.state.SetFlag(context.Background(), "foo", false, "foo set to false")
	c.Assert(err, jc.ErrorIsNil)

	value, err = s.state.GetFlag(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(value, jc.IsFalse)
}
