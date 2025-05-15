// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controlleragentconfig

import (
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/dependency"
	dependencytesting "github.com/juju/worker/v4/dependency/testing"
	"github.com/juju/worker/v4/workertest"

	"github.com/juju/juju/agent"
)

type manifoldSuite struct {
	baseSuite

	agent *mockAgent
}

var _ = tc.Suite(&manifoldSuite{})

func (s *manifoldSuite) SetUpTest(c *tc.C) {
	s.baseSuite.SetUpTest(c)

	s.agent = new(mockAgent)
	s.agent.conf.tag = names.NewMachineTag("99")
}

func (s *manifoldSuite) TestValidateConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.getConfig()
	c.Check(cfg.Validate(), tc.ErrorIsNil)

	cfg = s.getConfig()
	cfg.AgentName = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.Logger = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.Clock = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.NewSocketListener = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.SocketName = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)
}

func (s *manifoldSuite) getConfig() ManifoldConfig {
	return ManifoldConfig{
		AgentName:         "agent",
		Logger:            s.logger,
		Clock:             clock.WallClock,
		NewSocketListener: NewSocketListener,
		SocketName:        "test.socket",
	}
}

func (s *manifoldSuite) newContext() dependency.Getter {
	resources := map[string]any{
		"agent": s.agent,
	}
	return dependencytesting.StubGetter(resources)
}

func (s *manifoldSuite) TestInputs(c *tc.C) {
	c.Assert(Manifold(s.getConfig()).Inputs, tc.SameContents, []string{"agent"})
}

func (s *manifoldSuite) TestStart(c *tc.C) {
	defer s.setupMocks(c).Finish()

	w, err := Manifold(s.getConfig()).Start(c.Context(), s.newContext())
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)
}

func (s *manifoldSuite) TestOutput(c *tc.C) {
	defer s.setupMocks(c).Finish()

	man := Manifold(s.getConfig())
	w, err := man.Start(c.Context(), s.newContext())
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	var watcher ConfigWatcher
	c.Assert(man.Output(w, &watcher), tc.ErrorIsNil)
	c.Assert(watcher, tc.NotNil)
}

type mockAgent struct {
	agent.Agent
	conf mockConfig
}

func (ma *mockAgent) CurrentConfig() agent.Config {
	return &ma.conf
}

type mockConfig struct {
	agent.ConfigSetter
	tag names.Tag
}

func (mc *mockConfig) Tag() names.Tag {
	if mc.tag == nil {
		return names.NewMachineTag("99")
	}
	return mc.tag
}
