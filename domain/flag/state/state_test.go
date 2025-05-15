// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/tc"

	coreerrors "github.com/juju/juju/core/errors"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type stateSuite struct {
	schematesting.ControllerSuite

	state *State
}

var _ = tc.Suite(&stateSuite{})

func (s *stateSuite) SetUpTest(c *tc.C) {
	s.ControllerSuite.SetUpTest(c)

	s.state = NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
}

func (s *stateSuite) TestGetFlagNotFound(c *tc.C) {
	value, err := s.state.GetFlag(c.Context(), "foo")
	c.Assert(err, tc.ErrorIs, coreerrors.NotFound)
	c.Assert(value, tc.IsFalse)
}

func (s *stateSuite) TestGetFlagFound(c *tc.C) {
	err := s.state.SetFlag(c.Context(), "foo", true, "foo set to true")
	c.Assert(err, tc.ErrorIsNil)

	value, err := s.state.GetFlag(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(value, tc.IsTrue)
}

func (s *stateSuite) TestSetFlag(c *tc.C) {
	err := s.state.SetFlag(c.Context(), "foo", true, "foo set to true")
	c.Assert(err, tc.ErrorIsNil)

	var flag dbFlag
	err = s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		stmt, err := sqlair.Prepare(`
SELECT (value, description) AS (&dbFlag.*) 
FROM   flag 
WHERE  name = 'foo'`, flag)
		if err != nil {
			return errors.Capture(err)
		}
		err = tx.Query(ctx, stmt).Get(&flag)
		if err != nil {
			return errors.Capture(err)
		}
		if !flag.Value {
			return errors.Errorf("unexpected value: %v", flag.Value)
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(flag.Value, tc.IsTrue)
	c.Assert(flag.Description, tc.Equals, "foo set to true")
}

func (s *stateSuite) TestSetFlagAlreadyFound(c *tc.C) {
	err := s.state.SetFlag(c.Context(), "foo", true, "foo set to true")
	c.Assert(err, tc.ErrorIsNil)

	value, err := s.state.GetFlag(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(value, tc.IsTrue)

	err = s.state.SetFlag(c.Context(), "foo", false, "foo set to false")
	c.Assert(err, tc.ErrorIsNil)

	value, err = s.state.GetFlag(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(value, tc.IsFalse)
}
