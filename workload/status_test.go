// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workload_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/workload"
)

var (
	states = []string{
		workload.StateUndefined,
		workload.StateDefined,
		workload.StateStarting,
		workload.StateRunning,
		workload.StateStopping,
		workload.StateStopped,
	}

	initialStates = []string{
		workload.StateUndefined,
		workload.StateDefined,
	}

	finalStates = []string{
		workload.StateStopped,
	}
)

type statusSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&statusSuite{})

func (s *statusSuite) newStatus(state string) workload.Status {
	return workload.Status{
		State: state,
	}
}

func (s *statusSuite) checkStatus(c *gc.C, status workload.Status, state, msg, blocker string) {
	c.Check(status.State, gc.Equals, state)
	c.Check(status.Blocker, gc.Equals, blocker)
	c.Check(status.Message, gc.Equals, msg)
}

func (s *statusSuite) checkStatusOkay(c *gc.C, status workload.Status, state, msg string) {
	s.checkStatus(c, status, state, msg, "")
}

func (s *statusSuite) TestStringOkay(c *gc.C) {
	var status workload.Status
	status.Message = "nothing to see here"
	for _, state := range states {
		c.Logf("checking %q", state)
		status.State = state
		str := status.String()

		c.Check(str, gc.Equals, "nothing to see here")
	}
}

func (s *statusSuite) TestStringNoMessage(c *gc.C) {
	var status workload.Status
	for _, state := range states {
		c.Logf("checking %q", state)
		status.State = state
		str := status.String()

		c.Check(str, gc.Equals, "")
	}
}

func (s *statusSuite) TestStringBlockerNoMessage(c *gc.C) {
	status := workload.Status{
		State:   workload.StateRunning,
		Blocker: workload.BlockerFailed,
		Message: "",
	}
	str := status.String()

	c.Check(str, gc.Equals, "(failed) <no message>")
}

func (s *statusSuite) TestStringFailed(c *gc.C) {
	status := workload.Status{
		State:   workload.StateRunning,
		Blocker: workload.BlockerFailed,
		Message: "uh-oh",
	}
	str := status.String()

	c.Check(str, gc.Equals, "(failed) uh-oh")
}

func (s *statusSuite) TestStringError(c *gc.C) {
	status := workload.Status{
		State:   workload.StateRunning,
		Blocker: workload.BlockerError,
		Message: "uh-oh",
	}
	str := status.String()

	c.Check(str, gc.Equals, "(error) uh-oh")
}

func (s *statusSuite) TestIsBlockedFalse(c *gc.C) {
	status := s.newStatus(workload.StateRunning)
	blocked := status.IsBlocked()

	c.Check(blocked, jc.IsFalse)
}

func (s *statusSuite) TestIsBlockedTrue(c *gc.C) {
	status := s.newStatus(workload.StateRunning)
	status.Blocker = workload.BlockerFailed
	blocked := status.IsBlocked()

	c.Check(blocked, jc.IsTrue)
}

func (s *statusSuite) TestAdvanceTraverse(c *gc.C) {
	status := workload.Status{}
	s.checkStatusOkay(c, status, workload.StateUndefined, "")

	err := status.Advance("")
	c.Assert(err, jc.ErrorIsNil)
	s.checkStatusOkay(c, status, workload.StateDefined, "")

	err = status.Advance("")
	c.Assert(err, jc.ErrorIsNil)
	s.checkStatusOkay(c, status, workload.StateStarting, "")

	err = status.Advance("")
	c.Assert(err, jc.ErrorIsNil)
	s.checkStatusOkay(c, status, workload.StateRunning, "")

	err = status.Advance("")
	c.Assert(err, jc.ErrorIsNil)
	s.checkStatusOkay(c, status, workload.StateStopping, "")

	err = status.Advance("")
	c.Assert(err, jc.ErrorIsNil)
	s.checkStatusOkay(c, status, workload.StateStopped, "")

	err = status.Advance("")
	c.Check(err, gc.NotNil)
}

func (s *statusSuite) TestAdvanceMessage(c *gc.C) {
	status := s.newStatus(workload.StateDefined)
	c.Assert(status.Message, gc.Equals, "")

	err := status.Advance("good things to come")
	c.Assert(err, jc.ErrorIsNil)
	s.checkStatusOkay(c, status, workload.StateStarting, "good things to come")

	err = status.Advance("rock and roll")
	c.Assert(err, jc.ErrorIsNil)
	s.checkStatusOkay(c, status, workload.StateRunning, "rock and roll")

	err = status.Advance("")
	c.Assert(err, jc.ErrorIsNil)
	s.checkStatusOkay(c, status, workload.StateStopping, "")
}

