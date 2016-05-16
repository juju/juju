// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"

	jc "github.com/juju/testing/checkers"
	jujutxn "github.com/juju/txn"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

// RestoreInfoSuite is *tremendously* incomplete: this test exists purely to
// verify that independent RestoreInfoSetters can be created concurrently.
// This says nothing about whether that's a good idea (it's *not*) but it's
// what we currently do and we need it to not just arbitrarily fail.
//
// TODO(fwereade): 2016-03-23 lp:1560920
// None of the other functionality is tested, and little of it is reliable or
// consistent with the other state code, but that's not for today.
type RestoreInfoSuite struct {
	statetesting.StateSuite
	info *state.RestoreInfo
}

var _ = gc.Suite(&RestoreInfoSuite{})

func (s *RestoreInfoSuite) SetUpTest(c *gc.C) {
	s.StateSuite.SetUpTest(c)
	s.info = s.State.RestoreInfo()
}

func (s *RestoreInfoSuite) TestStartsNotActive(c *gc.C) {
	s.checkStatus(c, state.RestoreNotActive)
}

func (s *RestoreInfoSuite) TestSetBadStatus(c *gc.C) {
	err := s.info.SetStatus(state.RestoreStatus("LOLLYGAGGING"))
	c.Check(err, gc.ErrorMatches, "unknown restore status: LOLLYGAGGING")
	s.checkStatus(c, state.RestoreNotActive)
}

//--------------------------------------
// Whitebox race tests to trigger different paths through the SetStatus
// code; use arbitrary sample transitions, full set of valid transitions
// are checked further down.
func (s *RestoreInfoSuite) TestInsertRaceHarmless(c *gc.C) {
	defer state.SetBeforeHooks(
		c, s.State, func() {
			s.checkSetStatus(c, state.RestorePending)
		},
	).Check()
	s.checkSetStatus(c, state.RestorePending)
}

func (s *RestoreInfoSuite) TestInsertRaceFailure(c *gc.C) {
	defer state.SetBeforeHooks(
		c, s.State, func() {
			s.checkSetStatus(c, state.RestorePending)
			s.checkSetStatus(c, state.RestoreInProgress)
		},
	).Check()
	s.checkBadSetStatus(c, state.RestorePending)
	s.checkStatus(c, state.RestoreInProgress)
}

func (s *RestoreInfoSuite) TestUpdateRaceHarmless(c *gc.C) {
	s.setupInProgress(c)
	defer state.SetBeforeHooks(
		c, s.State, func() {
			s.checkSetStatus(c, state.RestoreFinished)
		},
	).Check()
	s.checkSetStatus(c, state.RestoreFinished)
}

func (s *RestoreInfoSuite) TestUpdateRaceFailure(c *gc.C) {
	s.setupInProgress(c)
	defer state.SetBeforeHooks(
		c, s.State, func() {
			s.checkSetStatus(c, state.RestoreFailed)
		},
	).Check()
	s.checkBadSetStatus(c, state.RestoreFinished)
	s.checkStatus(c, state.RestoreFailed)
}

func (s *RestoreInfoSuite) TestUpdateRaceExhaustion(c *gc.C) {
	s.setupPending(c)
	perturb := jujutxn.TestHook{
		Before: func() {
			s.checkSetStatus(c, state.RestoreFailed)
		},
		After: func() {
			s.checkSetStatus(c, state.RestorePending)
		},
	}
	defer state.SetTestHooks(
		c, s.State,
		perturb,
		perturb,
		perturb,
	).Check()
	err := s.info.SetStatus(state.RestoreInProgress)
	c.Check(err, gc.ErrorMatches, ".*state changing too quickly; try again soon")
}

//--------------------------------------
// Test NotActive -> ? transitions
func (s *RestoreInfoSuite) TestNotActiveSetNotActive(c *gc.C) {
	s.checkSetStatus(c, state.RestoreNotActive)
}

func (s *RestoreInfoSuite) TestNotActiveSetPending(c *gc.C) {
	s.checkSetStatus(c, state.RestorePending)
}

