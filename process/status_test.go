// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package process_test

import (
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
		process.StateError,
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

func (s *statusSuite) checkStatus(c *gc.C, status process.Status, state, msg string) {
	c.Check(status.State, gc.Equals, state)
	c.Check(status.Failed, jc.IsFalse)
	c.Check(status.Message, gc.Equals, msg)
}

func (s *statusSuite) TestIsBlockedFalse(c *gc.C) {
	status := s.newStatus(process.StateRunning)
	blocked := status.IsBlocked()

	c.Check(blocked, jc.IsFalse)
}

func (s *statusSuite) TestIsBlockedErrorOnly(c *gc.C) {
	status := s.newStatus(process.StateError)
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
	status := s.newStatus(process.StateError)
	status.Failed = true
	blocked := status.IsBlocked()

	c.Check(blocked, jc.IsTrue)
}

func (s *statusSuite) TestAdvanceTraverse(c *gc.C) {
	status := process.Status{}
	s.checkStatus(c, status, process.StateUndefined, "")

	err := status.Advance("")
	c.Assert(err, jc.ErrorIsNil)
	s.checkStatus(c, status, process.StateDefined, "")

	err = status.Advance("")
	c.Assert(err, jc.ErrorIsNil)
	s.checkStatus(c, status, process.StateStarting, "")

	err = status.Advance("")
	c.Assert(err, jc.ErrorIsNil)
	s.checkStatus(c, status, process.StateRunning, "")

	err = status.Advance("")
	c.Assert(err, jc.ErrorIsNil)
	s.checkStatus(c, status, process.StateStopping, "")

	err = status.Advance("")
	c.Assert(err, jc.ErrorIsNil)
	s.checkStatus(c, status, process.StateStopped, "")

	err = status.Advance("")
	c.Check(err, gc.NotNil)
}

func (s *statusSuite) TestAdvanceMessage(c *gc.C) {
	status := s.newStatus(process.StateDefined)
	c.Assert(status.Message, gc.Equals, "")

	err := status.Advance("good things to come")
	c.Assert(err, jc.ErrorIsNil)
	s.checkStatus(c, status, process.StateStarting, "good things to come")

	err = status.Advance("rock and roll")
	c.Assert(err, jc.ErrorIsNil)
	s.checkStatus(c, status, process.StateRunning, "rock and roll")

	err = status.Advance("")
	c.Assert(err, jc.ErrorIsNil)
	s.checkStatus(c, status, process.StateStopping, "")
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
	status := s.newStatus(process.StateError)
	err := status.Advance("")

	c.Check(err, gc.ErrorMatches, `cannot advance from an error state`)
}

func (s *statusSuite) TestSetFailedOkay(c *gc.C) {
	status := s.newStatus(process.StateRunning)
	status.Message = "good to go"
	err := status.SetFailed("uh-oh")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(status.State, gc.Equals, process.StateRunning)
	c.Check(status.Failed, jc.IsTrue)
	c.Check(status.Message, gc.Equals, "uh-oh")
}

func (s *statusSuite) TestSetFailedDefaultMessage(c *gc.C) {
	status := s.newStatus(process.StateRunning)
	status.Message = "good to go"
	err := status.SetFailed("")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(status.State, gc.Equals, process.StateRunning)
	c.Check(status.Failed, jc.IsTrue)
	c.Check(status.Message, gc.Equals, "problem while interacting with workload process")
}

func (s *statusSuite) TestSetFailedAlreadyFailed(c *gc.C) {
	status := s.newStatus(process.StateRunning)
	status.Message = "good to go"
	err := status.SetFailed("")
	c.Assert(err, jc.ErrorIsNil)
	err = status.SetFailed("uh-oh")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(status.State, gc.Equals, process.StateRunning)
	c.Check(status.Failed, jc.IsTrue)
	c.Check(status.Message, gc.Equals, "uh-oh")
}

func (s *statusSuite) TestSetFailedAlreadyError(c *gc.C) {
	status := s.newStatus(process.StateError)
	err := status.SetFailed("uh-oh")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(status.State, gc.Equals, process.StateError)
	c.Check(status.Failed, jc.IsTrue)
	c.Check(status.Message, gc.Equals, "uh-oh")
}

func (s *statusSuite) TestSetFailedBadState(c *gc.C) {
	status := process.Status{
		Failed:  false,
		Message: "good to go",
	}
	test := func(state, msg string) {
		status.State = state
		err := status.SetFailed("")

		c.Check(err, gc.ErrorMatches, msg)
		s.checkStatus(c, status, state, "good to go")
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

	s.checkStatus(c, status, process.StateError, "uh-oh")
}

func (s *statusSuite) TestSetErrorDefaultMessage(c *gc.C) {
	status := s.newStatus(process.StateRunning)
	status.Message = "good to go"
	err := status.SetError("")
	c.Assert(err, jc.ErrorIsNil)

	s.checkStatus(c, status, process.StateError, "the workload process has an error")
}

func (s *statusSuite) TestSetErrorAlreadyError(c *gc.C) {
	status := s.newStatus(process.StateRunning)
	status.Message = "good to go"
	err := status.SetError("")
	c.Assert(err, jc.ErrorIsNil)
	err = status.SetError("uh-oh")
	c.Assert(err, jc.ErrorIsNil)

	s.checkStatus(c, status, process.StateError, "uh-oh")
}

func (s *statusSuite) TestSetErrorAlreadyFailed(c *gc.C) {
	status := s.newStatus(process.StateRunning)
	status.Failed = true
	status.Message = "good to go"
	err := status.SetError("uh-oh")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(status.State, gc.Equals, process.StateError)
	c.Check(status.Failed, jc.IsTrue)
	c.Check(status.Message, gc.Equals, "uh-oh")
}

func (s *statusSuite) TestSetErrorBadState(c *gc.C) {
	status := s.newStatus(process.StateStarting)
	err := status.SetError("")

	c.Check(err, gc.ErrorMatches, `can error only while running`)
}

func (s *statusSuite) TestResolveErrorOkay(c *gc.C) {
	status := process.Status{
		State:   process.StateError,
		Failed:  false,
		Message: "uh-oh",
	}
	err := status.Resolve("good to go")
	c.Assert(err, jc.ErrorIsNil)

	s.checkStatus(c, status, process.StateRunning, "good to go")
}

func (s *statusSuite) TestResolveErrorDefaultMessage(c *gc.C) {
	status := process.Status{
		State:   process.StateError,
		Failed:  false,
		Message: "uh-oh",
	}
	err := status.Resolve("")
	c.Assert(err, jc.ErrorIsNil)

	s.checkStatus(c, status, process.StateRunning, "error resolved")
}

func (s *statusSuite) TestResolveFailedOkay(c *gc.C) {
	status := process.Status{
		State:   process.StateRunning,
		Failed:  true,
		Message: "uh-oh",
	}
	err := status.Resolve("good to go")
	c.Assert(err, jc.ErrorIsNil)

	s.checkStatus(c, status, process.StateRunning, "good to go")
}

func (s *statusSuite) TestResolveFailedDefaultMessage(c *gc.C) {
	status := process.Status{
		State:   process.StateRunning,
		Failed:  true,
		Message: "uh-oh",
	}
	err := status.Resolve("")
	c.Assert(err, jc.ErrorIsNil)

	s.checkStatus(c, status, process.StateRunning, "failure resolved")
}

func (s *statusSuite) TestResolveErrorAndFailed(c *gc.C) {
	status := process.Status{
		State:   process.StateError,
		Failed:  true,
		Message: "uh-oh",
	}
	err := status.Resolve("")
	c.Assert(err, jc.ErrorIsNil)

	s.checkStatus(c, status, process.StateRunning, "error resolved")
}

func (s *statusSuite) TestResolveNotBlocked(c *gc.C) {
	status := process.Status{
		State:   process.StateRunning,
		Failed:  false,
		Message: "nothing wrong",
	}
	err := status.Resolve("good to go")

	c.Check(err, gc.ErrorMatches, `not in an error or failed state`)
	s.checkStatus(c, status, process.StateRunning, "nothing wrong")
}
