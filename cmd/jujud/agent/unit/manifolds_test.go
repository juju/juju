// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unit_test

import (
	"github.com/juju/errors"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/cmd/jujud/agent/unit"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
)

type ManifoldsSuite struct {
	testing.BaseSuite

	stub *gitjujutesting.Stub
}

var _ = gc.Suite(&ManifoldsSuite{})

func (s *ManifoldsSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.stub = &gitjujutesting.Stub{}
}

func (s *ManifoldsSuite) TearDownTest(c *gc.C) {
	for name := range unit.RegisteredManifolds {
		delete(unit.RegisteredManifolds, name)
	}

	s.BaseSuite.TearDownTest(c)
}

func (s *ManifoldsSuite) getResourceFunc(apiCaller base.APICaller) dependency.GetResourceFunc {
	return func(name string, out interface{}) error {
		s.stub.AddCall("getResource", name, out)
		if err := s.stub.NextErr(); err != nil {
			return errors.Trace(err)
		}

		unpacked := out.(*base.APICaller)
		*unpacked = apiCaller
		return nil
	}
}

func (s *ManifoldsSuite) newManifold(config unit.ManifoldsConfig) (dependency.Manifold, error) {
	var manifold dependency.Manifold

	s.stub.AddCall("newManifold", config)
	if err := s.stub.NextErr(); err != nil {
		return manifold, errors.Trace(err)
	}

	manifold.Start = func(getResource dependency.GetResourceFunc) (worker.Worker, error) {
		s.stub.AddCall("Start", getResource)
		if err := s.stub.NextErr(); err != nil {
			return nil, errors.Trace(err)
		}

		loop := func(stopCh <-chan struct{}) error {
			s.stub.AddCall("loop", stopCh)
			if err := s.stub.NextErr(); err != nil {
				return errors.Trace(err)
			}

			return nil
		}
		return worker.NewSimpleWorker(loop), nil
	}
	return manifold, nil
}

func (s *ManifoldsSuite) TestRegisterManifold(c *gc.C) {
	err := unit.RegisterManifold("spam", s.newManifold)
	c.Assert(err, jc.ErrorIsNil)

	// We can't compare functions so we jump through hoops instead.
	c.Check(unit.RegisteredManifolds, gc.HasLen, 1)
	var config unit.ManifoldsConfig
	registered := unit.RegisteredManifolds["spam"]
	manifold, err := registered(config)
	c.Assert(err, jc.ErrorIsNil)
	manifold.Start(nil)
	s.stub.CheckCallNames(c, "newManifold", "Start")
}

func (s *ManifoldsSuite) TestStartFuncs(c *gc.C) {
	for _, name := range []string{"spam", "eggs"} {
		err := unit.RegisterManifold(name, s.newManifold)
		c.Assert(err, jc.ErrorIsNil)
	}

	config := unit.ManifoldsConfig{
		Agent: fakeAgent{},
	}
	manifolds, err := unit.Manifolds(config)
	c.Assert(err, jc.ErrorIsNil)

	var names []string
	for name, manifold := range manifolds {
		c.Logf("checking %q manifold", name)
		c.Check(manifold.Start, gc.NotNil)
		names = append(names, name)
	}
	c.Check(names, jc.SameContents, []string{
		unit.AgentName,
		unit.APIAdddressUpdaterName,
		unit.APICallerName,
		unit.LeadershipTrackerName,
		unit.LoggingConfigUpdaterName,
		unit.LogSenderName,
		unit.MachineLockName,
		unit.ProxyConfigUpdaterName,
		unit.RsyslogConfigUpdaterName,
		unit.UniterName,
		unit.UpgraderName,
		"spam",
		"eggs",
	})
	s.stub.CheckCallNames(c, "newManifold", "newManifold")
	s.stub.ResetCalls()
	manifolds["spam"].Start(s.getResourceFunc(nil))
	s.stub.CheckCallNames(c, "Start")
}

// TODO(cmars) 2015/08/10: rework this into builtin Engine cycle checker.
func (s *ManifoldsSuite) TestAcyclic(c *gc.C) {
	manifolds, err := unit.Manifolds(unit.ManifoldsConfig{
		Agent: fakeAgent{},
	})
	c.Assert(err, jc.ErrorIsNil)
	count := len(manifolds)

	// Set of vars for depth-first topological sort of manifolds. (Note that,
	// because we've already got outgoing links stored conveniently, we're
	// actually checking the transpose of the dependency graph. Cycles will
	// still be cycles in either direction, though.)
	done := make(map[string]bool)
	doing := make(map[string]bool)
	sorted := make([]string, 0, count)

	// Stupid _-suffix malarkey allows recursion. Seems cleaner to keep these
	// considerations inside this func than to embody the algorithm in a type.
	visit := func(node string) {}
	visit_ := func(node string) {
		if doing[node] {
			c.Fatalf("cycle detected at %q (considering: %v)", node, doing)
		}
		if !done[node] {
			doing[node] = true
			for _, input := range manifolds[node].Inputs {
				visit(input)
			}
			done[node] = true
			doing[node] = false
			sorted = append(sorted, node)
		}
	}
	visit = visit_

	// Actually sort them, or fail if we find a cycle.
	for node := range manifolds {
		visit(node)
	}
	c.Logf("got: %v", sorted)
	c.Check(sorted, gc.HasLen, count) // Final sanity check.
}

type fakeAgent struct {
	agent.Agent
}
