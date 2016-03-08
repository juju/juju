// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/worker/dependency"
	dt "github.com/juju/juju/worker/dependency/testing"
	"github.com/juju/juju/worker/provisioner"
)

type ManifoldSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) TestManifold(c *gc.C) {
	manifold := provisioner.Manifold(provisioner.ManifoldConfig{
		AgentName:     "jeff",
		APICallerName: "barry",
	})

	c.Check(manifold.Inputs, jc.DeepEquals, []string{"jeff", "barry"})
	c.Check(manifold.Output, gc.IsNil)
	c.Check(manifold.Start, gc.NotNil)
	// manifold.Start is tested extensively via direct use in provisioner_test
}

func (s *ManifoldSuite) TestMissingAgent(c *gc.C) {
	manifold := provisioner.Manifold(provisioner.ManifoldConfig{
		AgentName:     "agent",
		APICallerName: "api-caller",
	})
	w, err := manifold.Start(dt.StubGetResource(dt.StubResources{
		"agent":      dt.StubResource{Error: dependency.ErrMissing},
		"api-caller": dt.StubResource{Output: struct{ base.APICaller }{}},
	}))
	c.Check(w, gc.IsNil)
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestMissingAPICaller(c *gc.C) {
	manifold := provisioner.Manifold(provisioner.ManifoldConfig{
		AgentName:     "agent",
		APICallerName: "api-caller",
	})
	w, err := manifold.Start(dt.StubGetResource(dt.StubResources{
		"agent":      dt.StubResource{Output: struct{ agent.Agent }{}},
		"api-caller": dt.StubResource{Error: dependency.ErrMissing},
	}))
	c.Check(w, gc.IsNil)
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
}
