// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradeseries_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
	dt "github.com/juju/worker/v2/dependency/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/worker/upgradeseries"
)

type ManifoldSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) TestInputs(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	cfg, _, _ := validManifoldConfig(ctrl)
	cfg.NewFacade = nil

	c.Check(upgradeseries.Manifold(cfg).Inputs, jc.DeepEquals, []string{"agent-name", "api-caller-name"})
}

func (*ManifoldSuite) TestStartMissingNewFacade(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	cfg, _, _ := validManifoldConfig(ctrl)
	cfg.NewFacade = nil

	work, err := upgradeseries.Manifold(cfg).Start(newStubContext())
	c.Check(work, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "nil NewFacade function not valid")
}

func (*ManifoldSuite) TestStartMissingNewWorker(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	cfg, _, _ := validManifoldConfig(ctrl)
	cfg.NewWorker = nil

	work, err := upgradeseries.Manifold(cfg).Start(newStubContext())
	c.Check(work, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "nil NewWorker function not valid")
}

func (*ManifoldSuite) TestStartMissingLogger(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	cfg, _, _ := validManifoldConfig(ctrl)
	cfg.Logger = nil

	work, err := upgradeseries.Manifold(cfg).Start(newStubContext())
	c.Check(work, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "nil Logger not valid")
}

func (s *ManifoldSuite) TestStartMissingAgentName(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	cfg, _, _ := validManifoldConfig(ctrl)
	ctx := dt.StubContext(nil, map[string]interface{}{
		"agent-name":      dependency.ErrMissing,
		"api-caller-name": &dummyAPICaller{},
	})

	work, err := upgradeseries.Manifold(cfg).Start(ctx)
	c.Check(work, gc.IsNil)
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestStartMissingAPICallerName(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	cfg, _, _ := validManifoldConfig(ctrl)
	ctx := dt.StubContext(nil, map[string]interface{}{
		"agent-name":      &dummyAgent{},
		"api-caller-name": dependency.ErrMissing,
	})

	work, err := upgradeseries.Manifold(cfg).Start(ctx)
	c.Check(work, gc.IsNil)
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestStartSuccess(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	cfg, _, _ := validManifoldConfig(ctrl)

	work, err := upgradeseries.Manifold(cfg).Start(newStubContext())
	c.Check(work, gc.NotNil)
	c.Check(err, jc.ErrorIsNil)
}

func (s *ManifoldSuite) TestStartError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	cfg, _, _ := validManifoldConfig(ctrl)
	cfg.NewWorker = func(_ upgradeseries.Config) (worker.Worker, error) { return nil, errors.New("WHACK!") }

	work, err := upgradeseries.Manifold(cfg).Start(newStubContext())
	c.Check(work, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "starting machine upgrade series worker: WHACK!")
}

type dummyAPICaller struct {
	base.APICaller
}

type dummyConfig struct {
	agent.Config
}

type dummyAgent struct {
	agent.Agent
}

func (*dummyAgent) CurrentConfig() agent.Config {
	return &dummyConfig{}
}

func (*dummyConfig) Tag() names.Tag {
	return names.NewMachineTag("666")
}

func newStubContext() *dt.Context {
	return dt.StubContext(nil, map[string]interface{}{
		"agent-name":      &dummyAgent{},
		"api-caller-name": &dummyAPICaller{},
	})
}
