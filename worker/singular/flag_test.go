// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package singular_test

import (
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/singular"
)

type FlagSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&FlagSuite{})

func (s *FlagSuite) TestClaimError(c *gc.C) {
	var stub testing.Stub
	stub.SetErrors(errors.New("squish"))

	worker, err := singular.NewFlagWorker(singular.FlagConfig{
		Facade:   newStubFacade(&stub),
		Clock:    &fakeClock{},
		Duration: time.Hour,
	})
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "squish")
}

func (s *FlagSuite) TestClaimFailure(c *gc.C) {
	fix := newFixture(c, errClaimDenied, nil)
	fix.Run(c, func(flag *singular.FlagWorker, _ *testclock.Clock, _ func()) {
		c.Check(flag.Check(), jc.IsFalse)
		workertest.CheckAlive(c, flag)
	})
	fix.CheckClaimWait(c)
}

func (s *FlagSuite) TestClaimFailureWaitError(c *gc.C) {
	fix := newFixture(c, errClaimDenied, errors.New("glug"))
	fix.Run(c, func(flag *singular.FlagWorker, _ *testclock.Clock, unblock func()) {
		c.Check(flag.Check(), jc.IsFalse)
		unblock()
		err := workertest.CheckKilled(c, flag)
		c.Check(err, gc.ErrorMatches, "glug")
	})
	fix.CheckClaimWait(c)
}

func (s *FlagSuite) TestClaimFailureWaitSuccess(c *gc.C) {
	fix := newFixture(c, errClaimDenied, nil)
	fix.Run(c, func(flag *singular.FlagWorker, _ *testclock.Clock, unblock func()) {
		c.Check(flag.Check(), jc.IsFalse)
		unblock()
		err := workertest.CheckKilled(c, flag)
		c.Check(errors.Cause(err), gc.Equals, singular.ErrRefresh)
	})
	fix.CheckClaimWait(c)
}

func (s *FlagSuite) TestClaimSuccess(c *gc.C) {
	fix := newFixture(c, nil, errors.New("should not happen"))
	fix.Run(c, func(flag *singular.FlagWorker, clock *testclock.Clock, unblock func()) {
		<-clock.Alarms()
		clock.Advance(29 * time.Second)
		workertest.CheckAlive(c, flag)
	})
	fix.CheckClaims(c, 1)
}

func (s *FlagSuite) TestClaimSuccessThenFailure(c *gc.C) {
	fix := newFixture(c, nil, errClaimDenied)
	fix.Run(c, func(flag *singular.FlagWorker, clock *testclock.Clock, unblock func()) {
		<-clock.Alarms()
		clock.Advance(30 * time.Second)
		err := workertest.CheckKilled(c, flag)
		c.Check(errors.Cause(err), gc.Equals, singular.ErrRefresh)
	})
	fix.CheckClaims(c, 2)
}

func (s *FlagSuite) TestClaimSuccessesThenError(c *gc.C) {
	fix := newFixture(c)
	fix.Run(c, func(flag *singular.FlagWorker, clock *testclock.Clock, unblock func()) {
		<-clock.Alarms()
		clock.Advance(time.Minute)
		<-clock.Alarms()
		clock.Advance(time.Minute)
		workertest.CheckAlive(c, flag)
	})
	fix.CheckClaims(c, 3)
}
