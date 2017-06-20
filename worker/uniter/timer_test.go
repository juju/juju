// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/uniter"
)

type timerSuite struct{}

var _ = gc.Suite(&timerSuite{})

func (s *timerSuite) TestTimer(c *gc.C) {
	nominal := 100 * time.Second
	min := 80*time.Second - time.Millisecond
	max := 120*time.Second + time.Millisecond

	timer := uniter.NewUpdateStatusTimer()
	for i := 0; i < 100; i++ {
		wait := timer(nominal)
		c.Assert(min, jc.LessThan, wait)
		c.Assert(wait, jc.LessThan, max)
	}
}
