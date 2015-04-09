// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unit_test

import (
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coreagent "github.com/juju/juju/agent"
	"github.com/juju/juju/cmd/jujud/unit"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker/agent"
	"github.com/juju/juju/worker/dependency"
	dt "github.com/juju/juju/worker/dependency/testing"
)

type BinaryUpgraderManifoldSuite struct {
	testing.JujuConnSuite
	manifold dependency.Manifold
}

var _ = gc.Suite(&BinaryUpgraderManifoldSuite{})

func (s *BinaryUpgraderManifoldSuite) SetUpSuite(c *gc.C) {
	s.JujuConnSuite.SetUpSuite(c)
	s.manifold = unit.BinaryUpgraderManifold(
		unit.BinaryUpgraderManifoldConfig{
			AgentName:     "agent-name",
			ApiCallerName: "api-caller-name",
		},
	)
}

func (s *BinaryUpgraderManifoldSuite) TestInputs(c *gc.C) {
	c.Check(s.manifold.Inputs, jc.DeepEquals, []string{"agent-name", "api-caller-name"})
}

func (s *BinaryUpgraderManifoldSuite) TestStartAgentMissing(c *gc.C) {
	getResource := dt.StubGetResource(dt.StubResources{
		"agent-name": dt.StubResource{Error: dependency.ErrMissing},
	})

	worker, err := s.manifold.Start(getResource)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.Equals, dependency.ErrMissing)
}

func (s *BinaryUpgraderManifoldSuite) TestStartApiConnMissing(c *gc.C) {
	getResource := dt.StubGetResource(dt.StubResources{
		"agent-name":      dt.StubResource{Output: &mockAgent{}},
		"api-caller-name": dt.StubResource{Error: dependency.ErrMissing},
	})

	worker, err := s.manifold.Start(getResource)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.Equals, dependency.ErrMissing)
}

func (s *BinaryUpgraderManifoldSuite) TestStartSetVersionFailure(c *gc.C) {
	agent := &mockAgent{config: &mockAgentConfig{tag: names.NewUnitTag("foo/2")}}
	c.Fatalf("XXX %s", agent)
}

func (s *BinaryUpgraderManifoldSuite) TestStartSuccess(c *gc.C) {
	c.Fatalf("XXX")
}

type mockAgent struct {
	agent.Agent
	config *mockAgentConfig
}

func (mock *mockAgent) CurrentConfig() coreagent.Config {
	return mock.config
}

type mockAgentConfig struct {
	coreagent.Config
	tag     names.Tag
	version version.Number
}

func (mock *mockAgentConfig) Tag() names.Tag {
	return mock.tag
}

func (mock *mockAgentConfig) UpgradedToVersion() version.Number {
	return mock.version
}
