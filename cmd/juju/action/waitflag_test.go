// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type WaitFlagSuite struct {
}

var _ = gc.Suite(&WaitFlagSuite{})

func (s *WaitFlagSuite) TestEmpty(c *gc.C) {
	var flag waitFlag
	timeout, forever := flag.Get()
	c.Assert(forever, gc.Equals, false)
	c.Assert(timeout, gc.Equals, time.Duration(0))
}

func (s *WaitFlagSuite) TestForever(c *gc.C) {
	var flag waitFlag
	err := flag.Set("true")
	c.Assert(err, jc.ErrorIsNil)
	timeout, forever := flag.Get()
	c.Assert(forever, gc.Equals, true)
	c.Assert(timeout, gc.Equals, time.Duration(0))
}

func (s *WaitFlagSuite) TestDuration(c *gc.C) {
	var flag waitFlag
	err := flag.Set("1ms")
	c.Assert(err, jc.ErrorIsNil)
	timeout, forever := flag.Get()
	c.Assert(forever, gc.Equals, false)
	c.Assert(timeout, gc.Equals, time.Millisecond)
}
