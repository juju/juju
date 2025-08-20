// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiremotecaller

import (
	"testing"

	"github.com/juju/clock"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	dt "github.com/juju/worker/v4/dependency/testing"
	"go.uber.org/goleak"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	controllernodeservice "github.com/juju/juju/domain/controllernode/service"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/testhelpers"
)

type ManifoldSuite struct {
	testhelpers.IsolationSuite

	config ManifoldConfig
}

func TestManifoldSuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &ManifoldSuite{})
}

func (s *ManifoldSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.config = ManifoldConfig{
		AgentName:               "agent",
		ObjectStoreServicesName: "object-store-services",
		Clock:                   clock.WallClock,
		Logger:                  loggertesting.WrapCheckLog(c),
		NewWorker: func(wc WorkerConfig) (worker.Worker, error) {
			return &fakeWorker{}, nil
		},
	}
}

func (s *ManifoldSuite) TestInputs(c *tc.C) {
	c.Check(s.manifold().Inputs, tc.DeepEquals, []string{"agent", "object-store-services"})
}

func (s *ManifoldSuite) TestAgentMissing(c *tc.C) {
	getter := dt.StubGetter(map[string]interface{}{
		"agent": dependency.ErrMissing,
	})

	worker, err := s.manifold().Start(c.Context(), getter)
	c.Assert(err, tc.ErrorIs, dependency.ErrMissing)
	c.Check(worker, tc.IsNil)
}

func (s *ManifoldSuite) TestAgentAPIInfoNotReady(c *tc.C) {
	getter := dt.StubGetter(map[string]interface{}{
		"agent":                 &fakeAgent{missingAPIinfo: true},
		"object-store-services": objectStoreServices{},
	})

	worker, err := s.manifold().Start(c.Context(), getter)
	c.Assert(err, tc.ErrorIs, dependency.ErrMissing)
	c.Check(worker, tc.IsNil)
}

func (s *ManifoldSuite) TestNewWorkerArgs(c *tc.C) {
	clock := s.config.Clock

	var config WorkerConfig
	s.config.NewWorker = func(c WorkerConfig) (worker.Worker, error) {
		config = c
		return &fakeWorker{}, nil
	}

	getter := dt.StubGetter(map[string]interface{}{
		"agent":                 &fakeAgent{tag: names.NewMachineTag("42")},
		"object-store-services": objectStoreServices{},
	})

	worker, err := s.manifold().Start(c.Context(), getter)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(worker, tc.NotNil)

	c.Check(config.Origin, tc.Equals, names.NewMachineTag("42"))
	c.Check(config.Clock, tc.Equals, clock)
	c.Check(config.ControllerNodeService, tc.DeepEquals, &controllernodeservice.WatchableService{})
	c.Check(config.APIInfo.CACert, tc.Equals, "fake as")
	c.Check(config.NewRemote, tc.NotNil)
}

func (s *ManifoldSuite) manifold() dependency.Manifold {
	return Manifold(s.config)
}

type objectStoreServices struct {
	services.ObjectStoreServices
}

func (d objectStoreServices) ControllerNode() *controllernodeservice.WatchableService {
	return &controllernodeservice.WatchableService{}
}

type fakeWorker struct {
	worker.Worker
}

type fakeAgent struct {
	agent.Agent

	tag            names.Tag
	missingAPIinfo bool
}

type fakeConfig struct {
	agent.Config

	tag            names.Tag
	missingAPIinfo bool
}

func (f *fakeAgent) CurrentConfig() agent.Config {
	return &fakeConfig{tag: f.tag, missingAPIinfo: f.missingAPIinfo}
}

func (f *fakeConfig) APIInfo() (*api.Info, bool) {
	if f.missingAPIinfo {
		return nil, false
	}
	return &api.Info{
		CACert: "fake as",
		Tag:    f.tag,
	}, true
}

func (f *fakeConfig) Tag() names.Tag {
	return f.tag
}
