// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dependency_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
)

type EngineSuite struct {
	testing.IsolationSuite
	engine dependency.Engine
}

var _ = gc.Suite(&EngineSuite{})

func (s *EngineSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.startEngine(c, nothingFatal)
}

func (s *EngineSuite) TearDownTest(c *gc.C) {
	s.stopEngine(c)
	s.IsolationSuite.TearDownTest(c)
}

func (s *EngineSuite) startEngine(c *gc.C, isFatal dependency.IsFatalFunc) {
	s.engine = dependency.NewEngine(isFatal, coretesting.ShortWait/2, coretesting.ShortWait/10)
}

func (s *EngineSuite) stopEngine(c *gc.C) {
	if s.engine != nil {
		err := worker.Stop(s.engine)
		s.engine = nil
		c.Check(err, jc.ErrorIsNil)
	}
}

func (s *EngineSuite) TestInstallNoInputs(c *gc.C) {
	ews := newErrorWorkerStarter()
	err := s.engine.Install("some-task", ews.Manifold())
	c.Assert(err, jc.ErrorIsNil)
	ews.AssertOneStart(c)
}

func (s *EngineSuite) TestInstallUnknownInputs(c *gc.C) {
	ews := newErrorWorkerStarter("unknown-task")
	err := s.engine.Install("some-task", ews.Manifold())
	c.Assert(err, gc.ErrorMatches, "some-task manifold depends on unknown unknown-task manifold")
	ews.AssertNoStart(c)
}

func (s *EngineSuite) TestDoubleInstall(c *gc.C) {
	ews := newErrorWorkerStarter()
	err := s.engine.Install("some-task", ews.Manifold())
	c.Assert(err, jc.ErrorIsNil)
	ews.AssertOneStart(c)

	err = s.engine.Install("some-task", ews.Manifold())
	c.Assert(err, gc.ErrorMatches, "some-task manifold already installed")
	ews.AssertNoStart(c)
}

func (s *EngineSuite) TestInstallAlreadyStopped(c *gc.C) {
	err := worker.Stop(s.engine)
	c.Assert(err, jc.ErrorIsNil)

	ews := newErrorWorkerStarter()
	err = s.engine.Install("some-task", ews.Manifold())
	c.Assert(err, gc.ErrorMatches, "engine is shutting down")
	ews.AssertNoStart(c)
}

func (s *EngineSuite) TestStartGetResourceExistenceOnly(c *gc.C) {

	// Start a task with a dependency.
	ews1 := newErrorWorkerStarter()
	err := s.engine.Install("some-task", ews1.Manifold())
	c.Assert(err, jc.ErrorIsNil)
	ews2 := newErrorWorkerStarter("some-task")
	err = s.engine.Install("other-task", ews2.Manifold())
	c.Assert(err, jc.ErrorIsNil)

	// Each task should successfully start exactly once.
	ews1.AssertOneStart(c)
	ews2.AssertOneStart(c)
}

