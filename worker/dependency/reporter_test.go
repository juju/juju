// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dependency_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

func (s *EngineSuite) TestReport(c *gc.C) {
	mh1 := newManifoldHarness()
	err := s.engine.Install("task", mh1.Manifold())
	c.Assert(err, jc.ErrorIsNil)
	mh1.AssertOneStart(c)

	report := s.engine.Report()
	c.Assert(report["is-dying"], gc.Equals, false)
	c.Assert(report["manifold-count"], gc.Equals, 1)
}

func (s *EngineSuite) TestReportReachesManifolds(c *gc.C) {
	mh1 := newManifoldHarness()
	manifold := mh1.Manifold()
	manifold.Reporter = func() map[string]interface{} {
		return map[string]interface{}{"here": "hello world"}
	}
	err := s.engine.Install("task", manifold)
	c.Assert(err, jc.ErrorIsNil)

	mh2 := newManifoldHarness()
	err = s.engine.Install("another task", mh2.Manifold())
	c.Assert(err, jc.ErrorIsNil)

	report := s.engine.Report()
	c.Assert(report["is-dying"], gc.Equals, false)
	c.Assert(report["manifold-count"], gc.Equals, 2)
	c.Assert(report["manifolds"], gc.HasLen, 2)
	manifolds := report["manifolds"].(map[string]interface{})
	c.Assert(manifolds["task"], gc.DeepEquals, map[string]interface{}{"here": "hello world"})
	c.Assert(manifolds["another task"], gc.DeepEquals, map[string]interface{}(nil))
}
