// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controlleragentconfig

import (
	"github.com/juju/clock"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/dependency"
	dependencytesting "github.com/juju/worker/v3/dependency/testing"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"
)

type manifoldSuite struct {
	baseSuite
}

var _ = gc.Suite(&manifoldSuite{})

func (s *manifoldSuite) TestValidateConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.getConfig()
	c.Check(cfg.Validate(), jc.ErrorIsNil)

	cfg = s.getConfig()
	cfg.Logger = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.Clock = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)
}

func (s *manifoldSuite) getConfig() ManifoldConfig {
	return ManifoldConfig{
		Logger: s.logger,
		Clock:  clock.WallClock,
	}
}

func (s *manifoldSuite) getContext() dependency.Context {
	resources := map[string]any{}
	return dependencytesting.StubContext(nil, resources)
}

var expectedInputs = []string{}

func (s *manifoldSuite) TestInputs(c *gc.C) {
	c.Assert(Manifold(s.getConfig()).Inputs, jc.SameContents, expectedInputs)
}

func (s *manifoldSuite) TestStart(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w, err := Manifold(s.getConfig()).Start(s.getContext())
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)
}

func (s *manifoldSuite) TestOutput(c *gc.C) {
	defer s.setupMocks(c).Finish()

	man := Manifold(s.getConfig())
	w, err := man.Start(s.getContext())
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	var watcher ConfigWatcher
	c.Assert(man.Output(w, &watcher), jc.ErrorIsNil)
	c.Assert(watcher, gc.NotNil)
}
