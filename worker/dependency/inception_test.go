// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dependency_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
)

type InceptionSuite struct {
	engineFixture
}

var _ = gc.Suite(&InceptionSuite{})

func (s *InceptionSuite) TestInputs(c *gc.C) {
	manifold := dependency.InceptionManifold(s.engine)
	c.Check(manifold.Inputs, gc.HasLen, 0)
}

func (s *InceptionSuite) TestStart(c *gc.C) {
	manifold := dependency.InceptionManifold(s.engine)
	engine, err := manifold.Start(nil)
	c.Check(err, jc.ErrorIsNil)
	c.Check(engine, gc.Equals, s.engine)
}

func (s *InceptionSuite) TestOutputBadInput(c *gc.C) {
	manifold := dependency.InceptionManifold(s.engine)
	var input dependency.Engine
	err := manifold.Output(input, nil)
	c.Check(err, gc.ErrorMatches, "unexpected input worker")
}

func (s *InceptionSuite) TestOutputBadOutput(c *gc.C) {
	manifold := dependency.InceptionManifold(s.engine)
	var unknown interface{}
	err := manifold.Output(s.engine, &unknown)
	c.Check(err, gc.ErrorMatches, "out should be a \\*Installer or a \\*Reporter; is .*")
	c.Check(unknown, gc.IsNil)
}

func (s *InceptionSuite) TestOutputReporter(c *gc.C) {
	manifold := dependency.InceptionManifold(s.engine)
	var reporter dependency.Reporter
	err := manifold.Output(s.engine, &reporter)
	c.Check(err, jc.ErrorIsNil)
	c.Check(reporter, gc.Equals, s.engine)
}

func (s *InceptionSuite) TestOutputInstaller(c *gc.C) {
	manifold := dependency.InceptionManifold(s.engine)
	var installer dependency.Installer
	err := manifold.Output(s.engine, &installer)
	c.Check(err, jc.ErrorIsNil)
	c.Check(installer, gc.Equals, s.engine)
}

func (s *InceptionSuite) TestActuallyWorks(c *gc.C) {

	// Install an engine inside itself.
	manifold := dependency.InceptionManifold(s.engine)
	name := "bwaaawwwwwmmmmmmmmmmm"
	err := s.engine.Install(name, manifold)
	c.Assert(err, jc.ErrorIsNil)

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
