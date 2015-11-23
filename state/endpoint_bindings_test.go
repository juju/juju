// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type BindingsSuite struct {
	ConnSuite
}

var _ = gc.Suite(&BindingsSuite{})

func (s *BindingsSuite) TestSetBSON(c *gc.C) {
	svc, err := s.State.Service("foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(svc.Name(), gc.Equals, "foo")
}
