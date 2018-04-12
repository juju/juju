// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	workercontroller "github.com/juju/juju/worker/controller"
)

type TrackerSuite struct {
	statetesting.StateSuite
	ctlr    *state.Controller
	tracker workercontroller.Tracker
}

var _ = gc.Suite(&TrackerSuite{})

func (s *TrackerSuite) SetUpTest(c *gc.C) {
	s.StateSuite.SetUpTest(c)
	s.tracker = workercontroller.NewTracker(s.Controller)
}

func (s *TrackerSuite) TestDoneWithNoUse(c *gc.C) {
	err := s.tracker.Done()
	c.Assert(err, jc.ErrorIsNil)
	assertControllerClosed(c, s.Controller)
}

func (s *TrackerSuite) TestTooManyDones(c *gc.C) {
	err := s.tracker.Done()
	c.Assert(err, jc.ErrorIsNil)
	assertControllerClosed(c, s.Controller)

	err = s.tracker.Done()
	c.Assert(err, gc.Equals, workercontroller.ErrControllerClosed)
	assertControllerClosed(c, s.Controller)
}

func (s *TrackerSuite) TestUse(c *gc.C) {
	st, err := s.tracker.Use()
	c.Check(st, gc.Equals, s.Controller)
	c.Check(err, jc.ErrorIsNil)

	st, err = s.tracker.Use()
	c.Check(st, gc.Equals, s.Controller)
	c.Check(err, jc.ErrorIsNil)
}

func (s *TrackerSuite) TestUseAndDone(c *gc.C) {
	// Ref count starts at 1 (the creator/owner)

	_, err := s.tracker.Use()
	// 2
	c.Check(err, jc.ErrorIsNil)

	_, err = s.tracker.Use()
	// 3
	c.Check(err, jc.ErrorIsNil)

	c.Check(s.tracker.Done(), jc.ErrorIsNil)
	// 2
	assertControllerNotClosed(c, s.Controller)

	_, err = s.tracker.Use()
	// 3
	c.Check(err, jc.ErrorIsNil)

	c.Check(s.tracker.Done(), jc.ErrorIsNil)
	// 2
	assertControllerNotClosed(c, s.Controller)

	c.Check(s.tracker.Done(), jc.ErrorIsNil)
	// 1
	assertControllerNotClosed(c, s.Controller)

	c.Check(s.tracker.Done(), jc.ErrorIsNil)
	// 0
	assertControllerClosed(c, s.Controller)
}

func (s *TrackerSuite) TestUseWhenClosed(c *gc.C) {
	c.Assert(s.tracker.Done(), jc.ErrorIsNil)

	st, err := s.tracker.Use()
	c.Check(st, gc.IsNil)
	c.Check(err, gc.Equals, workercontroller.ErrControllerClosed)
}

func assertControllerNotClosed(c *gc.C, ctlr *state.Controller) {
	err := ctlr.Ping()
	c.Assert(err, jc.ErrorIsNil)
}

func assertControllerClosed(c *gc.C, ctlr *state.Controller) {
	c.Assert(ctlr.Ping, gc.PanicMatches, "Session already closed")
}
