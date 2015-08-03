// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package process_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/process"
	"github.com/juju/juju/testing"
)

var (
	states = []string{
		process.StateUndefined,
		process.StateDefined,
		process.StateStarting,
		process.StateRunning,
		process.StateStopping,
		process.StateStopped,
	}

	initialStates = []string{
		process.StateUndefined,
		process.StateDefined,
	}

	finalStates = []string{
		process.StateStopped,
	}
)

type statusSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&statusSuite{})

func (s *statusSuite) newStatus(state string) process.Status {
	return process.Status{
		State: state,
	}
}

func (s *statusSuite) checkStatus(c *gc.C, status process.Status, state, msg string, failed, err bool) {
	c.Check(status.State, gc.Equals, state)
	c.Check(status.Failed, gc.Equals, failed)
	c.Check(status.Error, gc.Equals, err)
	c.Check(status.Message, gc.Equals, msg)
}

func (s *statusSuite) checkStatusOkay(c *gc.C, status process.Status, state, msg string) {
	s.checkStatus(c, status, state, msg, false, false)
}

func (s *statusSuite) TestStringOkay(c *gc.C) {
	var status process.Status
	status.Message = "nothing to see here"
	for _, state := range states {
		c.Logf("checking %q", state)
		status.State = state
		str := status.String()

		c.Check(str, gc.Equals, "nothing to see here")
	}
}

func (s *statusSuite) TestStringNoMessage(c *gc.C) {
	var status process.Status
	for _, state := range states {
		c.Logf("checking %q", state)
		status.State = state
		str := status.String()

		c.Check(str, gc.Equals, "<no message>")
	}
}

func (s *statusSuite) TestStringFailed(c *gc.C) {
	status := process.Status{
		State:   process.StateRunning,
		Failed:  true,
		Message: "uh-oh",
	}
	str := status.String()

	c.Check(str, gc.Equals, "(failed) uh-oh")
}

func (s *statusSuite) TestStringError(c *gc.C) {
	status := process.Status{
		State:   process.StateRunning,
		Error:   true,
		Message: "uh-oh",
	}
	str := status.String()

	c.Check(str, gc.Equals, "(error) uh-oh")
}

func (s *statusSuite) TestStringFailedAndError(c *gc.C) {
	status := process.Status{
		State:   process.StateRunning,
		Failed:  true,
		Error:   true,
		Message: "uh-oh",
	}
	str := status.String()

	c.Check(str, gc.Equals, "(failed) uh-oh")
}

func (s *statusSuite) TestIsBlockedFalse(c *gc.C) {
	status := s.newStatus(process.StateRunning)
	blocked := status.IsBlocked()

	c.Check(blocked, jc.IsFalse)
}

func (s *statusSuite) TestIsBlockedErrorOnly(c *gc.C) {
	status := s.newStatus(process.StateRunning)
	status.Error = true
	blocked := status.IsBlocked()

	c.Check(blocked, jc.IsTrue)
}

func (s *statusSuite) TestIsBlockedFailedOnly(c *gc.C) {
	status := s.newStatus(process.StateRunning)
	status.Failed = true
	blocked := status.IsBlocked()

	c.Check(blocked, jc.IsTrue)
}

func (s *statusSuite) TestIsBlockedErrorAndFailed(c *gc.C) {
	status := s.newStatus(process.StateRunning)
	status.Error = true
	status.Failed = true
	blocked := status.IsBlocked()

	c.Check(blocked, jc.IsTrue)
}

func (s *statusSuite) TestAdvanceTraverse(c *gc.C) {
	status := process.Status{}
	s.checkStatusOkay(c, status, process.StateUndefined, "")

	err := status.Advance("")
	c.Assert(err, jc.ErrorIsNil)
	s.checkStatusOkay(c, status, process.StateDefined, "")

	err = status.Advance("")
	c.Assert(err, jc.ErrorIsNil)
	s.checkStatusOkay(c, status, process.StateStarting, "")

	err = status.Advance("")
	c.Assert(err, jc.ErrorIsNil)
	s.checkStatusOkay(c, status, process.StateRunning, "")

	err = status.Advance("")
	c.Assert(err, jc.ErrorIsNil)
	s.checkStatusOkay(c, status, process.StateStopping, "")

	err = status.Advance("")
	c.Assert(err, jc.ErrorIsNil)
	s.checkStatusOkay(c, status, process.StateStopped, "")

	err = status.Advance("")
	c.Check(err, gc.NotNil)
}