func (s *RestoreInfoSuite) TestNotActiveSetInProgress(c *gc.C) {
	s.checkBadSetStatus(c, state.RestoreInProgress)
}

func (s *RestoreInfoSuite) TestNotActiveSetChecked(c *gc.C) {
	s.checkBadSetStatus(c, state.RestoreChecked)
}

func (s *RestoreInfoSuite) TestNotActiveSetFailed(c *gc.C) {
	s.checkBadSetStatus(c, state.RestoreFailed)
}

//--------------------------------------
// Test Pending -> ? transitions
func (s *RestoreInfoSuite) setupPending(c *gc.C) {
	s.checkSetStatus(c, state.RestorePending)
}

func (s *RestoreInfoSuite) TestPendingSetNotActive(c *gc.C) {
	s.setupPending(c)
	s.checkBadSetStatus(c, state.RestoreNotActive)
}

func (s *RestoreInfoSuite) TestPendingSetPending(c *gc.C) {
	s.setupPending(c)
	s.checkSetStatus(c, state.RestorePending)
}

func (s *RestoreInfoSuite) TestPendingSetInProgress(c *gc.C) {
	s.setupPending(c)
	s.checkSetStatus(c, state.RestoreInProgress)
}

func (s *RestoreInfoSuite) TestPendingSetChecked(c *gc.C) {
	s.setupPending(c)
	s.checkBadSetStatus(c, state.RestoreChecked)
}

func (s *RestoreInfoSuite) TestPendingSetFailed(c *gc.C) {
	s.setupPending(c)
	s.checkSetStatus(c, state.RestoreFailed)
}

//--------------------------------------
// Test InProgress -> ? transitions
func (s *RestoreInfoSuite) setupInProgress(c *gc.C) {
	s.checkSetStatus(c, state.RestorePending)
	s.checkSetStatus(c, state.RestoreInProgress)
}

func (s *RestoreInfoSuite) TestInProgressSetNotActive(c *gc.C) {
	s.setupInProgress(c)
	s.checkBadSetStatus(c, state.RestoreNotActive)
}

func (s *RestoreInfoSuite) TestInProgressSetPending(c *gc.C) {
	s.setupInProgress(c)
	s.checkBadSetStatus(c, state.RestorePending)
}

func (s *RestoreInfoSuite) TestInProgressSetInProgress(c *gc.C) {
	s.setupInProgress(c)
	s.checkSetStatus(c, state.RestoreInProgress)
}

func (s *RestoreInfoSuite) TestInProgressSetFinished(c *gc.C) {
	s.setupInProgress(c)
	s.checkSetStatus(c, state.RestoreFinished)
}

func (s *RestoreInfoSuite) TestInProgressSetChecked(c *gc.C) {
	s.setupInProgress(c)
	s.checkBadSetStatus(c, state.RestoreChecked)
}

func (s *RestoreInfoSuite) TestInProgressSetFailed(c *gc.C) {
	s.setupInProgress(c)
	s.checkSetStatus(c, state.RestoreFailed)
}

//--------------------------------------
// Test Finished -> ? transitions
func (s *RestoreInfoSuite) setupFinished(c *gc.C) {
	s.checkSetStatus(c, state.RestorePending)
	s.checkSetStatus(c, state.RestoreInProgress)
	s.checkSetStatus(c, state.RestoreFinished)
}

func (s *RestoreInfoSuite) TestFinishedSetNotActive(c *gc.C) {
	s.setupFinished(c)
	s.checkBadSetStatus(c, state.RestoreNotActive)
}

func (s *RestoreInfoSuite) TestFinishedSetPending(c *gc.C) {
	s.setupFinished(c)
	s.checkBadSetStatus(c, state.RestorePending)
}

func (s *RestoreInfoSuite) TestFinishedSetInProgress(c *gc.C) {
	s.setupFinished(c)
	s.checkBadSetStatus(c, state.RestoreInProgress)
}

