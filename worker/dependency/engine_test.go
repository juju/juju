// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dependency_test

import (
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
)

type EngineSuite struct {
	engineFixture
}

var _ = gc.Suite(&EngineSuite{})

func (s *EngineSuite) TestInstallConvenienceWrapper(c *gc.C) {
	mh1 := newManifoldHarness()
	mh2 := newManifoldHarness()
	mh3 := newManifoldHarness()

	err := dependency.Install(s.engine, dependency.Manifolds{
		"mh1": mh1.Manifold(),
		"mh2": mh2.Manifold(),
		"mh3": mh3.Manifold(),
	})
	c.Assert(err, jc.ErrorIsNil)

	mh1.AssertOneStart(c)
	mh2.AssertOneStart(c)
	mh3.AssertOneStart(c)
}

func (s *EngineSuite) TestInstallNoInputs(c *gc.C) {

	// Install a worker, check it starts.
	mh1 := newManifoldHarness()
	err := s.engine.Install("some-task", mh1.Manifold())
	c.Assert(err, jc.ErrorIsNil)
	mh1.AssertOneStart(c)

	// Install a second independent worker; check the first in untouched.
	mh2 := newManifoldHarness()
	err = s.engine.Install("other-task", mh2.Manifold())
	c.Assert(err, jc.ErrorIsNil)
	mh2.AssertOneStart(c)
	mh1.AssertNoStart(c)
}

func (s *EngineSuite) TestInstallUnknownInputs(c *gc.C) {

	// Install a worker with an unmet dependency, check it doesn't start
	// (because the implementation returns ErrMissing).
	mh1 := newManifoldHarness("later-task")
	err := s.engine.Install("some-task", mh1.Manifold())
	c.Assert(err, jc.ErrorIsNil)
	mh1.AssertNoStart(c)

	// Install its dependency; check both start.
	mh2 := newManifoldHarness()
	err = s.engine.Install("later-task", mh2.Manifold())
	c.Assert(err, jc.ErrorIsNil)
	mh2.AssertOneStart(c)
	mh1.AssertOneStart(c)
}

func (s *EngineSuite) TestDoubleInstall(c *gc.C) {

	// Install a worker.
	mh := newManifoldHarness()
	err := s.engine.Install("some-task", mh.Manifold())
	c.Assert(err, jc.ErrorIsNil)
	mh.AssertOneStart(c)

	// Can't install another worker with the same name.
	err = s.engine.Install("some-task", mh.Manifold())
	c.Assert(err, gc.ErrorMatches, `"some-task" manifold already installed`)
	mh.AssertNoStart(c)
}

func (s *EngineSuite) TestInstallCycle(c *gc.C) {

	// Install a worker with an unmet dependency.
	mh1 := newManifoldHarness("robin-hood")
	err := s.engine.Install("friar-tuck", mh1.Manifold())
	c.Assert(err, jc.ErrorIsNil)
	mh1.AssertNoStart(c)

	// Can't install another worker that creates a dependency cycle.
	mh2 := newManifoldHarness("friar-tuck")
	err = s.engine.Install("robin-hood", mh2.Manifold())
	c.Assert(err, gc.ErrorMatches, `cannot install "robin-hood" manifold: cycle detected at .*`)
	mh2.AssertNoStart(c)
}

func (s *EngineSuite) TestInstallAlreadyStopped(c *gc.C) {

	// Shut down the engine.
	err := worker.Stop(s.engine)
	c.Assert(err, jc.ErrorIsNil)

	// Can't start a new task.
	mh := newManifoldHarness()
	err = s.engine.Install("some-task", mh.Manifold())
	c.Assert(err, gc.ErrorMatches, "engine is shutting down")
	mh.AssertNoStart(c)
}

func (s *EngineSuite) TestStartGetResourceExistenceOnly(c *gc.C) {

	// Start a task with a dependency.
	mh1 := newManifoldHarness()
	err := s.engine.Install("some-task", mh1.Manifold())
	c.Assert(err, jc.ErrorIsNil)
	mh1.AssertOneStart(c)

	// Start another task that depends on it, ourselves depending on the
	// implementation of manifoldHarness, which calls getResource(foo, nil).
	mh2 := newManifoldHarness("some-task")
	err = s.engine.Install("other-task", mh2.Manifold())
	c.Assert(err, jc.ErrorIsNil)
	mh2.AssertOneStart(c)
}

