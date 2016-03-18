// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dependency_test

import (
	"fmt"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/workertest"
)

type SelfSuite struct {
	testing.IsolationSuite
	fix *engineFixture
}

var _ = gc.Suite(&SelfSuite{})

func (s *SelfSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.fix = &engineFixture{}
}

func (s *SelfSuite) TestInputs(c *gc.C) {
	s.fix.run(c, func(engine dependency.Engine) {
		manifold := dependency.SelfManifold(engine)
		c.Check(manifold.Inputs, gc.HasLen, 0)
	})
}

func (s *SelfSuite) TestStart(c *gc.C) {
	s.fix.run(c, func(engine dependency.Engine) {
		manifold := dependency.SelfManifold(engine)
		actual, err := manifold.Start(nil)
		c.Check(err, jc.ErrorIsNil)
		c.Check(actual, gc.Equals, engine)
	})
}

func (s *SelfSuite) TestOutputBadInput(c *gc.C) {
	s.fix.run(c, func(engine dependency.Engine) {
		manifold := dependency.SelfManifold(engine)
		var input dependency.Engine
		err := manifold.Output(input, nil)
		c.Check(err, gc.ErrorMatches, "unexpected input worker")
	})
}

func (s *SelfSuite) TestOutputBadOutput(c *gc.C) {
	s.fix.run(c, func(engine dependency.Engine) {
		manifold := dependency.SelfManifold(engine)
		var unknown interface{}
		err := manifold.Output(engine, &unknown)
		c.Check(err, gc.ErrorMatches, "out should be a \\*Installer or a \\*Reporter; is .*")
		c.Check(unknown, gc.IsNil)
	})
}

func (s *SelfSuite) TestOutputReporter(c *gc.C) {
	s.fix.run(c, func(engine dependency.Engine) {
		manifold := dependency.SelfManifold(engine)
		var reporter dependency.Reporter
		err := manifold.Output(engine, &reporter)
		c.Check(err, jc.ErrorIsNil)
		c.Check(reporter, gc.Equals, engine)
	})
}

func (s *SelfSuite) TestOutputInstaller(c *gc.C) {
	s.fix.run(c, func(engine dependency.Engine) {
		manifold := dependency.SelfManifold(engine)
		var installer dependency.Installer
		err := manifold.Output(engine, &installer)
		c.Check(err, jc.ErrorIsNil)
		c.Check(installer, gc.Equals, engine)
	})
}

func (s *SelfSuite) TestActuallyWorks(c *gc.C) {
	s.fix.run(c, func(engine dependency.Engine) {

		// Create and install a manifold with an unsatisfied dependency.
		mh1 := newManifoldHarness("self")
		err := engine.Install("dependent", mh1.Manifold())
		c.Assert(err, jc.ErrorIsNil)
		mh1.AssertNoStart(c)

		// Install an engine inside itself; once it's "started", dependent will
		// be restarted.
		manifold := dependency.SelfManifold(engine)
		err = engine.Install("self", manifold)
		c.Assert(err, jc.ErrorIsNil)
		mh1.AssertOneStart(c)

		// Give it a moment to screw up if it's going to
		// (injudicious implementation could induce deadlock)
		// then let the fixture worry about a clean kill.
		workertest.CheckAlive(c, engine)
	})
}

func (s *SelfSuite) TestStress(c *gc.C) {
	s.fix.run(c, func(engine dependency.Engine) {

		// Repeatedly install a manifold inside itself.
		manifold := dependency.SelfManifold(engine)
		for i := 0; i < 100; i++ {
			go engine.Install(fmt.Sprintf("self-%d", i), manifold)
		}

		// Give it a moment to screw up if it's going to
		// (injudicious implementation could induce deadlock)
		// then let the fixture worry about a clean kill.
		workertest.CheckAlive(c, engine)
	})
}
