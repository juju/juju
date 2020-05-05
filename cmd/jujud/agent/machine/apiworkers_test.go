// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
	dt "github.com/juju/worker/v2/dependency/testing"
	gc "gopkg.in/check.v1"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/cmd/jujud/agent/machine"
	"github.com/juju/juju/testing"
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
		APICallerName:   "api-caller",
		StartAPIWorkers: s.startAPIWorkers,
	})
}

func (s *APIWorkersSuite) startAPIWorkers(api.Connection) (worker.Worker, error) {
	s.startCalled = true
	return new(mockWorker), nil
}

func (s *APIWorkersSuite) TestInputs(c *gc.C) {
	c.Assert(s.manifold.Inputs, jc.SameContents, []string{
		"api-caller",
	})
}

func (s *APIWorkersSuite) TestStartNoStartAPIWorkers(c *gc.C) {
	manifold := machine.APIWorkersManifold(machine.APIWorkersConfig{})
	worker, err := manifold.Start(dt.StubContext(nil, nil))
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "StartAPIWorkers not specified")
	c.Check(s.startCalled, jc.IsFalse)
}

func (s *APIWorkersSuite) TestStartAPIMissing(c *gc.C) {
	context := dt.StubContext(nil, map[string]interface{}{
		"api-caller": dependency.ErrMissing,
	})
	worker, err := s.manifold.Start(context)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.Equals, dependency.ErrMissing)
	c.Check(s.startCalled, jc.IsFalse)
}

func (s *APIWorkersSuite) TestStartSuccess(c *gc.C) {
	context := dt.StubContext(nil, map[string]interface{}{
		"api-caller": new(mockAPIConn),
	})
	worker, err := s.manifold.Start(context)
	c.Check(worker, gc.Not(gc.IsNil))
	c.Check(err, jc.ErrorIsNil)
	c.Check(s.startCalled, jc.IsTrue)
}

type mockAPIConn struct {
	api.Connection
}

type mockWorker struct {
	tomb tomb.Tomb
}

func (w *mockWorker) Kill() {
	w.tomb.Kill(nil)
}

func (w *mockWorker) Wait() error {
	return w.tomb.Wait()
}