func (s *EngineSuite) TestStartGetResourceUndeclaredName(c *gc.C) {

	// Install a task and make sure it's started.
	ews1 := newErrorWorkerStarter()
	err := s.engine.Install("some-task", ews1.Manifold())
	c.Assert(err, jc.ErrorIsNil)
	ews1.AssertOneStart(c)

	// Install another task with an undeclared dependency on the started task.
	done := make(chan struct{})
	err = s.engine.Install("other-task", dependency.Manifold{
		Start: func(getResource dependency.GetResourceFunc) (worker.Worker, error) {
			success := getResource("some-task", nil)
			c.Check(success, jc.IsFalse)
			close(done)
			// Return a real worker so we don't keep restarting and potentially double-closing.
			return degenerateStart(getResource)
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	// Wait for the check to complete before we stop.
	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("dependent task never started")
	}
}

func (s *EngineSuite) testStartGetResource(c *gc.C, accept bool) {

	// Start a task with an Output func that checks what it's passed, and wait for it to start.
	var target interface{}
	expectTarget := &target
	ews1 := newErrorWorkerStarter()
	manifold := ews1.Manifold()
	manifold.Output = func(worker worker.Worker, target interface{}) bool {
		// Check we got passed what we expect regardless...
		c.Check(target, gc.DeepEquals, expectTarget)
		// ...and return whether we're meant to accept it or not.
		return accept
	}
	err := s.engine.Install("some-task", manifold)
	c.Assert(err, jc.ErrorIsNil)
	ews1.AssertOneStart(c)

	// Start another that tries to use the above dependency.
	done := make(chan struct{})
	err = s.engine.Install("other-task", dependency.Manifold{
		Inputs: []string{"some-task"},
		Start: func(getResource dependency.GetResourceFunc) (worker.Worker, error) {
			success := getResource("some-task", &target)
			// Check the result from some-task's Output func matches what we expect.
			c.Check(success, gc.Equals, accept)
			close(done)
			// Return a real worker so we don't keep restarting and potentially double-closing.
			return degenerateStart(getResource)
		},
	})
	c.Check(err, jc.ErrorIsNil)

	// Wait for the check to complete before we stop.
	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("other-task never started")
	}
}

func (s *EngineSuite) TestStartGetResourceReject(c *gc.C) {
	s.testStartGetResource(c, false)
}

func (s *EngineSuite) TestStartGetResourceAccept(c *gc.C) {
	s.testStartGetResource(c, true)
}

func (s *EngineSuite) TestErrorRestartsDependents(c *gc.C) {

	// Start two tasks, one dependent on the other.
	ews1 := newErrorWorkerStarter()
	err := s.engine.Install("error-task", ews1.Manifold())
	c.Assert(err, jc.ErrorIsNil)
	ews2 := newErrorWorkerStarter("error-task")
	err = s.engine.Install("some-task", ews2.Manifold())
	c.Assert(err, jc.ErrorIsNil)
	ews1.AssertOneStart(c)
	ews2.AssertOneStart(c)

	// Induce an error in the dependency...
	ews1.InjectError(c, errors.New("ZAP"))

	// ...and check that each task restarts once.
	ews1.AssertOneStart(c)
	ews2.AssertOneStart(c)
}

func (s *EngineSuite) TestErrorPreservesDependencies(c *gc.C) {

	// Start two tasks, one dependent on the other.
	ews1 := newErrorWorkerStarter()
	err := s.engine.Install("some-task", ews1.Manifold())
	c.Assert(err, jc.ErrorIsNil)
	ews2 := newErrorWorkerStarter("some-task")
	err = s.engine.Install("error-task", ews2.Manifold())
	c.Assert(err, jc.ErrorIsNil)
	ews1.AssertOneStart(c)
	ews2.AssertOneStart(c)

	// Induce an error in the dependent...
	ews2.InjectError(c, errors.New("BLAM"))

	// ...and check that only the dependent restarts.
	ews1.AssertNoStart(c)
	ews2.AssertOneStart(c)
}

func (s *EngineSuite) TestCompletedWorkerNotRestartedOnExit(c *gc.C) {

	// Start a task.
	ews1 := newErrorWorkerStarter()
	err := s.engine.Install("stop-task", ews1.Manifold())
	c.Assert(err, jc.ErrorIsNil)
	ews1.AssertOneStart(c)

	// Stop it without error, and check it doesn't start again.
	ews1.InjectError(c, nil)
	ews1.AssertNoStart(c)
}

func (s *EngineSuite) TestCompletedWorkerRestartedByDependencyChange(c *gc.C) {

	// Start a task with a dependency.
	ews1 := newErrorWorkerStarter()
	err := s.engine.Install("some-task", ews1.Manifold())
	c.Assert(err, jc.ErrorIsNil)
	ews2 := newErrorWorkerStarter("some-task")
	err = s.engine.Install("stop-task", ews2.Manifold())
	c.Assert(err, jc.ErrorIsNil)
	ews1.AssertOneStart(c)
	ews2.AssertOneStart(c)

	// Complete the dependent task successfully.
	ews2.InjectError(c, nil)
	ews2.AssertNoStart(c)

	// Bounce the dependency, and check the dependent is started again.
	ews1.InjectError(c, errors.New("CLUNK"))
	ews1.AssertOneStart(c)
	ews2.AssertOneStart(c)
}

func (s *EngineSuite) TestRestartRestartsDependents(c *gc.C) {

	// Start a dependency chain of 3 workers.
	ews1 := newErrorWorkerStarter()
	err := s.engine.Install("error-task", ews1.Manifold())
	c.Assert(err, jc.ErrorIsNil)
	ews2 := newErrorWorkerStarter("error-task")
	err = s.engine.Install("restart-task", ews2.Manifold())
	c.Assert(err, jc.ErrorIsNil)
	ews3 := newErrorWorkerStarter("restart-task")
	err = s.engine.Install("consequent-restart-task", ews3.Manifold())
	c.Assert(err, jc.ErrorIsNil)
	ews1.AssertOneStart(c)
	ews2.AssertOneStart(c)
	ews3.AssertOneStart(c)

	// Once they're all running, induce an error at the top level, which will
	// cause the next level to be killed cleanly....
	ews1.InjectError(c, errors.New("ZAP"))

	// ...but should still cause all 3 workers to bounce.
	ews1.AssertOneStart(c)
	ews2.AssertOneStart(c)
	ews3.AssertOneStart(c)
}

func (s *EngineSuite) TestIsFatal(c *gc.C) {

	// Start an engine that pays attention to fatal errors.
	fatalError := errors.New("KABOOM")
	s.stopEngine(c)
	s.startEngine(c, func(err error) bool {
		return err == fatalError
	})

	// Start two independent workers.
	ews1 := newErrorWorkerStarter()
	err := s.engine.Install("some-task", ews1.Manifold())
	c.Assert(err, jc.ErrorIsNil)
	ews2 := newErrorWorkerStarter()
	err = s.engine.Install("other-task", ews2.Manifold())
	c.Assert(err, jc.ErrorIsNil)
	ews1.AssertOneStart(c)
	ews2.AssertOneStart(c)

	// Bounce one worker with Just Some Error; check that worker bounces.
	ews1.InjectError(c, errors.New("splort"))
	ews1.AssertOneStart(c)
	ews2.AssertNoStart(c)

	// Bounce another worker with the fatal error; check the engine exist with
	// the right error.
	ews2.InjectError(c, fatalError)
	ews1.AssertNoStart(c)
	ews2.AssertNoStart(c)
	err = worker.Stop(s.engine)
	c.Assert(err, gc.Equals, fatalError)

	// Clear out s.engine -- lest TearDownTest freak out about the error.
	s.engine = nil
}

func (s *EngineSuite) TestErrUnmetDependencies(c *gc.C) {

	// ErrUnmetDependencies is implicitly and indirectly tested by the
	// default errorWorkerStarter.start method throughout this suite, but
	// this test explores its behaviour in pathological cases.

	// Start a simple dependency.
	ews1 := newErrorWorkerStarter()
	err := s.engine.Install("some-task", ews1.Manifold())
	c.Assert(err, jc.ErrorIsNil)
	ews1.AssertOneStart(c)

	// Start a dependent that always complains ErrUnmetDependencies.
	ews2 := newErrorWorkerStarter("some-task")
	manifold := ews2.Manifold()
	manifold.Start = func(_ dependency.GetResourceFunc) (worker.Worker, error) {
		ews2.starts <- struct{}{}
		return nil, dependency.ErrUnmetDependencies
	}
	err = s.engine.Install("unmet-task", manifold)
	c.Assert(err, jc.ErrorIsNil)
	ews2.AssertOneStart(c)

	// Bounce the dependency; check the dependent bounces once or twice (it will
	// react to both the stop and the start of the dependency, but may be lucky
	// enough to only restart once).
	ews1.InjectError(c, errors.New("kerrang"))
	ews1.AssertOneStart(c)
	startCount := 0
	stable := false
	for !stable {
		select {
		case <-ews2.starts:
			startCount++
		case <-time.After(coretesting.ShortWait):
			stable = true
		}
	}
	c.Logf("saw %d starts", startCount)
	c.Assert(startCount > 0, jc.IsTrue)
	c.Assert(startCount < 3, jc.IsTrue)

	// Stop the dependency for good; check the dependent is restarted just once.
	ews1.InjectError(c, nil)
	ews1.AssertNoStart(c)
	ews2.AssertOneStart(c)
}
