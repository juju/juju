// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package centralhub_test

import (
	"github.com/juju/errors"
	"github.com/juju/pubsub"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2/dependency"
	dt "github.com/juju/worker/v2/dependency/testing"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/centralhub"
)

type ManifoldSuite struct {
	testing.IsolationSuite
	hub    *pubsub.StructuredHub
	config centralhub.ManifoldConfig
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.hub = pubsub.NewStructuredHub(nil)
	s.config = centralhub.ManifoldConfig{
		StateConfigWatcherName: "state-config-watcher",
		Hub:                    s.hub,
	}
}

func (s *ManifoldSuite) manifold() dependency.Manifold {
	return centralhub.Manifold(s.config)
}

func (s *ManifoldSuite) TestInputs(c *gc.C) {
	c.Check(s.manifold().Inputs, jc.DeepEquals, []string{"state-config-watcher"})
}

func (s *ManifoldSuite) TestStateConfigWatcherMissing(c *gc.C) {
	context := dt.StubContext(nil, map[string]interface{}{
		"state-config-watcher": dependency.ErrMissing,
	})

	worker, err := s.manifold().Start(context)
	c.Check(worker, gc.IsNil)
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestStateConfigWatcherNotAStateServer(c *gc.C) {
	context := dt.StubContext(nil, map[string]interface{}{
		"state-config-watcher": false,
	})

	worker, err := s.manifold().Start(context)
	c.Check(worker, gc.IsNil)
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestMissingHub(c *gc.C) {
	s.config.Hub = nil
	context := dt.StubContext(nil, map[string]interface{}{
		"state-config-watcher": true,
	})

	worker, err := s.manifold().Start(context)
	c.Check(worker, gc.IsNil)
	c.Check(errors.Cause(err), jc.Satisfies, errors.IsNotValid)
}

func (s *ManifoldSuite) TestHubOutput(c *gc.C) {
	context := dt.StubContext(nil, map[string]interface{}{
		"state-config-watcher": true,
	})

	manifold := s.manifold()
	worker, err := manifold.Start(context)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(worker, gc.NotNil)
	defer workertest.CleanKill(c, worker)

	var hub *pubsub.StructuredHub
	err = manifold.Output(worker, &hub)
	c.Check(err, jc.ErrorIsNil)
	c.Check(hub, gc.Equals, s.hub)
}
