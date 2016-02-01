// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/cmd/jujud/agent/machine"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	dt "github.com/juju/juju/worker/dependency/testing"
)

type APIWorkersSuite struct {
	testing.BaseSuite
	manifold    dependency.Manifold
	startCalled bool
}

var _ = gc.Suite(&APIWorkersSuite{})

func (s *APIWorkersSuite) SetUpTest(c *gc.C) {
	s.startCalled = false
	s.manifold = machine.APIWorkersManifold(machine.APIWorkersConfig{
		APICallerName:     "api-caller",
		UpgradeWaiterName: "upgrade-waiter",
		StartAPIWorkers:   s.startAPIWorkers,
	})
}

func (s *APIWorkersSuite) startAPIWorkers(api.Connection) (worker.Worker, error) {
	s.startCalled = true
	return new(mockWorker), nil
}

func (s *APIWorkersSuite) TestInputs(c *gc.C) {
	c.Assert(s.manifold.Inputs, jc.SameContents, []string{
		"api-caller",
		"upgrade-waiter",
	})
}

func (s *APIWorkersSuite) TestStartNoStartAPIWorkers(c *gc.C) {
	manifold := machine.APIWorkersManifold(machine.APIWorkersConfig{})
	worker, err := manifold.Start(dt.StubGetResource(nil))
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "StartAPIWorkers not specified")
	c.Check(s.startCalled, jc.IsFalse)
}

func (s *APIWorkersSuite) TestStartAPIMissing(c *gc.C) {
	getResource := dt.StubGetResource(dt.StubResources{
		"api-caller":     dt.StubResource{Error: dependency.ErrMissing},
		"upgrade-waiter": dt.StubResource{Output: true},
	})
	worker, err := s.manifold.Start(getResource)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.Equals, dependency.ErrMissing)
	c.Check(s.startCalled, jc.IsFalse)
}

func (s *APIWorkersSuite) TestStartUpgradeWaiterMissing(c *gc.C) {
	getResource := dt.StubGetResource(dt.StubResources{
		"api-caller":     dt.StubResource{Output: new(mockAPIConn)},
		"upgrade-waiter": dt.StubResource{Error: dependency.ErrMissing},
	})
	worker, err := s.manifold.Start(getResource)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.Equals, dependency.ErrMissing)
	c.Check(s.startCalled, jc.IsFalse)
}

func (s *APIWorkersSuite) TestStartUpgradesNotComplete(c *gc.C) {
	getResource := dt.StubGetResource(dt.StubResources{
		"api-caller":     dt.StubResource{Output: new(mockAPIConn)},
		"upgrade-waiter": dt.StubResource{Output: false},
	})
	worker, err := s.manifold.Start(getResource)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.Equals, dependency.ErrMissing)
	c.Check(s.startCalled, jc.IsFalse)
}

func (s *APIWorkersSuite) TestStartSuccess(c *gc.C) {
	getResource := dt.StubGetResource(dt.StubResources{
		"api-caller":     dt.StubResource{Output: new(mockAPIConn)},
		"upgrade-waiter": dt.StubResource{Output: true},
	})
	worker, err := s.manifold.Start(getResource)
	c.Check(worker, gc.Not(gc.IsNil))
	c.Check(err, jc.ErrorIsNil)
	c.Check(s.startCalled, jc.IsTrue)
}

type mockAPIConn struct {
	api.Connection
}

type mockWorker struct {
	worker.Worker
}
