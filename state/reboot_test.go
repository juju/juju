// Copyright 2014 Cloudbase Solutions SRL.
// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

type RebootSuite struct {
	ConnSuite

	machine *state.Machine
	c1      *state.Machine
	c2      *state.Machine
	c3      *state.Machine

	w   state.NotifyWatcher
	wC1 state.NotifyWatcher
	wC2 state.NotifyWatcher
	wC3 state.NotifyWatcher

	wc   statetesting.NotifyWatcherC
	wcC1 statetesting.NotifyWatcherC
	wcC2 statetesting.NotifyWatcherC
	wcC3 statetesting.NotifyWatcherC
}

var _ = gc.Suite(&RebootSuite{})

func (s *RebootSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	var err error

	// Add machine
	s.machine, err = s.State.AddMachine("quantal", state.JobManageEnviron)
	c.Assert(err, jc.ErrorIsNil)
	// Add first container
	s.c1, err = s.State.AddMachineInsideMachine(state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}, s.machine.Id(), instance.LXC)
	c.Assert(err, jc.ErrorIsNil)
	// Add second container
	s.c2, err = s.State.AddMachineInsideMachine(state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}, s.c1.Id(), instance.LXC)
	c.Assert(err, jc.ErrorIsNil)

	// Add container on the same level as the first container.
	s.c3, err = s.State.AddMachineInsideMachine(state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}, s.machine.Id(), instance.LXC)
	c.Assert(err, jc.ErrorIsNil)

	s.w, err = s.machine.WatchForRebootEvent()
	c.Assert(err, jc.ErrorIsNil)

	s.wc = statetesting.NewNotifyWatcherC(c, s.State, s.w)
	s.wc.AssertOneChange()

	s.wC1, err = s.c1.WatchForRebootEvent()
	c.Assert(err, jc.ErrorIsNil)

	// Initial event on container 1.
	s.wcC1 = statetesting.NewNotifyWatcherC(c, s.State, s.wC1)
	s.wcC1.AssertOneChange()

	// Get reboot watcher on container 2
	s.wC2, err = s.c2.WatchForRebootEvent()
	c.Assert(err, jc.ErrorIsNil)

	// Initial event on container 2.
	s.wcC2 = statetesting.NewNotifyWatcherC(c, s.State, s.wC2)
	s.wcC2.AssertOneChange()

	// Get reboot watcher on container 3
	s.wC3, err = s.c3.WatchForRebootEvent()
	c.Assert(err, jc.ErrorIsNil)

	// Initial event on container 3.
	s.wcC3 = statetesting.NewNotifyWatcherC(c, s.State, s.wC3)
	s.wcC3.AssertOneChange()
}

func (s *RebootSuite) TearDownSuit(c *gc.C) {
	if s.w != nil {
		statetesting.AssertStop(c, s.w)
	}
	if s.wC1 != nil {
		statetesting.AssertStop(c, s.wC1)
	}
	if s.wC2 != nil {
		statetesting.AssertStop(c, s.wC2)
	}
	if s.wC3 != nil {
		statetesting.AssertStop(c, s.wC3)
	}
}

func (s *RebootSuite) TestWatchForRebootEvent(c *gc.C) {
	err := s.machine.SetRebootFlag(true)
	c.Assert(err, jc.ErrorIsNil)

	s.wc.AssertOneChange()

	inState, err := s.machine.GetRebootFlag()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(inState, jc.IsTrue)

	err = s.machine.SetRebootFlag(false)
	c.Assert(err, jc.ErrorIsNil)

	s.wc.AssertOneChange()

	inState, err = s.machine.GetRebootFlag()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(inState, jc.IsFalse)

	err = s.machine.SetRebootFlag(true)
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine.SetRebootFlag(false)
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine.SetRebootFlag(true)
	c.Assert(err, jc.ErrorIsNil)

	s.wc.AssertOneChange()

	// Stop all watchers and check they are closed
	statetesting.AssertStop(c, s.w)
	s.wc.AssertClosed()
	statetesting.AssertStop(c, s.wC1)
	s.wcC1.AssertClosed()
	statetesting.AssertStop(c, s.wC2)
	s.wcC2.AssertClosed()
	statetesting.AssertStop(c, s.wC3)
	s.wcC3.AssertClosed()
}

func (s *RebootSuite) TestWatchRebootHappensOnMachine(c *gc.C) {
	// Reboot request happens on machine: everyone see it (including container3)
	err := s.machine.SetRebootFlag(true)
	c.Assert(err, jc.ErrorIsNil)

	s.wc.AssertOneChange()
	s.wcC1.AssertOneChange()
	s.wcC2.AssertOneChange()
	s.wcC3.AssertOneChange()

	statetesting.AssertStop(c, s.w)
	s.wc.AssertClosed()
	statetesting.AssertStop(c, s.wC1)
	s.wcC1.AssertClosed()
	statetesting.AssertStop(c, s.wC2)
	s.wcC2.AssertClosed()
	statetesting.AssertStop(c, s.wC3)
	s.wcC3.AssertClosed()
}

func (s *RebootSuite) TestWatchRebootHappensOnContainer1(c *gc.C) {
	// Reboot request happens on container1: only container1 andcontainer2
	// react
	err := s.c1.SetRebootFlag(true)
	c.Assert(err, jc.ErrorIsNil)

	s.wc.AssertNoChange()
	s.wcC1.AssertOneChange()
	s.wcC2.AssertOneChange()
	s.wcC3.AssertNoChange()

	// Stop all watchers and check they are closed
	statetesting.AssertStop(c, s.w)
	s.wc.AssertClosed()
	statetesting.AssertStop(c, s.wC1)
	s.wcC1.AssertClosed()
	statetesting.AssertStop(c, s.wC2)
	s.wcC2.AssertClosed()
	statetesting.AssertStop(c, s.wC3)
	s.wcC3.AssertClosed()
}

func (s *RebootSuite) TestWatchRebootHappensOnContainer2(c *gc.C) {
	// Reboot request happens on container2: only container2 sees it
	err := s.c2.SetRebootFlag(true)
	c.Assert(err, jc.ErrorIsNil)

	s.wc.AssertNoChange()
	s.wcC1.AssertNoChange()
	s.wcC2.AssertOneChange()
	s.wcC3.AssertNoChange()

	// Stop all watchers and check they are closed
	statetesting.AssertStop(c, s.w)
	s.wc.AssertClosed()
	statetesting.AssertStop(c, s.wC1)
	s.wcC1.AssertClosed()
	statetesting.AssertStop(c, s.wC2)
	s.wcC2.AssertClosed()
	statetesting.AssertStop(c, s.wC3)
	s.wcC3.AssertClosed()
}

func (s *RebootSuite) TestWatchRebootHappensOnContainer3(c *gc.C) {
	// Reboot request happens on container2: only container2 sees it
	err := s.c3.SetRebootFlag(true)
	c.Assert(err, jc.ErrorIsNil)

	s.wc.AssertNoChange()
	s.wcC1.AssertNoChange()
	s.wcC2.AssertNoChange()
	s.wcC3.AssertOneChange()

	// Stop all watchers and check they are closed
	statetesting.AssertStop(c, s.w)
	s.wc.AssertClosed()
	statetesting.AssertStop(c, s.wC1)
	s.wcC1.AssertClosed()
	statetesting.AssertStop(c, s.wC2)
	s.wcC2.AssertClosed()
	statetesting.AssertStop(c, s.wC3)
	s.wcC3.AssertClosed()
}
