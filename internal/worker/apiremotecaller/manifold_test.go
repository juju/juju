// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiremotecaller

import (
	"context"
	"testing"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"
	dependencytesting "github.com/juju/worker/v5/dependency/testing"

	"github.com/juju/juju/api"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/watcher"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

type ManifoldSuite struct {
	testhelpers.IsolationSuite

	config ManifoldConfig
}

func TestManifoldSuite(t *testing.T) {
	testhelpers.PrintGoroutineLeaks(t, func(t *testing.T) {
		tc.Run(t, &ManifoldSuite{})
	})
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
	c.Check(s.config.Validate(), tc.ErrorIsNil)
}

func (s *ManifoldSuite) TestStartUsesControllerNetworkServiceAndAPIPort(c *tc.C) {
	network := &stubControllerNetworkService{}
	services := stubRemoteCallerServices{
		config:  controller.Config{"api-port": 17071},
		network: network,
	}
	config := s.config
	config.GetRemoteCallerServices = func(getter dependency.Getter, name string) (RemoteCallerServices, error) {
		c.Check(name, tc.Equals, config.ObjectStoreServicesName)
		return services, nil
	}
	config.NewWorker = func(cfg WorkerConfig) (worker.Worker, error) {
		c.Check(cfg.ControllerNetworkService, tc.Equals, network)
		c.Check(cfg.APIPort, tc.Equals, 17071)
		return &fakeWorker{}, nil
	}

	_, err := Manifold(config).Start(c.Context(), dependencytesting.StubGetter(nil))

	c.Check(err, tc.ErrorIsNil)
}

func (s *ManifoldSuite) TestStartPropagatesControllerConfigError(c *tc.C) {
	config := s.config
	config.GetRemoteCallerServices = func(dependency.Getter, string) (RemoteCallerServices, error) {
		return stubRemoteCallerServices{err: errors.New("boom")}, nil
	}
	config.NewWorker = func(WorkerConfig) (worker.Worker, error) {
		c.Fatalf("NewWorker should not be called")
		return nil, nil
	}

	_, err := Manifold(config).Start(c.Context(), dependencytesting.StubGetter(nil))

	c.Check(err, tc.ErrorMatches, "boom")
}

func (s *ManifoldSuite) manifold() dependency.Manifold {
	return Manifold(s.config)
}

type stubRemoteCallerServices struct {
	config  controller.Config
	network ControllerNetworkService
	err     error
}

func (s stubRemoteCallerServices) ControllerConfig(context.Context) (controller.Config, error) {
	return s.config, s.err
}

func (s stubRemoteCallerServices) Network() ControllerNetworkService {
	return s.network
}

type stubControllerNetworkService struct{}

func (*stubControllerNetworkService) GetControllerRemoteAPIAddresses(context.Context, int) (map[string][]string, error) {
	return nil, nil
}

func (*stubControllerNetworkService) WatchControllerRemoteEndpoints(context.Context) (watcher.NotifyWatcher, error) {
	return nil, nil
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