func (s *RestoreInfoSuite) TestFinishedSetFinished(c *gc.C) {
	s.setupFinished(c)
	s.checkSetStatus(c, state.RestoreFinished)
}

func (s *RestoreInfoSuite) TestFinishedSetChecked(c *gc.C) {
	s.setupFinished(c)
	s.checkSetStatus(c, state.RestoreChecked)
}

func (s *RestoreInfoSuite) TestFinishedSetFailed(c *gc.C) {
	s.setupFinished(c)
	s.checkSetStatus(c, state.RestoreFailed)
}

//--------------------------------------
// Test Checked -> ? transitions
func (s *RestoreInfoSuite) setupChecked(c *gc.C) {
	s.checkSetStatus(c, state.RestorePending)
	s.checkSetStatus(c, state.RestoreInProgress)
	s.checkSetStatus(c, state.RestoreFinished)
	s.checkSetStatus(c, state.RestoreChecked)
}

func (s *RestoreInfoSuite) TestCheckedSetNotActive(c *gc.C) {
	s.setupChecked(c)
	s.checkBadSetStatus(c, state.RestoreNotActive)
}

func (s *RestoreInfoSuite) TestCheckedSetPending(c *gc.C) {
	s.setupChecked(c)
	s.checkSetStatus(c, state.RestorePending)
}

func (s *RestoreInfoSuite) TestCheckedSetInProgress(c *gc.C) {
	s.setupChecked(c)
	s.checkBadSetStatus(c, state.RestoreInProgress)
}

func (s *RestoreInfoSuite) TestCheckedSetChecked(c *gc.C) {
	s.setupChecked(c)
	s.checkSetStatus(c, state.RestoreChecked)
}

func (s *RestoreInfoSuite) TestCheckedSetFailed(c *gc.C) {
	s.setupChecked(c)
	s.checkBadSetStatus(c, state.RestoreFailed)
}

//--------------------------------------
// Test Failed -> ? transitions
func (s *RestoreInfoSuite) setupFailed(c *gc.C) {
	s.checkSetStatus(c, state.RestorePending)
	s.checkSetStatus(c, state.RestoreFailed)
}

func (s *RestoreInfoSuite) TestFailedSetNotActive(c *gc.C) {
	s.setupFailed(c)
	s.checkBadSetStatus(c, state.RestoreNotActive)
}

func (s *RestoreInfoSuite) TestFailedSetPending(c *gc.C) {
	s.setupFailed(c)
	s.checkSetStatus(c, state.RestorePending)
}

func (s *RestoreInfoSuite) TestFailedSetInProgress(c *gc.C) {
	s.setupFailed(c)
	s.checkBadSetStatus(c, state.RestoreInProgress)
}

func (s *RestoreInfoSuite) TestFailedSetFinished(c *gc.C) {
	s.setupFailed(c)
	s.checkBadSetStatus(c, state.RestoreFinished)
}

func (s *RestoreInfoSuite) TestFailedSetChecked(c *gc.C) {
	s.setupFailed(c)
	s.checkBadSetStatus(c, state.RestoreChecked)
}

func (s *RestoreInfoSuite) TestFailedSetFailed(c *gc.C) {
	s.setupFailed(c)
	s.checkSetStatus(c, state.RestoreFailed)
}

//--------------------

func (s *RestoreInfoSuite) checkStatus(c *gc.C, expect state.RestoreStatus) {
	actual, err := s.info.Status()
	c.Check(err, jc.ErrorIsNil)
	c.Check(actual, gc.Equals, expect)
}

func (s *RestoreInfoSuite) checkSetStatus(c *gc.C, status state.RestoreStatus) {
	err := s.info.SetStatus(status)
	c.Check(err, jc.ErrorIsNil)
	s.checkStatus(c, status)
}

func (s *RestoreInfoSuite) checkBadSetStatus(c *gc.C, status state.RestoreStatus) {
	err := s.info.SetStatus(status)
	expect := fmt.Sprintf("setting status %q: invalid restore transition: [-A-Z]+ => %s", status, status)
	c.Check(err, gc.ErrorMatches, expect)
}