func (s *statusSuite) TestAdvanceInvalid(c *gc.C) {
	status := s.newStatus("some bogus state")
	err := status.Advance("")

	c.Check(err, gc.ErrorMatches, `unrecognized state.*`)
}

func (s *statusSuite) TestAdvanceFinal(c *gc.C) {
	for _, state := range finalStates {
		c.Logf("checking %q", state)
		status := s.newStatus(state)
		err := status.Advance("")

		c.Check(err, gc.ErrorMatches, `cannot advance from a final state`)
	}
}

func (s *statusSuite) TestAdvanceBlocker(c *gc.C) {
	for _, state := range states {
		c.Logf("checking %q", state)
		status := s.newStatus(state)
		status.Blocker = workload.BlockerFailed
		err := status.Advance("")

		c.Check(err, gc.ErrorMatches, `cannot advance state \(failed\)`)
	}
}

func (s *statusSuite) TestSetFailedOkay(c *gc.C) {
	status := s.newStatus(workload.StateRunning)
	status.Message = "good to go"
	err := status.SetFailed("uh-oh")
	c.Assert(err, jc.ErrorIsNil)

	s.checkStatus(c, status, workload.StateRunning, "uh-oh", workload.BlockerFailed)
}

func (s *statusSuite) TestSetFailedDefaultMessage(c *gc.C) {
	status := s.newStatus(workload.StateRunning)
	status.Message = "good to go"
	err := status.SetFailed("")
	c.Assert(err, jc.ErrorIsNil)

	msg := "problem while interacting with workload"
	s.checkStatus(c, status, workload.StateRunning, msg, workload.BlockerFailed)
}

func (s *statusSuite) TestSetFailedAlreadyFailed(c *gc.C) {
	status := s.newStatus(workload.StateRunning)
	status.Message = "good to go"
	err := status.SetFailed("")
	c.Assert(err, jc.ErrorIsNil)
	err = status.SetFailed("uh-oh")
	c.Assert(err, jc.ErrorIsNil)

	s.checkStatus(c, status, workload.StateRunning, "uh-oh", workload.BlockerFailed)
}

func (s *statusSuite) TestSetFailedAlreadyError(c *gc.C) {
	status := s.newStatus(workload.StateRunning)
	status.Blocker = workload.BlockerError
	err := status.SetFailed("uh-oh")
	c.Assert(err, jc.ErrorIsNil)

	s.checkStatus(c, status, workload.StateRunning, "uh-oh", workload.BlockerFailed)
}

func (s *statusSuite) TestSetFailedBadState(c *gc.C) {
	status := workload.Status{
		Blocker: "",
		Message: "good to go",
	}
	test := func(state, msg string) {
		c.Logf("checking %q", state)
		status.State = state
		err := status.SetFailed("")

		c.Check(err, gc.ErrorMatches, msg)
		s.checkStatusOkay(c, status, state, "good to go")
	}
	for _, state := range initialStates {
		test(state, `cannot fail while in an initial state`)
	}
	for _, state := range finalStates {
		test(state, `cannot fail while in a final state`)
	}
}

func (s *statusSuite) TestSetErrorOkay(c *gc.C) {
	status := s.newStatus(workload.StateRunning)
	status.Message = "good to go"
	err := status.SetError("uh-oh")
	c.Assert(err, jc.ErrorIsNil)

	s.checkStatus(c, status, workload.StateRunning, "uh-oh", workload.BlockerError)
}

func (s *statusSuite) TestSetErrorDefaultMessage(c *gc.C) {
	status := s.newStatus(workload.StateRunning)
	status.Message = "good to go"
	err := status.SetError("")
	c.Assert(err, jc.ErrorIsNil)

	msg := "the workload has an error"
	s.checkStatus(c, status, workload.StateRunning, msg, workload.BlockerError)
}

func (s *statusSuite) TestSetErrorAlreadyError(c *gc.C) {
	status := s.newStatus(workload.StateRunning)
	status.Message = "good to go"
	err := status.SetError("")
	c.Assert(err, jc.ErrorIsNil)
	err = status.SetError("uh-oh")
	c.Assert(err, jc.ErrorIsNil)

	s.checkStatus(c, status, workload.StateRunning, "uh-oh", workload.BlockerError)
}

func (s *statusSuite) TestSetErrorAlreadyFailed(c *gc.C) {
	status := s.newStatus(workload.StateRunning)
	status.Blocker = workload.BlockerFailed
	status.Message = "good to go"
	err := status.SetError("uh-oh")
	c.Assert(err, jc.ErrorIsNil)

	s.checkStatus(c, status, workload.StateRunning, "uh-oh", workload.BlockerError)
}

