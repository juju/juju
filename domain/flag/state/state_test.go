// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	schematesting "github.com/juju/juju/domain/schema/testing"
	jujutesting "github.com/juju/juju/testing"
)

type stateSuite struct {
	schematesting.ControllerSuite

	state *State
}

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) SetUpTest(c *gc.C) {
	s.ControllerSuite.SetUpTest(c)

	s.state = NewState(s.TxnRunnerFactory(), jujutesting.NewCheckLogger(c))
}

func (s *stateSuite) TestGetFlagNotFound(c *gc.C) {
	value, err := s.state.GetFlag(context.Background(), "foo")
	c.Assert(err, jc.ErrorIs, errors.NotFound)
	c.Assert(value, jc.IsFalse)
}

func (s *stateSuite) TestGetFlagFound(c *gc.C) {
	err := s.state.SetFlag(context.Background(), "foo", true)
	c.Assert(err, jc.ErrorIsNil)

	value, err := s.state.GetFlag(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(value, jc.IsTrue)
}

func (s *stateSuite) TestSetFlagAlreadyFound(c *gc.C) {
	err := s.state.SetFlag(context.Background(), "foo", true)
	c.Assert(err, jc.ErrorIsNil)

	value, err := s.state.GetFlag(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(value, jc.IsTrue)

	err = s.state.SetFlag(context.Background(), "foo", false)
	c.Assert(err, jc.ErrorIsNil)

	value, err = s.state.GetFlag(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(value, jc.IsFalse)
}
