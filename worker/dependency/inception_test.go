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

func (s *InceptionSuite) TestInceptionServices(c *gc.C) {

	// Install an engine inside itself.
	manifold := dependency.InceptionManifold(s.engine)
	name := "bwaaawwwwwmmmmmmmmmmm"
	err := s.engine.Install(name, manifold)
	c.Assert(err, jc.ErrorIsNil)

	// Start a dependent task that'll run all our tests.
	done := make(chan struct{})
	err = s.engine.Install("test-task", dependency.Manifold{
		Inputs: []string{name},
		Start: func(getResource dependency.GetResourceFunc) (worker.Worker, error) {

			// Retry until the inception manifold worker is registered.
			err := getResource(name, nil)
			if err == dependency.ErrMissing {
				return nil, err
			}

			// Run the tests. Mustn't assert on this goroutine.
			defer close(done)
			c.Check(err, jc.ErrorIsNil)

			var reporter dependency.Reporter
			err = getResource(name, &reporter)
			c.Check(err, jc.ErrorIsNil)
			c.Check(reporter, gc.Equals, s.engine)

			var installer dependency.Installer
			err = getResource(name, &installer)
			c.Check(err, jc.ErrorIsNil)
			c.Check(installer, gc.Equals, s.engine)

			var unknown interface{}
			err = getResource(name, &unknown)
			c.Check(err, gc.ErrorMatches, "out should be a \\*Installer or a \\*Reporter; is .*")
			c.Check(unknown, gc.IsNil)

			// Return a real worker so we don't keep restarting and potentially double-closing.
			return startMinimalWorker(getResource)
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out")
	}
}
