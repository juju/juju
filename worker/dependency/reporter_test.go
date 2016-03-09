// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dependency_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/dependency"
)

type ReportSuite struct {
	engineFixture
}

var _ = gc.Suite(&ReportSuite{})

func (s *ReportSuite) TestReportStarted(c *gc.C) {
	report := s.engine.Report()
	c.Check(report, jc.DeepEquals, map[string]interface{}{
		"state":     "started",
		"error":     nil,
		"manifolds": map[string]interface{}{},
	})
}

func (s *ReportSuite) TestReportStopped(c *gc.C) {
	s.engine.Kill()
	err := s.engine.Wait()
	c.Check(err, jc.ErrorIsNil)
	report := s.engine.Report()
	c.Check(report, jc.DeepEquals, map[string]interface{}{
		"state":     "stopped",
		"error":     nil,
		"manifolds": map[string]interface{}{},
	})
}

func (s *ReportSuite) TestReportStopping(c *gc.C) {
	mh1 := newErrorIgnoringManifoldHarness()
	err := s.engine.Install("task", mh1.Manifold())
	c.Assert(err, jc.ErrorIsNil)
	defer func() {
		s.engine.Kill()
		mh1.InjectError(c, nil)
		err := s.engine.Wait()
		c.Check(err, jc.ErrorIsNil)
	}()
	mh1.AssertOneStart(c)

	// It may take a short time for the main loop to notice
	// the change and stop the "task" worker.
	s.engine.Kill()
	var isTaskStopping = func(report map[string]interface{}) bool {
		manifolds := report["manifolds"].(map[string]interface{})
		task := manifolds["task"].(map[string]interface{})
		switch taskState := task["state"]; taskState {
		case "started":
			return false
		case "stopping":
			return true
		default:
			c.Fatalf("unexpected task state: %v", taskState)
		}
		panic("unreachable")
	}

	var report map[string]interface{}
	for i := 0; i < 3; i++ {
		report = s.engine.Report()
		if isTaskStopping(report) {
			break
		}
		time.Sleep(coretesting.ShortWait)
	}
	c.Check(report, jc.DeepEquals, map[string]interface{}{
		"state": "stopping",
		"error": nil,
		"manifolds": map[string]interface{}{
			"task": map[string]interface{}{
				"state":        "stopping",
				"error":        nil,
				"inputs":       ([]string)(nil),
				"resource-log": []map[string]interface{}{},
				"report": map[string]interface{}{
					"key1": "hello there",
				},
			},
		},
	})
}

func (s *ReportSuite) TestReportInputs(c *gc.C) {
	mh1 := newManifoldHarness()
	err := s.engine.Install("task", mh1.Manifold())
	c.Assert(err, jc.ErrorIsNil)
	mh1.AssertOneStart(c)

	mh2 := newManifoldHarness("task")
	err = s.engine.Install("another task", mh2.Manifold())
	c.Assert(err, jc.ErrorIsNil)
	mh2.AssertOneStart(c)

	report := s.engine.Report()
	c.Check(report, jc.DeepEquals, map[string]interface{}{
		"state": "started",
		"error": nil,
		"manifolds": map[string]interface{}{
			"task": map[string]interface{}{
				"state":        "started",
				"error":        nil,
				"inputs":       ([]string)(nil),
				"resource-log": []map[string]interface{}{},
				"report": map[string]interface{}{
					"key1": "hello there",
				},
			},
			"another task": map[string]interface{}{
				"state":  "started",
				"error":  nil,
				"inputs": []string{"task"},
				"resource-log": []map[string]interface{}{{
					"name":  "task",
					"type":  "<nil>",
					"error": nil,
				}},
				"report": map[string]interface{}{
					"key1": "hello there",
				},
			},
		},
	})
}
func (s *ReportSuite) TestReportError(c *gc.C) {
	mh1 := newManifoldHarness("missing")
	manifold := mh1.Manifold()
	err := s.engine.Install("task", manifold)
	c.Assert(err, jc.ErrorIsNil)
	mh1.AssertNoStart(c)

	s.engine.Kill()
	err = s.engine.Wait()
	c.Check(err, jc.ErrorIsNil)

	report := s.engine.Report()
	c.Check(report, jc.DeepEquals, map[string]interface{}{
		"state": "stopped",
		"error": nil,
		"manifolds": map[string]interface{}{
			"task": map[string]interface{}{
				"state":  "stopped",
				"error":  dependency.ErrMissing,
				"inputs": []string{"missing"},
				"resource-log": []map[string]interface{}{{
					"name":  "missing",
					"type":  "<nil>",
					"error": dependency.ErrMissing,
				}},
				"report": (map[string]interface{})(nil),
			},
		},
	})
}
