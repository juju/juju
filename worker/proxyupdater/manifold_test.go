// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxyupdater_test

import (
	"github.com/juju/names"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	apiproxyupdater "github.com/juju/juju/api/proxyupdater"
	"github.com/juju/juju/worker"
	proxyup "github.com/juju/juju/worker/proxyupdater"
	workertesting "github.com/juju/juju/worker/testing"
)

type ManifoldSuite struct {
	testing.IsolationSuite
	newCalled, writeSystemFiles bool
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.newCalled = false
	s.PatchValue(&proxyup.NewWorker,
		func(_ *apiproxyupdater.Facade, writeSystemFiles bool) (worker.Worker, error) {
			s.newCalled = true
			s.writeSystemFiles = writeSystemFiles
			return nil, nil
		},
	)
}

func (s *ManifoldSuite) makeConfig(writeFunc func(agent.Config) bool) proxyup.ManifoldConfig {
	return proxyup.ManifoldConfig{
		PostUpgradeManifoldConfig: workertesting.PostUpgradeManifoldTestConfig(),
		ShouldWriteProxyFiles:     writeFunc,
	}
}

func (s *ManifoldSuite) TestMachineShouldWrite(c *gc.C) {
	config := s.makeConfig(func(agent.Config) bool { return true })
	_, err := workertesting.RunPostUpgradeManifold(
		proxyup.Manifold(config),
		&fakeAgent{tag: names.NewMachineTag("42")},
		nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.newCalled, jc.IsTrue)
	c.Assert(s.writeSystemFiles, jc.IsTrue)
}

func (s *ManifoldSuite) TestMachineShouldntWrite(c *gc.C) {
	config := s.makeConfig(func(agent.Config) bool { return false })
	_, err := workertesting.RunPostUpgradeManifold(
		proxyup.Manifold(config),
		&fakeAgent{tag: names.NewMachineTag("42")},
		nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.newCalled, jc.IsTrue)
	c.Assert(s.writeSystemFiles, jc.IsFalse)
}

func (s *ManifoldSuite) TestUnit(c *gc.C) {
	config := s.makeConfig(nil)
	_, err := workertesting.RunPostUpgradeManifold(
		proxyup.Manifold(config),
		&fakeAgent{tag: names.NewUnitTag("foo/0")},
		nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.newCalled, jc.IsTrue)
	c.Assert(s.writeSystemFiles, jc.IsFalse)
}

func (s *ManifoldSuite) TestNonAgent(c *gc.C) {
	config := s.makeConfig(nil)
	_, err := workertesting.RunPostUpgradeManifold(
		proxyup.Manifold(config),
		&fakeAgent{tag: names.NewUserTag("foo")},
		nil)
	c.Assert(err, gc.ErrorMatches, "unknown agent type:.+")
	c.Assert(s.newCalled, jc.IsFalse)
	c.Assert(s.writeSystemFiles, jc.IsFalse)
}

type fakeAgent struct {
	agent.Agent
	tag names.Tag
}

func (a *fakeAgent) CurrentConfig() agent.Config {
	return &fakeConfig{tag: a.tag}
}

type fakeConfig struct {
	agent.Config
	tag names.Tag
}

func (c *fakeConfig) Tag() names.Tag {
	return c.tag
}
