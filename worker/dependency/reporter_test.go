// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dependency_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

func (s *EngineSuite) TestReport(c *gc.C) {
	report := s.engine.Report()
	c.Assert(report, jc.DeepEquals, map[string]interface{}{
		"is-dying":       false,
		"manifold-count": 0,
		"workers":        map[string]interface{}{},
	})
}

func (s *EngineSuite) TestShuttingDownEngineReport(c *gc.C) {
	s.engine.Kill()
	s.engine.Wait()
	report := s.engine.Report()
	c.Assert(report, jc.DeepEquals, map[string]interface{}{
		"error": "engine is shutting down",
	})
}

func (s *EngineSuite) TestReportReachesManifolds(c *gc.C) {
	mh1 := newManifoldHarness()
	manifold := mh1.Manifold()
	err := s.engine.Install("task", manifold)
	c.Assert(err, jc.ErrorIsNil)

	mh2 := newManifoldHarness()
	err = s.engine.Install("another task", mh2.Manifold())
	c.Assert(err, jc.ErrorIsNil)

	mh1.AssertStart(c)
	mh2.AssertStart(c)

	report := s.engine.Report()
	expectedReport := map[string]interface{}{
		"is-dying":       false,
		"manifold-count": 2,
		"workers": map[string]interface{}{
			"task": map[string]interface{}{
				"starting": false,
				"stopping": false,
				"report": map[string]interface{}{
					"key1": "hello there",
				},
			},
			"another task": map[string]interface{}{
				"starting": false,
				"stopping": false,
				"report": map[string]interface{}{
					"key1": "hello there",
				},
			},
		},
	}
	c.Assert(report, jc.DeepEquals, expectedReport)
}
