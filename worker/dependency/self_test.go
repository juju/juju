// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dependency_test

import (
	"fmt"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
)

type SelfSuite struct {
	engineFixture
}

var _ = gc.Suite(&SelfSuite{})

func (s *SelfSuite) TestInputs(c *gc.C) {
	manifold := dependency.SelfManifold(s.engine)
	c.Check(manifold.Inputs, gc.HasLen, 0)
}

func (s *SelfSuite) TestStart(c *gc.C) {
	manifold := dependency.SelfManifold(s.engine)
	engine, err := manifold.Start(nil)
	c.Check(err, jc.ErrorIsNil)
	c.Check(engine, gc.Equals, s.engine)
}

func (s *SelfSuite) TestOutputBadInput(c *gc.C) {
	manifold := dependency.SelfManifold(s.engine)
	var input dependency.Engine
	err := manifold.Output(input, nil)
	c.Check(err, gc.ErrorMatches, "unexpected input worker")
}

func (s *SelfSuite) TestOutputBadOutput(c *gc.C) {
	manifold := dependency.SelfManifold(s.engine)
	var unknown interface{}
	err := manifold.Output(s.engine, &unknown)
	c.Check(err, gc.ErrorMatches, "out should be a \\*Installer or a \\*Reporter; is .*")
	c.Check(unknown, gc.IsNil)
}

func (s *SelfSuite) TestOutputReporter(c *gc.C) {
	manifold := dependency.SelfManifold(s.engine)
	var reporter dependency.Reporter
	err := manifold.Output(s.engine, &reporter)
	c.Check(err, jc.ErrorIsNil)
	c.Check(reporter, gc.Equals, s.engine)
}

func (s *SelfSuite) TestOutputInstaller(c *gc.C) {
	manifold := dependency.SelfManifold(s.engine)
	var installer dependency.Installer
	err := manifold.Output(s.engine, &installer)
	c.Check(err, jc.ErrorIsNil)
	c.Check(installer, gc.Equals, s.engine)
}

func (s *SelfSuite) TestActuallyWorks(c *gc.C) {

	// Create and install a manifold with an unsatisfied dependency.
	mh1 := newManifoldHarness("self")
	err := s.engine.Install("dependent", mh1.Manifold())
	c.Assert(err, jc.ErrorIsNil)
	mh1.AssertNoStart(c)

	// Install an engine inside itself; once it's "started", dependent will
	// be restarted.
	manifold := dependency.SelfManifold(s.engine)
	err = s.engine.Install("self", manifold)
	c.Assert(err, jc.ErrorIsNil)
	mh1.AssertOneStart(c)

	// Check we can still stop it (with a timeout -- injudicious
	// implementation changes could induce deadlocks).
	done := make(chan struct{})
	go func() {
		err := worker.Stop(s.engine)
		c.Check(err, jc.ErrorIsNil)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out")
	}
}

func (s *SelfSuite) TestStress(c *gc.C) {

	// Repeatedly install a manifold inside itself.
	manifold := dependency.SelfManifold(s.engine)
	for i := 0; i < 100; i++ {
		go s.engine.Install(fmt.Sprintf("self-%d", i), manifold)
	}

	// Check we can still stop it (with a timeout -- injudicious
	// implementation changes could induce deadlocks).
	done := make(chan struct{})
	go func() {
		err := worker.Stop(s.engine)
		c.Check(err, jc.ErrorIsNil)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out")
	}
}
