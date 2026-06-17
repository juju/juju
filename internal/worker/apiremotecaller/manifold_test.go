// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiremotecaller

import (
	"testing"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"
	dt "github.com/juju/worker/v5/dependency/testing"
	"go.uber.org/goleak"

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
		ObjectStoreServicesName: "object-store-services",
		APIInfo:                 &stubAPIInfoProvider{info: &api.Info{CACert: "cert", Tag: names.NewControllerAgentTag("0")}},
		Origin:                  names.NewControllerAgentTag("0"),
		Clock:                   clock.WallClock,
		Logger:                  loggertesting.WrapCheckLog(c),
		NewWorker: func(wc WorkerConfig) (worker.Worker, error) {
			return &fakeWorker{}, nil
		},
	}
}

func (s *ManifoldSuite) TestInputs(c *tc.C) {
	c.Check(s.manifold().Inputs, tc.DeepEquals, []string{"object-store-services"})
}

func (s *ManifoldSuite) TestValidateRequiresAPIInfo(c *tc.C) {
	config := s.config
	config.APIInfo = nil
	c.Check(config.Validate(), tc.ErrorIs, errors.NotValid)

	config.APIInfo = &stubAPIInfoProvider{info: &api.Info{CACert: "cert"}}
	c.Check(config.Validate(), tc.ErrorIsNil)
}

func (s *ManifoldSuite) TestValidateRequiresOrigin(c *tc.C) {
	config := s.config
	config.Origin = nil
	c.Check(config.Validate(), tc.ErrorIs, errors.NotValid)

	config.Origin = names.NewControllerAgentTag("0")
	c.Check(config.Validate(), tc.ErrorIsNil)
}

func (s *ManifoldSuite) TestNewWorkerArgs(c *tc.C) {
	clock := s.config.Clock

	var config WorkerConfig
	s.config.NewWorker = func(c WorkerConfig) (worker.Worker, error) {
		config = c
		return &fakeWorker{}, nil
	}

	getter := dt.StubGetter(map[string]any{
		"object-store-services": objectStoreServices{},
	})

	worker, err := s.manifold().Start(c.Context(), getter)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(worker, tc.NotNil)

	c.Check(config.Origin, tc.Equals, names.NewControllerAgentTag("0"))
	c.Check(config.Clock, tc.Equals, clock)
	c.Check(config.ControllerNodeService, tc.DeepEquals, &controllernodeservice.WatchableService{})
	c.Check(config.APIInfo.CACert, tc.Equals, "cert")
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

type stubAPIInfoProvider struct {
	info *api.Info
	err  error
}

func (p *stubAPIInfoProvider) APIInfo() (*api.Info, error) {
	return p.info, p.err
}
