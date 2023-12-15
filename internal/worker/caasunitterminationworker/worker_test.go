// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitterminationworker_test

import (
	"os"
	"testing"

	"github.com/juju/clock"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/agent/caasapplication"
	"github.com/juju/juju/internal/worker/caasunitterminationworker"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

var _ = gc.Suite(&TerminationWorkerSuite{})

type TerminationWorkerSuite struct {
	state      *mockState
	terminator *mockTerminator
}

func (s *TerminationWorkerSuite) newWorker(c *gc.C, willRestart bool) worker.Worker {
	s.state = &mockState{
		termination: caasapplication.UnitTermination{
			WillRestart: willRestart,
		},
	}
	s.terminator = &mockTerminator{}
	config := caasunitterminationworker.Config{
		Agent:          &mockAgent{},
		Logger:         loggo.GetLogger("test"),
		Clock:          clock.WallClock,
		State:          s.state,
		UnitTerminator: s.terminator,
	}
	return caasunitterminationworker.NewWorker(config)
}

func (s *TerminationWorkerSuite) TestStartStop(c *gc.C) {
	w := s.newWorker(c, false)
	w.Kill()
	err := w.Wait()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *TerminationWorkerSuite) TestAgentWillRestart(c *gc.C) {
	w := s.newWorker(c, true)
	proc, err := os.FindProcess(os.Getpid())
	c.Assert(err, jc.ErrorIsNil)
	defer proc.Release()
	err = proc.Signal(caasunitterminationworker.TerminationSignal)
	c.Assert(err, jc.ErrorIsNil)
	err = w.Wait()
	c.Assert(err, jc.ErrorIsNil)
	s.state.CheckCallNames(c, "UnitTerminating")
	c.Assert(s.state.Calls()[0].Args[0], gc.DeepEquals, names.NewUnitTag("gitlab/0"))
	s.terminator.CheckCallNames(c, "Terminate")
}

func (s *TerminationWorkerSuite) TestAgentDying(c *gc.C) {
	w := s.newWorker(c, false)
	proc, err := os.FindProcess(os.Getpid())
	c.Assert(err, jc.ErrorIsNil)
	defer proc.Release()
	err = proc.Signal(caasunitterminationworker.TerminationSignal)
	c.Assert(err, jc.ErrorIsNil)
	err = w.Wait()
	c.Assert(err, jc.ErrorIsNil)
	s.state.CheckCallNames(c, "UnitTerminating")
	c.Assert(s.state.Calls()[0].Args[0], gc.DeepEquals, names.NewUnitTag("gitlab/0"))
	s.terminator.CheckCallNames(c)
}

type mockAgent struct {
	agent.Agent
}

func (a *mockAgent) CurrentConfig() agent.Config {
	return &mockAgentConfig{}
}

type mockAgentConfig struct {
	agent.Config
}

func (c *mockAgentConfig) Tag() names.Tag {
	return names.NewUnitTag("gitlab/0")
}

type mockState struct {
	jujutesting.Stub

	termination caasapplication.UnitTermination
}

func (s *mockState) UnitTerminating(tag names.UnitTag) (caasapplication.UnitTermination, error) {
	s.MethodCall(s, "UnitTerminating", tag)
	return s.termination, s.NextErr()
}

type mockTerminator struct {
	jujutesting.Stub
}

func (t *mockTerminator) Terminate() error {
	t.MethodCall(t, "Terminate")
	return t.NextErr()
}
