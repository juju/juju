// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spacenamer_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v3"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/dependency"
	dt "gopkg.in/juju/worker.v1/dependency/testing"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	workermocks "github.com/juju/juju/worker/mocks"
	"github.com/juju/juju/worker/spacenamer"
	"github.com/juju/juju/worker/spacenamer/mocks"
)

type ManifoldSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) TestInputs(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	cfg, _, _ := validManifoldConfig(ctrl)
	cfg.NewClient = nil

	c.Check(spacenamer.Manifold(cfg).Inputs, jc.DeepEquals, []string{"agent-name", "api-caller-name"})
}

func (*ManifoldSuite) TestStartMissingNewFacade(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	cfg, _, _ := validManifoldConfig(ctrl)
	cfg.NewClient = nil

	work, err := spacenamer.Manifold(cfg).Start(newStubContext())
	c.Check(work, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "missing NewClient function not valid")
}

func (*ManifoldSuite) TestStartMissingNewWorker(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	cfg, _, _ := validManifoldConfig(ctrl)
	cfg.NewWorker = nil

	work, err := spacenamer.Manifold(cfg).Start(newStubContext())
	c.Check(work, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "missing NewWorker function not valid")
}

func (*ManifoldSuite) TestStartMissingLogger(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	cfg, _, _ := validManifoldConfig(ctrl)
	cfg.Logger = nil

	work, err := spacenamer.Manifold(cfg).Start(newStubContext())
	c.Check(work, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "missing Logger not valid")
}

func (s *ManifoldSuite) TestStartMissingAgentName(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	cfg, _, _ := validManifoldConfig(ctrl)
	ctx := dt.StubContext(nil, map[string]interface{}{
		"agent-name":      dependency.ErrMissing,
		"api-caller-name": &dummyAPICaller{},
	})

	work, err := spacenamer.Manifold(cfg).Start(ctx)
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

	work, err := spacenamer.Manifold(cfg).Start(ctx)
	c.Check(work, gc.IsNil)
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestStartSuccess(c *gc.C) {
}

func (s *ManifoldSuite) TestStartError(c *gc.C) {
}

// validManifoldConfig returns a valid manifold config created from mocks based
// on the incoming controller. The mocked facade and worker are returned.
func validManifoldConfig(ctrl *gomock.Controller) (spacenamer.ManifoldConfig, spacenamer.SpaceNamerAPI, worker.Worker) {
	api := mocks.NewMockSpaceNamerAPI(ctrl)
	work := workermocks.NewMockWorker(ctrl)

	cfg := newManifoldConfig(
		voidLogger(ctrl),
		func(_ base.APICaller) spacenamer.SpaceNamerAPI { return api },
		func(_ spacenamer.WorkerConfig) (worker.Worker, error) { return work, nil },
	)

	return cfg, api, work
}

// newManifoldConfig creates and returns a new ManifoldConfig instance based on
// the supplied arguments.
func newManifoldConfig(
	logger spacenamer.Logger,
	newClient func(base.APICaller) spacenamer.SpaceNamerAPI,
	newWorker func(spacenamer.WorkerConfig) (worker.Worker, error),
) spacenamer.ManifoldConfig {
	return spacenamer.ManifoldConfig{
		AgentName:     "agent-name",
		APICallerName: "api-caller-name",
		NewClient:     newClient,
		NewWorker:     newWorker,
		Logger:        logger,
	}
}

// voidLogger creates a new mock Logger that with no call verification.
func voidLogger(ctrl *gomock.Controller) spacenamer.Logger {
	log := mocks.NewMockLogger(ctrl)

	exp := log.EXPECT()
	any := gomock.Any()
	exp.Debugf(any, any).AnyTimes()
	exp.Infof(any, any).AnyTimes()
	exp.Warningf(any, any).AnyTimes()
	exp.Errorf(any, any).AnyTimes()

	return log
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
