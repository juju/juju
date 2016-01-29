// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradewaiter_test

import (
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	dt "github.com/juju/juju/worker/dependency/testing"
	"github.com/juju/juju/worker/gate"
	"github.com/juju/juju/worker/upgradewaiter"
)

type ManifoldSuite struct {
	testing.IsolationSuite
	manifold dependency.Manifold
	worker   worker.Worker
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.manifold = upgradewaiter.Manifold(upgradewaiter.ManifoldConfig{
		UpgradeStepsWaiterName: "steps-waiter",
		UpgradeCheckWaiterName: "check-waiter",
	})
}

func (s *ManifoldSuite) TestInputs(c *gc.C) {
	c.Assert(s.manifold.Inputs, jc.SameContents, []string{"steps-waiter", "check-waiter"})
}

func (s *ManifoldSuite) TestStartNoStepsWaiter(c *gc.C) {
	getResource := dt.StubGetResource(dt.StubResources{
		"steps-waiter": dt.StubResource{Error: dependency.ErrMissing},
		"check-waiter": dt.StubResource{Output: gate.NewLock()},
	})
	w, err := s.manifold.Start(getResource)
	c.Assert(w, gc.IsNil)
	c.Assert(err, gc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestStartNoCheckWaiter(c *gc.C) {
	getResource := dt.StubGetResource(dt.StubResources{
		"steps-waiter": dt.StubResource{Output: gate.NewLock()},
		"check-waiter": dt.StubResource{Error: dependency.ErrMissing},
	})
	w, err := s.manifold.Start(getResource)
	c.Assert(w, gc.IsNil)
	c.Assert(err, gc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestStartSuccess(c *gc.C) {
	getResource := dt.StubGetResource(dt.StubResources{
		"steps-waiter": dt.StubResource{Output: gate.NewLock()},
		"check-waiter": dt.StubResource{Output: gate.NewLock()},
	})
	w, err := s.manifold.Start(getResource)
	c.Assert(err, jc.ErrorIsNil)
	checkStop(c, w)
}

func (s *ManifoldSuite) TestOutput(c *gc.C) {
	stepsLock := gate.NewLock()
	checkLock := gate.NewLock()
	getResource := dt.StubGetResource(dt.StubResources{
		"steps-waiter": dt.StubResource{Output: stepsLock},
		"check-waiter": dt.StubResource{Output: checkLock},
	})
	w, err := s.manifold.Start(getResource)
	c.Assert(err, jc.ErrorIsNil)

	// Upgrades not completed yet so output is false.
	s.assertOutputFalse(c, w)

	// Unlock one of the upgrade gates, output should still be false.
	stepsLock.Unlock()
	s.assertOutputFalse(c, w)

	// Unlock the other gate, output should now be true.
	checkLock.Unlock()
	s.assertOutputTrue(c, w)

	// .. and the worker should exit with ErrBounce.
	checkStopWithError(c, w, dependency.ErrBounce)

	// Restarting the worker should result in the output immediately
	// being true.
	w2, err := s.manifold.Start(getResource)
	c.Assert(err, jc.ErrorIsNil)
	s.assertOutputTrue(c, w)
	checkStop(c, w2)
}

func (s *ManifoldSuite) TestOutputWithWrongWorker(c *gc.C) {
	getResource := dt.StubGetResource(dt.StubResources{
		"steps-waiter": dt.StubResource{Output: gate.NewLock()},
		"check-waiter": dt.StubResource{Output: gate.NewLock()},
	})
	_, err := s.manifold.Start(getResource)
	c.Assert(err, jc.ErrorIsNil)

	type dummyWorker struct {
		worker.Worker
	}
	var foo bool
	err = s.manifold.Output(new(dummyWorker), &foo)
	c.Assert(err, gc.ErrorMatches, `in should be a \*upgradeWaiter;.+`)
}

func (s *ManifoldSuite) TestOutputWithWrongType(c *gc.C) {
	getResource := dt.StubGetResource(dt.StubResources{
		"steps-waiter": dt.StubResource{Output: gate.NewLock()},
		"check-waiter": dt.StubResource{Output: gate.NewLock()},
	})
	w, err := s.manifold.Start(getResource)
	c.Assert(err, jc.ErrorIsNil)

	var foo int
	err = s.manifold.Output(w, &foo)
	c.Assert(err, gc.ErrorMatches, `out should be a \*bool;.+`)
}

func (s *ManifoldSuite) assertOutputFalse(c *gc.C, w worker.Worker) {
	time.Sleep(coretesting.ShortWait)
	var done bool
	s.manifold.Output(w, &done)
	c.Assert(done, jc.IsFalse)
}

func (s *ManifoldSuite) assertOutputTrue(c *gc.C, w worker.Worker) {
	for attempt := coretesting.LongAttempt.Start(); attempt.Next(); {
		var done bool
		s.manifold.Output(w, &done)
		if done == true {
			return
		}
	}
	c.Fatalf("timed out waiting for output to become true")
}

func checkStop(c *gc.C, w worker.Worker) {
	err := worker.Stop(w)
	c.Check(err, jc.ErrorIsNil)
}

func checkStopWithError(c *gc.C, w worker.Worker, expectedErr error) {
	err := worker.Stop(w)
	c.Check(err, gc.Equals, expectedErr)
}