func (s *statusSuite) TestSetErrorBadState(c *gc.C) {
	status := s.newStatus(workload.StateStarting)
	err := status.SetError("")

	c.Check(err, gc.ErrorMatches, `can error only while running`)
}

func (s *statusSuite) TestResolveErrorOkay(c *gc.C) {
	status := workload.Status{
		State:   workload.StateRunning,
		Blocker: workload.BlockerError,
		Message: "uh-oh",
	}
	err := status.Resolve("good to go")
	c.Assert(err, jc.ErrorIsNil)

	s.checkStatusOkay(c, status, workload.StateRunning, "good to go")
}

func (s *statusSuite) TestResolveErrorDefaultMessage(c *gc.C) {
	status := workload.Status{
		State:   workload.StateRunning,
		Blocker: workload.BlockerError,
		Message: "uh-oh",
	}
	err := status.Resolve("")
	c.Assert(err, jc.ErrorIsNil)

	s.checkStatusOkay(c, status, workload.StateRunning, `resolved blocker "error"`)
}

func (s *statusSuite) TestResolveFailedOkay(c *gc.C) {
	status := workload.Status{
		State:   workload.StateRunning,
		Blocker: workload.BlockerFailed,
		Message: "uh-oh",
	}
	err := status.Resolve("good to go")
	c.Assert(err, jc.ErrorIsNil)

	s.checkStatusOkay(c, status, workload.StateRunning, "good to go")
}

func (s *statusSuite) TestResolveFailedDefaultMessage(c *gc.C) {
	status := workload.Status{
		State:   workload.StateRunning,
		Blocker: workload.BlockerFailed,
		Message: "uh-oh",
	}
	err := status.Resolve("")
	c.Assert(err, jc.ErrorIsNil)

	s.checkStatusOkay(c, status, workload.StateRunning, `resolved blocker "failed"`)
}

func (s *statusSuite) TestResolveNotBlocked(c *gc.C) {
	status := workload.Status{
		State:   workload.StateRunning,
		Blocker: "",
		Message: "nothing wrong",
	}
	err := status.Resolve("good to go")

	c.Check(err, gc.ErrorMatches, `not in an error or failed state`)
	s.checkStatusOkay(c, status, workload.StateRunning, "nothing wrong")
}

func (s *statusSuite) TestValidateOkay(c *gc.C) {
	status := workload.Status{
		Blocker: "",
		Message: "nothing wrong",
	}
	for _, state := range states {
		if state == workload.StateUndefined || state == workload.StateDefined {
			continue
		}
		c.Logf("checking %q", state)
		status.State = state
		err := status.Validate()

		c.Check(err, jc.ErrorIsNil)
	}
}

func (s *statusSuite) TestValidateNoMessage(c *gc.C) {
	status := workload.Status{
		State:   workload.StateRunning,
		Message: "",
	}
	err := status.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *statusSuite) TestValidateUndefinedState(c *gc.C) {
	var status workload.Status
	err := status.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *statusSuite) TestValidateBadState(c *gc.C) {
	var status workload.Status
	status.State = "some bogus state"
	err := status.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *statusSuite) TestValidateFailedBadState(c *gc.C) {
	var status workload.Status
	status.Blocker = workload.BlockerFailed
	test := func(state string) {
		c.Logf("checking %q", state)
		status.State = state
		err := status.Validate()

		c.Check(err, jc.Satisfies, errors.IsNotValid)
	}
	for _, state := range initialStates {
		test(state)
	}
	for _, state := range finalStates {
		test(state)
	}
}

func (s *statusSuite) TestValidateFailedOkay(c *gc.C) {
	okay := []string{
		workload.StateStarting,
		workload.StateRunning,
		workload.StateStopping,
	}

	var status workload.Status
	status.Blocker = workload.BlockerFailed
	for _, state := range okay {
		c.Logf("checking %q", state)
		status.State = state
		err := status.Validate()

		c.Check(err, jc.ErrorIsNil)
	}
}

func (s *statusSuite) TestValidateErrorBadState(c *gc.C) {
	var status workload.Status
	status.Blocker = workload.BlockerError
	for _, state := range states {
		if state == workload.StateRunning {
			continue
		}
		c.Logf("checking %q", state)
		status.State = state
		err := status.Validate()

		c.Check(err, jc.Satisfies, errors.IsNotValid)
	}
}

func (s *statusSuite) TestValidateErrorOkay(c *gc.C) {
	status := workload.Status{
		State:   workload.StateRunning,
		Blocker: workload.BlockerError,
	}
	err := status.Validate()

	c.Check(err, jc.ErrorIsNil)
}
