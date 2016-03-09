// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmrevision_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/clock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/charmrevision"
)

type ValidateSuite struct {
	testing.IsolationSuite
	config charmrevision.Config
}

var _ = gc.Suite(&ValidateSuite{})

func (s *ValidateSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.config = charmrevision.Config{
		RevisionUpdater: struct{ charmrevision.RevisionUpdater }{},
		Clock:           struct{ clock.Clock }{},
		Period:          time.Hour,
	}
}

func (s *ValidateSuite) TestValid(c *gc.C) {
	err := s.config.Validate()
	c.Check(err, jc.ErrorIsNil)
}

func (s *ValidateSuite) TestNilRevisionUpdater(c *gc.C) {
	s.config.RevisionUpdater = nil
	s.checkNotValid(c, "nil RevisionUpdater not valid")
}

func (s *ValidateSuite) TestNilClock(c *gc.C) {
	s.config.Clock = nil
	s.checkNotValid(c, "nil Clock not valid")
}

func (s *ValidateSuite) TestBadPeriods(c *gc.C) {
	for i, period := range []time.Duration{
		0, -time.Nanosecond, -time.Hour,
	} {
		c.Logf("test %d", i)
		s.config.Period = period
		s.checkNotValid(c, "non-positive Period not valid")
	}
}

func (s *ValidateSuite) checkNotValid(c *gc.C, match string) {
	check := func(err error) {
		c.Check(err, jc.Satisfies, errors.IsNotValid)
		c.Check(err, gc.ErrorMatches, match)
	}
	err := s.config.Validate()
	check(err)

	worker, err := charmrevision.NewWorker(s.config)
	c.Check(worker, gc.IsNil)
	check(err)
}