func (s *statusSuite) TestAdvanceMessage(c *gc.C) {
	status := s.newStatus(process.StateDefined)
	c.Assert(status.Message, gc.Equals, "")

	err := status.Advance("good things to come")
	c.Assert(err, jc.ErrorIsNil)
	s.checkStatusOkay(c, status, process.StateStarting, "good things to come")

	err = status.Advance("rock and roll")
	c.Assert(err, jc.ErrorIsNil)
	s.checkStatusOkay(c, status, process.StateRunning, "rock and roll")

	err = status.Advance("")
	c.Assert(err, jc.ErrorIsNil)
	s.checkStatusOkay(c, status, process.StateStopping, "")
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

func (s *statusSuite) TestAdvanceFailed(c *gc.C) {
	for _, state := range states {
		c.Logf("checking %q", state)
		status := s.newStatus(state)
		status.Failed = true
		err := status.Advance("")

		c.Check(err, gc.ErrorMatches, `cannot advance from a failed state`)
	}
}

func (s *statusSuite) TestAdvanceError(c *gc.C) {
	for _, state := range states {
		c.Logf("checking %q", state)
		status := s.newStatus(state)
		status.Error = true
		err := status.Advance("")

		c.Check(err, gc.ErrorMatches, `cannot advance from an error state`)
	}
}

func (s *statusSuite) TestAdvanceErrorAndFailed(c *gc.C) {
	for _, state := range states {
		c.Logf("checking %q", state)
		status := s.newStatus(state)
		status.Failed = true
		status.Error = true
		err := status.Advance("")

		c.Check(err, gc.ErrorMatches, `cannot advance from a failed state`)
	}
}

func (s *statusSuite) TestSetFailedOkay(c *gc.C) {
	status := s.newStatus(process.StateRunning)
	status.Message = "good to go"
	err := status.SetFailed("uh-oh")
	c.Assert(err, jc.ErrorIsNil)

	s.checkStatus(c, status, process.StateRunning, "uh-oh", true, false)
}

func (s *statusSuite) TestSetFailedDefaultMessage(c *gc.C) {
	status := s.newStatus(process.StateRunning)
	status.Message = "good to go"
	err := status.SetFailed("")
	c.Assert(err, jc.ErrorIsNil)

	msg := "problem while interacting with workload process"
	s.checkStatus(c, status, process.StateRunning, msg, true, false)
}

func (s *statusSuite) TestSetFailedAlreadyFailed(c *gc.C) {
	status := s.newStatus(process.StateRunning)
	status.Message = "good to go"
	err := status.SetFailed("")
	c.Assert(err, jc.ErrorIsNil)
	err = status.SetFailed("uh-oh")
	c.Assert(err, jc.ErrorIsNil)

	s.checkStatus(c, status, process.StateRunning, "uh-oh", true, false)
}

func (s *statusSuite) TestSetFailedAlreadyError(c *gc.C) {
	status := s.newStatus(process.StateRunning)
	status.Error = true
	err := status.SetFailed("uh-oh")
	c.Assert(err, jc.ErrorIsNil)

	s.checkStatus(c, status, process.StateRunning, "uh-oh", true, true)
}

func (s *statusSuite) TestSetFailedBadState(c *gc.C) {
	status := process.Status{
		Failed:  false,
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
	status := s.newStatus(process.StateRunning)
	status.Message = "good to go"
	err := status.SetError("uh-oh")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(status.State, gc.Equals, process.StateRunning)
	c.Check(status.Failed, jc.IsFalse)
	c.Check(status.Error, jc.IsTrue)
	c.Check(status.Message, gc.Equals, "uh-oh")
	s.checkStatus(c, status, process.StateRunning, "uh-oh", false, true)
}

func (s *statusSuite) TestSetErrorDefaultMessage(c *gc.C) {
	status := s.newStatus(process.StateRunning)
	status.Message = "good to go"
	err := status.SetError("")
	c.Assert(err, jc.ErrorIsNil)

	msg := "the workload process has an error"
	s.checkStatus(c, status, process.StateRunning, msg, false, true)
}

func (s *statusSuite) TestSetErrorAlreadyError(c *gc.C) {
	status := s.newStatus(process.StateRunning)
	status.Message = "good to go"
	err := status.SetError("")
	c.Assert(err, jc.ErrorIsNil)
	err = status.SetError("uh-oh")
	c.Assert(err, jc.ErrorIsNil)

	s.checkStatus(c, status, process.StateRunning, "uh-oh", false, true)
}

func (s *statusSuite) TestSetErrorAlreadyFailed(c *gc.C) {
	status := s.newStatus(process.StateRunning)
	status.Failed = true
	status.Message = "good to go"
	err := status.SetError("uh-oh")
	c.Assert(err, jc.ErrorIsNil)

	s.checkStatus(c, status, process.StateRunning, "uh-oh", true, true)
}

func (s *statusSuite) TestSetErrorBadState(c *gc.C) {
	status := s.newStatus(process.StateStarting)
	err := status.SetError("")

	c.Check(err, gc.ErrorMatches, `can error only while running`)
}

func (s *statusSuite) TestResolveErrorOkay(c *gc.C) {
	status := process.Status{
		State:   process.StateRunning,
		Error:   true,
		Message: "uh-oh",
	}
	err := status.Resolve("good to go")
	c.Assert(err, jc.ErrorIsNil)

	s.checkStatusOkay(c, status, process.StateRunning, "good to go")
}

func (s *statusSuite) TestResolveErrorDefaultMessage(c *gc.C) {
	status := process.Status{
		State:   process.StateRunning,
		Error:   true,
		Message: "uh-oh",
	}
	err := status.Resolve("")
	c.Assert(err, jc.ErrorIsNil)

	s.checkStatusOkay(c, status, process.StateRunning, "error resolved")
}

func (s *statusSuite) TestResolveFailedOkay(c *gc.C) {
	status := process.Status{
		State:   process.StateRunning,
		Failed:  true,
		Message: "uh-oh",
	}
	err := status.Resolve("good to go")
	c.Assert(err, jc.ErrorIsNil)

	s.checkStatusOkay(c, status, process.StateRunning, "good to go")
}

func (s *statusSuite) TestResolveFailedDefaultMessage(c *gc.C) {
	status := process.Status{
		State:   process.StateRunning,
		Failed:  true,
		Message: "uh-oh",
	}
	err := status.Resolve("")
	c.Assert(err, jc.ErrorIsNil)

	s.checkStatusOkay(c, status, process.StateRunning, "failure resolved")
}

func (s *statusSuite) TestResolveErrorAndFailed(c *gc.C) {
	status := process.Status{
		State:   process.StateRunning,
		Failed:  true,
		Error:   true,
		Message: "uh-oh",
	}
	err := status.Resolve("")
	c.Assert(err, jc.ErrorIsNil)

	s.checkStatusOkay(c, status, process.StateRunning, "failure resolved")
}

func (s *statusSuite) TestResolveNotBlocked(c *gc.C) {
	status := process.Status{
		State:   process.StateRunning,
		Failed:  false,
		Error:   false,
		Message: "nothing wrong",
	}
	err := status.Resolve("good to go")

	c.Check(err, gc.ErrorMatches, `not in an error or failed state`)
	s.checkStatusOkay(c, status, process.StateRunning, "nothing wrong")
}

func (s *statusSuite) TestValidateOkay(c *gc.C) {
	status := process.Status{
		Failed:  false,
		Error:   false,
		Message: "nothing wrong",
	}
	for _, state := range states {
		if state == process.StateUndefined {
			continue
		}
		c.Logf("checking %q", state)
		status.State = state
		err := status.Validate()

		c.Check(err, jc.ErrorIsNil)
	}
}

func (s *statusSuite) TestValidateNoMessage(c *gc.C) {
	status := process.Status{
		State:   process.StateRunning,
		Message: "",
	}
	err := status.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *statusSuite) TestValidateUndefinedState(c *gc.C) {
	var status process.Status
	err := status.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *statusSuite) TestValidateBadState(c *gc.C) {
	var status process.Status
	status.State = "some bogus state"
	err := status.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *statusSuite) TestValidateFailedBadState(c *gc.C) {
	var status process.Status
	status.Failed = true
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
		process.StateStarting,
		process.StateRunning,
		process.StateStopping,
	}

	var status process.Status
	status.Failed = true
	for _, state := range okay {
		c.Logf("checking %q", state)
		status.State = state
		err := status.Validate()

		c.Check(err, jc.ErrorIsNil)
	}
}

func (s *statusSuite) TestValidateErrorBadState(c *gc.C) {
	var status process.Status
	status.Error = true
	for _, state := range states {
		if state == process.StateRunning {
			continue
		}
		c.Logf("checking %q", state)
		status.State = state
		err := status.Validate()

		c.Check(err, jc.Satisfies, errors.IsNotValid)
	}
}

func (s *statusSuite) TestValidateErrorOkay(c *gc.C) {
	status := process.Status{
		State: process.StateRunning,
		Error: true,
	}
	err := status.Validate()

	c.Check(err, jc.ErrorIsNil)
}
