// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package util_test

import (
	"testing"

	jujuerrors "github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/cmd/jujud-controller/util"
	internalerrors "github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	jworker "github.com/juju/juju/internal/worker"
)

func TestAgentDoneSuite(t *testing.T) {
	tc.Run(t, &agentDoneSuite{})
}

type agentDoneSuite struct{}

// TestAgentDoneNilError verifies that a nil error passes through unchanged.
func (s *agentDoneSuite) TestAgentDoneNilError(c *tc.C) {
	logger := loggertesting.WrapCheckLog(c)
	err := util.AgentDone(logger, nil)
	c.Check(err, tc.ErrorIsNil)
}

// TestAgentDoneTerminateAgent verifies that a bare ErrTerminateAgent is
// swallowed so the init system does not restart the agent.
func (s *agentDoneSuite) TestAgentDoneTerminateAgent(c *tc.C) {
	logger := loggertesting.WrapCheckLog(c)
	err := util.AgentDone(logger, jworker.ErrTerminateAgent)
	c.Check(err, tc.ErrorIsNil)
}

// TestAgentDoneRebootMachine verifies that a bare ErrRebootMachine is
// swallowed so the init system does not restart the agent.
func (s *agentDoneSuite) TestAgentDoneRebootMachine(c *tc.C) {
	logger := loggertesting.WrapCheckLog(c)
	err := util.AgentDone(logger, jworker.ErrRebootMachine)
	c.Check(err, tc.ErrorIsNil)
}

// TestAgentDoneShutdownMachine verifies that a bare ErrShutdownMachine is
// swallowed so the init system does not restart the agent.
func (s *agentDoneSuite) TestAgentDoneShutdownMachine(c *tc.C) {
	logger := loggertesting.WrapCheckLog(c)
	err := util.AgentDone(logger, jworker.ErrShutdownMachine)
	c.Check(err, tc.ErrorIsNil)
}

// TestAgentDoneRebootMachineCaptured is the regression test for the original
// bug: when ErrRebootMachine is wrapped by internal/errors.Capture (as it is
// when it travels through core/watcher.NotifyWorker), the old
// errors.Cause()-based switch failed to recognise it and the agent exited
// without executing the reboot. The fix uses errors.Is(), which correctly
// traverses Unwrap() chains produced by Capture.
func (s *agentDoneSuite) TestAgentDoneRebootMachineCaptured(c *tc.C) {
	logger := loggertesting.WrapCheckLog(c)
	wrapped := internalerrors.Capture(jworker.ErrRebootMachine)
	err := util.AgentDone(logger, wrapped)
	c.Check(err, tc.ErrorIsNil)
}

// TestAgentDoneShutdownMachineCaptured mirrors the above for ErrShutdownMachine.
func (s *agentDoneSuite) TestAgentDoneShutdownMachineCaptured(c *tc.C) {
	logger := loggertesting.WrapCheckLog(c)
	wrapped := internalerrors.Capture(jworker.ErrShutdownMachine)
	err := util.AgentDone(logger, wrapped)
	c.Check(err, tc.ErrorIsNil)
}

// TestAgentDoneTerminateAgentCaptured mirrors the above for ErrTerminateAgent.
func (s *agentDoneSuite) TestAgentDoneTerminateAgentCaptured(c *tc.C) {
	logger := loggertesting.WrapCheckLog(c)
	wrapped := internalerrors.Capture(jworker.ErrTerminateAgent)
	err := util.AgentDone(logger, wrapped)
	c.Check(err, tc.ErrorIsNil)
}

// TestAgentDoneRebootMachineTraced verifies that an ErrRebootMachine annotated
// via juju/errors.Trace is also swallowed correctly.
func (s *agentDoneSuite) TestAgentDoneRebootMachineTraced(c *tc.C) {
	logger := loggertesting.WrapCheckLog(c)
	wrapped := jujuerrors.Trace(jworker.ErrRebootMachine)
	err := util.AgentDone(logger, wrapped)
	c.Check(err, tc.ErrorIsNil)
}

// TestAgentDoneUnknownErrorPassesThrough verifies that unrelated errors are
// returned unchanged by AgentDone.
func (s *agentDoneSuite) TestAgentDoneUnknownErrorPassesThrough(c *tc.C) {
	logger := loggertesting.WrapCheckLog(c)
	sentinel := jujuerrors.New("some unexpected error")
	err := util.AgentDone(logger, sentinel)
	c.Check(err, tc.ErrorIs, sentinel)
}