func (s *EngineSuite) TestStartGetResourceUndeclaredName(c *gc.C) {

	// Install a task and make sure it's started.
	mh1 := newManifoldHarness()
	err := s.engine.Install("some-task", mh1.Manifold())
	c.Assert(err, jc.ErrorIsNil)
	mh1.AssertOneStart(c)

	// Install another task with an undeclared dependency on the started task.
	done := make(chan struct{})
	err = s.engine.Install("other-task", dependency.Manifold{
		Start: func(getResource dependency.GetResourceFunc) (worker.Worker, error) {
			err := getResource("some-task", nil)
			c.Check(err, gc.Equals, dependency.ErrMissing)
			close(done)
			// Return a real worker so we don't keep restarting and potentially double-closing.
			return startMinimalWorker(getResource)
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

func (s *EngineSuite) testStartGetResource(c *gc.C, outErr error) {

	// Start a task with an Output func that checks what it's passed, and wait for it to start.
	var target interface{}
	expectTarget := &target
	mh1 := newManifoldHarness()
	manifold := mh1.Manifold()
	manifold.Output = func(worker worker.Worker, target interface{}) error {
		// Check we got passed what we expect regardless...
		c.Check(target, gc.DeepEquals, expectTarget)
		// ...and return the configured error.
		return outErr
	}
	err := s.engine.Install("some-task", manifold)
	c.Assert(err, jc.ErrorIsNil)
	mh1.AssertOneStart(c)

	// Start another that tries to use the above dependency.
	done := make(chan struct{})
	err = s.engine.Install("other-task", dependency.Manifold{
		Inputs: []string{"some-task"},
		Start: func(getResource dependency.GetResourceFunc) (worker.Worker, error) {
			err := getResource("some-task", &target)
			// Check the result from some-task's Output func matches what we expect.
			c.Check(err, gc.Equals, outErr)
			close(done)
			// Return a real worker so we don't keep restarting and potentially double-closing.
			return startMinimalWorker(getResource)
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

func (s *EngineSuite) TestStartGetResourceAccept(c *gc.C) {
	s.testStartGetResource(c, nil)
}

func (s *EngineSuite) TestStartGetResourceReject(c *gc.C) {
	s.testStartGetResource(c, errors.New("not good enough"))
}

func (s *EngineSuite) TestErrorRestartsDependents(c *gc.C) {

	// Start two tasks, one dependent on the other.
	mh1 := newManifoldHarness()
	err := s.engine.Install("error-task", mh1.Manifold())
	c.Assert(err, jc.ErrorIsNil)
	mh1.AssertOneStart(c)

	mh2 := newManifoldHarness("error-task")
	err = s.engine.Install("some-task", mh2.Manifold())
	c.Assert(err, jc.ErrorIsNil)
	mh2.AssertOneStart(c)

	// Induce an error in the dependency...
	mh1.InjectError(c, errors.New("ZAP"))

	// ...and check that each task restarts once.
	mh1.AssertOneStart(c)
	mh2.AssertOneStart(c)
}

func (s *EngineSuite) TestErrorPreservesDependencies(c *gc.C) {

	// Start two tasks, one dependent on the other.
	mh1 := newManifoldHarness()
	err := s.engine.Install("some-task", mh1.Manifold())
	c.Assert(err, jc.ErrorIsNil)
	mh1.AssertOneStart(c)
	mh2 := newManifoldHarness("some-task")
	err = s.engine.Install("error-task", mh2.Manifold())
	c.Assert(err, jc.ErrorIsNil)
	mh2.AssertOneStart(c)

	// Induce an error in the dependent...
	mh2.InjectError(c, errors.New("BLAM"))

	// ...and check that only the dependent restarts.
	mh1.AssertNoStart(c)
	mh2.AssertOneStart(c)
}

func (s *EngineSuite) TestCompletedWorkerNotRestartedOnExit(c *gc.C) {

	// Start a task.
	mh1 := newManifoldHarness()
	err := s.engine.Install("stop-task", mh1.Manifold())
	c.Assert(err, jc.ErrorIsNil)
	mh1.AssertOneStart(c)

	// Stop it without error, and check it doesn't start again.
	mh1.InjectError(c, nil)
	mh1.AssertNoStart(c)
}

func (s *EngineSuite) TestCompletedWorkerRestartedByDependencyChange(c *gc.C) {

	// Start a task with a dependency.
	mh1 := newManifoldHarness()
	err := s.engine.Install("some-task", mh1.Manifold())
	c.Assert(err, jc.ErrorIsNil)
	mh1.AssertOneStart(c)
	mh2 := newManifoldHarness("some-task")
	err = s.engine.Install("stop-task", mh2.Manifold())
	c.Assert(err, jc.ErrorIsNil)
	mh2.AssertOneStart(c)

	// Complete the dependent task successfully.
	mh2.InjectError(c, nil)
	mh2.AssertNoStart(c)

	// Bounce the dependency, and check the dependent is started again.
	mh1.InjectError(c, errors.New("CLUNK"))
	mh1.AssertOneStart(c)
	mh2.AssertOneStart(c)
}

func (s *EngineSuite) TestRestartRestartsDependents(c *gc.C) {

	// Start a dependency chain of 3 workers.
	mh1 := newManifoldHarness()
	err := s.engine.Install("error-task", mh1.Manifold())
	c.Assert(err, jc.ErrorIsNil)
	mh1.AssertOneStart(c)
	mh2 := newManifoldHarness("error-task")
	err = s.engine.Install("restart-task", mh2.Manifold())
	c.Assert(err, jc.ErrorIsNil)
	mh2.AssertOneStart(c)
	mh3 := newManifoldHarness("restart-task")
	err = s.engine.Install("consequent-restart-task", mh3.Manifold())
	c.Assert(err, jc.ErrorIsNil)
	mh3.AssertOneStart(c)

	// Once they're all running, induce an error at the top level, which will
	// cause the next level to be killed cleanly....
	mh1.InjectError(c, errors.New("ZAP"))

	// ...but should still cause all 3 workers to bounce.
	mh1.AssertOneStart(c)
	mh2.AssertOneStart(c)
	mh3.AssertOneStart(c)
}

func (s *EngineSuite) TestIsFatal(c *gc.C) {

	// Start an engine that pays attention to fatal errors.
	fatalError := errors.New("KABOOM")
	s.stopEngine(c)
	s.startEngine(c, func(err error) bool {
		return err == fatalError
	})

	// Start two independent workers.
	mh1 := newManifoldHarness()
	err := s.engine.Install("some-task", mh1.Manifold())
	c.Assert(err, jc.ErrorIsNil)
	mh1.AssertOneStart(c)
	mh2 := newManifoldHarness()
	err = s.engine.Install("other-task", mh2.Manifold())
	c.Assert(err, jc.ErrorIsNil)
	mh2.AssertOneStart(c)

	// Bounce one worker with Just Some Error; check that worker bounces.
	mh1.InjectError(c, errors.New("splort"))
	mh1.AssertOneStart(c)
	mh2.AssertNoStart(c)

	// Bounce another worker with the fatal error; check the engine exits with
	// the right error.
	mh2.InjectError(c, fatalError)
	mh1.AssertNoStart(c)
	mh2.AssertNoStart(c)
	err = worker.Stop(s.engine)
	c.Assert(err, gc.Equals, fatalError)

	// Clear out s.engine -- lest TearDownTest freak out about the error.
	s.engine = nil
}

func (s *EngineSuite) TestErrMissing(c *gc.C) {

	// ErrMissing is implicitly and indirectly tested by the default
	// manifoldHarness.start method throughout this suite, but this
	// test explores its behaviour in pathological cases.

	// Start a simple dependency.
	mh1 := newManifoldHarness()
	err := s.engine.Install("some-task", mh1.Manifold())
	c.Assert(err, jc.ErrorIsNil)
	mh1.AssertOneStart(c)

	// Start a dependent that always complains ErrMissing.
	mh2 := newManifoldHarness("some-task")
	manifold := mh2.Manifold()
	manifold.Start = func(_ dependency.GetResourceFunc) (worker.Worker, error) {
		mh2.starts <- struct{}{}
		return nil, dependency.ErrMissing
	}
	err = s.engine.Install("unmet-task", manifold)
	c.Assert(err, jc.ErrorIsNil)
	mh2.AssertOneStart(c)

	// Bounce the dependency; check the dependent bounces once or twice (it will
	// react to both the stop and the start of the dependency, but may be lucky
	// enough to only restart once).
	mh1.InjectError(c, errors.New("kerrang"))
	mh1.AssertOneStart(c)
	startCount := 0
	stable := false
	for !stable {
		select {
		case <-mh2.starts:
			startCount++
		case <-time.After(coretesting.ShortWait):
			stable = true
		}
	}
	c.Logf("saw %d starts", startCount)
	c.Assert(startCount, jc.GreaterThan, 0)
	c.Assert(startCount, jc.LessThan, 3)

	// Stop the dependency for good; check the dependent is restarted just once.
	mh1.InjectError(c, nil)
	mh1.AssertNoStart(c)
	mh2.AssertOneStart(c)
}

func (s *EngineSuite) TestErrBounce(c *gc.C) {

	// Start a simple dependency.
	mh1 := newManifoldHarness()
	err := s.engine.Install("some-task", mh1.Manifold())
	c.Assert(err, jc.ErrorIsNil)
	mh1.AssertOneStart(c)

	// Start its dependent.
	mh2 := newResourceIgnoringManifoldHarness("some-task")
	err = s.engine.Install("another-task", mh2.Manifold())
	c.Assert(err, jc.ErrorIsNil)
	mh2.AssertOneStart(c)

	// The parent requests bounce causing both to restart.
	// Note(mjs): the lack of a restart delay is not specifically
	// tested as I can't think of a reliable way to do this.
	mh1.InjectError(c, dependency.ErrBounce)
	mh1.AssertOneStart(c)
	mh2.AssertStart(c) // Might restart more than once
}

func (s *EngineSuite) TestErrUninstall(c *gc.C) {

	// Start a simple dependency.
	mh1 := newManifoldHarness()
	err := s.engine.Install("some-task", mh1.Manifold())
	c.Assert(err, jc.ErrorIsNil)
	mh1.AssertOneStart(c)

	// Start its dependent. Note that in this case we want to record all start
	// attempts, even if there are resource errors.
	mh2 := newResourceIgnoringManifoldHarness("some-task")
	err = s.engine.Install("another-task", mh2.Manifold())
	c.Assert(err, jc.ErrorIsNil)
	mh2.AssertOneStart(c)

	// Uninstall the dependency; it should not be restarted, but its dependent should.
	mh1.InjectError(c, dependency.ErrUninstall)
	mh1.AssertNoStart(c)
	mh2.AssertOneStart(c)

	// Installing a new some-task manifold restarts the dependent.
	mh3 := newManifoldHarness()
	err = s.engine.Install("some-task", mh3.Manifold())
	c.Assert(err, jc.ErrorIsNil)
	mh3.AssertOneStart(c)
	mh2.AssertOneStart(c)
}

// TestWorstError starts an engine with two manifolds that always error
// with fatal errors. We test that the most important error is the one
// returned by the engine.
//
// This test uses manifolds whose workers ignore kill requests. We want
// this (dangerous!) behaviour so that we don't race over which fatal
// error is seen by the engine first.
func (s *EngineSuite) TestWorstError(c *gc.C) {
	// Setup the errors, their importance, and the function
	// that decides.
	importantError := errors.New("an important error")
	moreImportant := func(_, _ error) error {
		return importantError
	}

	allFatal := func(error) bool { return true }

	// Start a new engine with moreImportant configured
	config := dependency.EngineConfig{
		IsFatal:     allFatal,
		WorstError:  moreImportant,
		ErrorDelay:  coretesting.ShortWait / 2,
		BounceDelay: coretesting.ShortWait / 10,
	}
	engine, err := dependency.NewEngine(config)
	c.Assert(err, jc.ErrorIsNil)

	mh1 := newErrorIgnoringManifoldHarness()
	err = engine.Install("task", mh1.Manifold())
	c.Assert(err, jc.ErrorIsNil)
	mh1.AssertOneStart(c)

	mh2 := newErrorIgnoringManifoldHarness()
	err = engine.Install("another task", mh2.Manifold())
	c.Assert(err, jc.ErrorIsNil)
	mh2.AssertOneStart(c)

	mh1.InjectError(c, errors.New("kerrang"))
	mh2.InjectError(c, importantError)

	err = engine.Wait()
	c.Check(err, gc.ErrorMatches, importantError.Error())
	report := engine.Report()
	c.Check(report["error"], gc.ErrorMatches, importantError.Error())
}

func (s *EngineSuite) TestConfigValidate(c *gc.C) {
	validIsFatal := func(error) bool { return true }
	validWorstError := func(err0, err1 error) error { return err0 }
	validErrorDelay := time.Second
	validBounceDelay := time.Second

	tests := []struct {
		about  string
		config dependency.EngineConfig
		err    string
	}{{
		"IsFatal invalid",
		dependency.EngineConfig{nil, validWorstError, validErrorDelay, validBounceDelay},
		"IsFatal not specified",
	}, {
		"WorstError invalid",
		dependency.EngineConfig{validIsFatal, nil, validErrorDelay, validBounceDelay},
		"WorstError not specified",
	}, {
		"ErrorDelay invalid",
		dependency.EngineConfig{validIsFatal, validWorstError, -time.Second, validBounceDelay},
		"ErrorDelay is negative",
	}, {
		"BounceDelay invalid",
		dependency.EngineConfig{validIsFatal, validWorstError, validErrorDelay, -time.Second},
		"BounceDelay is negative",
	}}

	for i, test := range tests {
		c.Logf("test %d: %v", i, test.about)

		c.Logf("config validation...")
		validateErr := test.config.Validate()
		c.Check(validateErr, gc.ErrorMatches, test.err)

		c.Logf("engine creation...")
		engine, createErr := dependency.NewEngine(test.config)
		c.Check(engine, gc.IsNil)
		c.Check(createErr, gc.ErrorMatches, "invalid config: "+test.err)
	}
}

func (s *EngineSuite) TestValidateEmptyManifolds(c *gc.C) {
	err := dependency.Validate(dependency.Manifolds{})
	c.Check(err, jc.ErrorIsNil)
}

func (s *EngineSuite) TestValidateTrivialCycle(c *gc.C) {
	err := dependency.Validate(dependency.Manifolds{
		"a": dependency.Manifold{Inputs: []string{"a"}},
	})
	c.Check(err.Error(), gc.Equals, `cycle detected at "a" (considering: map[a:true])`)
}

func (s *EngineSuite) TestValidateComplexManifolds(c *gc.C) {

	// Create a bunch of manifolds with tangled but acyclic dependencies; check
	// that they pass validation.
	manifolds := dependency.Manifolds{
		"root1": dependency.Manifold{},
		"root2": dependency.Manifold{},
		"mid1":  dependency.Manifold{Inputs: []string{"root1"}},
		"mid2":  dependency.Manifold{Inputs: []string{"root1", "root2"}},
		"leaf1": dependency.Manifold{Inputs: []string{"root2", "mid1"}},
		"leaf2": dependency.Manifold{Inputs: []string{"root1", "mid2"}},
		"leaf3": dependency.Manifold{Inputs: []string{"root1", "root2", "mid1", "mid2"}},
	}
	err := dependency.Validate(manifolds)
	c.Check(err, jc.ErrorIsNil)

	// Introduce a cycle; check the manifolds no longer validate.
	manifolds["root1"] = dependency.Manifold{Inputs: []string{"leaf1"}}
	err = dependency.Validate(manifolds)
	c.Check(err, gc.ErrorMatches, "cycle detected at .*")
}

func (s *EngineSuite) TestTracedErrMissing(c *gc.C) {

	// Install a worker with an unmet dependency, check it doesn't start
	// (because the implementation returns a Trace()d ErrMissing).
	mh1 := newTracedManifoldHarness("later-task")
	err := s.engine.Install("some-task", mh1.Manifold())
	c.Assert(err, jc.ErrorIsNil)
	mh1.AssertNoStart(c)

	// Install its dependency; check both start.
	mh2 := newTracedManifoldHarness()
	err = s.engine.Install("later-task", mh2.Manifold())
	c.Assert(err, jc.ErrorIsNil)
	mh2.AssertOneStart(c)
	mh1.AssertOneStart(c)
}
