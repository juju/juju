// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"context"
	"net"
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"
	dt "github.com/juju/worker/v5/dependency/testing"
	"github.com/juju/worker/v5/workertest"
	"github.com/prometheus/client_golang/prometheus"
	gossh "golang.org/x/crypto/ssh"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/model"
	coressh "github.com/juju/juju/core/ssh"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/core/virtualhostname"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	controllersshservice "github.com/juju/juju/domain/ssh/service/controller"
	modelsshservice "github.com/juju/juju/domain/ssh/service/model"
	"github.com/juju/juju/internal/jwtparser"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/services"
	internalTunneler "github.com/juju/juju/internal/sshtunneler"
	"github.com/juju/juju/internal/testhelpers"
)

type manifoldSuite struct {
	testhelpers.IsolationSuite

	controllerConfigService *MockControllerConfigService
	controllerSSHService    *controllersshservice.Service
	sshService              *modelsshservice.WatchableService
}

func TestManifoldSuite(t *testing.T) {
	testhelpers.PrintGoroutineLeaks(t, func(t *testing.T) {
		tc.Run(t, &manifoldSuite{})
	})
}

func (s *manifoldSuite) TestConfigValidate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Check config as expected.

	cfg := s.newManifoldConfig(c, func(cfg *ManifoldConfig) {})
	c.Assert(cfg.Validate(), tc.IsNil)

	// Entirely missing.
	cfg = s.newManifoldConfig(c, func(cfg *ManifoldConfig) {
		cfg.DomainServicesName = ""
		cfg.ControllerUUID = ""
		cfg.NewServerWrapperWorker = nil
		cfg.NewServerWorker = nil
		cfg.GetControllerConfigService = nil
		cfg.GetControllerSSHService = nil
		cfg.GetDomainServicesGetter = nil
		cfg.GetSSHService = nil
		cfg.Logger = nil
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), tc.IsTrue)

	// Missing domain services name.
	cfg = s.newManifoldConfig(c, func(cfg *ManifoldConfig) {
		cfg.DomainServicesName = ""
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), tc.IsTrue)

	// Missing ControllerUUID.
	cfg = s.newManifoldConfig(c, func(cfg *ManifoldConfig) {
		cfg.ControllerUUID = ""
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), tc.IsTrue)

	// Missing NewServerWrapperWorker.
	cfg = s.newManifoldConfig(c, func(cfg *ManifoldConfig) {
		cfg.NewServerWrapperWorker = nil
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), tc.IsTrue)

	// Missing NewServerWorker.
	cfg = s.newManifoldConfig(c, func(cfg *ManifoldConfig) {
		cfg.NewServerWorker = nil
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), tc.IsTrue)

	// Missing GetControllerConfigService.
	cfg = s.newManifoldConfig(c, func(cfg *ManifoldConfig) {
		cfg.GetControllerConfigService = nil
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), tc.IsTrue)

	// Missing GetControllerSSHService.
	cfg = s.newManifoldConfig(c, func(cfg *ManifoldConfig) {
		cfg.GetControllerSSHService = nil
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), tc.IsTrue)

	// Missing GetDomainServicesGetter.
	cfg = s.newManifoldConfig(c, func(cfg *ManifoldConfig) {
		cfg.GetDomainServicesGetter = nil
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), tc.IsTrue)

	// Missing Logger.
	cfg = s.newManifoldConfig(c, func(cfg *ManifoldConfig) {
		cfg.Logger = nil
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), tc.IsTrue)

	// Missing Prometheus registerer.
	cfg = s.newManifoldConfig(c, func(cfg *ManifoldConfig) {
		cfg.PrometheusRegisterer = nil
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), tc.IsTrue)

	// Missing GetSSHService.
	cfg = s.newManifoldConfig(c, func(cfg *ManifoldConfig) {
		cfg.GetSSHService = nil
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), tc.IsTrue)

}

func (s *manifoldSuite) TestManifoldStart(c *tc.C) {
	defer s.setupMocks(c).Finish()
	sshServiceCalled := false

	// Setup the manifold
	manifold := Manifold(ManifoldConfig{
		DomainServicesName:     "domain-services",
		SSHTunnelerName:        "ssh-tunneler",
		JWTParserName:          "jwt-parser",
		ControllerID:           "0",
		ControllerUUID:         "8419cd78-4993-4c3a-928e-c646226beeee",
		NewServerWrapperWorker: NewServerWrapperWorker,
		NewServerWorker: func(ServerWorkerConfig) (worker.Worker, error) {
			return workertest.NewErrorWorker(nil), nil
		},
		GetControllerConfigService: func(getter dependency.Getter, name string) (ControllerConfigService, error) {
			return s.controllerConfigService, nil
		},
		GetControllerSSHService: func(getter dependency.Getter, name string) (*controllersshservice.Service, error) {
			return s.controllerSSHService, nil
		},
		GetDomainServicesGetter: func(dependency.Getter, string) (services.DomainServicesGetter, error) {
			return stubDomainServicesGetter{}, nil
		},
		GetSSHService: func(context.Context, services.DomainServicesGetter, model.UUID) (*modelsshservice.WatchableService, error) {
			sshServiceCalled = true
			return s.sshService, nil
		},
		Logger:               loggertesting.WrapCheckLog(c),
		PrometheusRegisterer: prometheus.NewRegistry(),
	})

	// Check the inputs are as expected
	c.Assert(manifold.Inputs, tc.DeepEquals, []string{"domain-services", "ssh-tunneler", "jwt-parser"})

	// Start the worker
	result, err := manifold.Start(
		c.Context(),
		dt.StubGetter(map[string]any{
			"ssh-tunneler": stubTunnelTracker{},
			"jwt-parser":   &jwtparser.Parser{},
		}),
	)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, result)

	c.Check(result, tc.NotNil)
	c.Check(sshServiceCalled, tc.IsFalse)
	workertest.CleanKill(c, result)
}

func (s *manifoldSuite) TestSSHServiceVirtualHostKeyUsesRequestModelUUID(c *tc.C) {
	info, err := virtualhostname.NewInfoMachineTarget("8419cd78-4993-4c3a-928e-c646226beeee", "1")
	c.Assert(err, tc.ErrorIsNil)

	var resolvedModelUUID model.UUID
	sshService := sshService{
		controllerSSHService: nil,
		domainServicesGetter: stubDomainServicesGetter{},
		getSSHService: func(_ context.Context, _ services.DomainServicesGetter, modelUUID model.UUID) (*modelsshservice.WatchableService, error) {
			resolvedModelUUID = modelUUID
			return modelsshservice.NewWatchableService(
				&stubModelSSHState{},
				modelUUID,
				clock.WallClock,
				nil,
			), nil
		},
	}

	virtualHostKey, err := sshService.VirtualHostKey(c.Context(), info)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(virtualHostKey, tc.Equals, testHostKey)
	c.Check(resolvedModelUUID, tc.Equals, info.ModelUUID())
}

func (s *manifoldSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.controllerConfigService = NewMockControllerConfigService(ctrl)
	s.controllerSSHService = controllersshservice.NewService(stubControllerSSHState{})
	s.sshService = &modelsshservice.WatchableService{}

	s.controllerConfigService.EXPECT().WatchControllerConfig(gomock.Any()).DoAndReturn(func(context.Context) (watcher.Watcher[[]string], error) {
		return watchertest.NewMockStringsWatcher(make(<-chan []string)), nil
	}).AnyTimes()
	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).DoAndReturn(func(context.Context) (controller.Config, error) {
		return controller.Config{
			controller.SSHServerPort:               22,
			controller.SSHMaxConcurrentConnections: 10,
		}, nil
	}).AnyTimes()
	return ctrl
}

func (s *manifoldSuite) newManifoldConfig(c *tc.C, modifier func(cfg *ManifoldConfig)) *ManifoldConfig {
	cfg := &ManifoldConfig{
		DomainServicesName: "domain-services",
		SSHTunnelerName:    "ssh-tunneler",
		JWTParserName:      "jwt-parser",
		ControllerID:       "0",
		ControllerUUID:     "8419cd78-4993-4c3a-928e-c646226beeee",
		NewServerWrapperWorker: func(ServerWrapperWorkerConfig) (worker.Worker, error) {
			return nil, nil
		},
		NewServerWorker: func(ServerWorkerConfig) (worker.Worker, error) {
			return nil, nil
		},
		GetControllerConfigService: func(getter dependency.Getter, name string) (ControllerConfigService, error) {
			return s.controllerConfigService, nil
		},
		GetControllerSSHService: func(getter dependency.Getter, name string) (*controllersshservice.Service, error) {
			return s.controllerSSHService, nil
		},
		GetDomainServicesGetter: func(dependency.Getter, string) (services.DomainServicesGetter, error) {
			return stubDomainServicesGetter{}, nil
		},
		GetSSHService: func(context.Context, services.DomainServicesGetter, model.UUID) (*modelsshservice.WatchableService, error) {
			return s.sshService, nil
		},
		Logger:               loggertesting.WrapCheckLog(c),
		PrometheusRegisterer: prometheus.NewRegistry(),
	}

	modifier(cfg)

	return cfg
}

func (s *manifoldSuite) TestManifoldMissingDependency(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Setup the manifold
	manifold := Manifold(ManifoldConfig{
		DomainServicesName:     "domain-services",
		SSHTunnelerName:        "ssh-tunneler",
		JWTParserName:          "jwt-parser",
		ControllerID:           "0",
		ControllerUUID:         "8419cd78-4993-4c3a-928e-c646226beeee",
		NewServerWrapperWorker: NewServerWrapperWorker,
		NewServerWorker: func(ServerWorkerConfig) (worker.Worker, error) {
			return workertest.NewErrorWorker(nil), nil
		},
		GetControllerConfigService: func(getter dependency.Getter, name string) (ControllerConfigService, error) {
			return s.controllerConfigService, nil
		},
		GetControllerSSHService: func(getter dependency.Getter, name string) (*controllersshservice.Service, error) {
			return s.controllerSSHService, nil
		},
		GetDomainServicesGetter: func(dependency.Getter, string) (services.DomainServicesGetter, error) {
			return stubDomainServicesGetter{}, nil
		},
		GetSSHService: func(context.Context, services.DomainServicesGetter, model.UUID) (*modelsshservice.WatchableService, error) {
			return s.sshService, nil
		},
		Logger:               loggertesting.WrapCheckLog(c),
		PrometheusRegisterer: prometheus.NewRegistry(),
	})

	// Check the inputs are as expected
	c.Assert(manifold.Inputs, tc.DeepEquals, []string{"domain-services", "ssh-tunneler", "jwt-parser"})

	// Start the worker
	_, err := manifold.Start(
		c.Context(),
		dt.StubGetter(map[string]any{}),
	)
	c.Assert(err, tc.ErrorMatches, `.*unexpected resource name: ssh-tunneler.*`)
}

type stubDomainServicesGetter struct{}

type stubModelSSHState struct {
	modelsshservice.State
}

func (*stubModelSSHState) GetMachineVirtualHostKeyByMachineName(context.Context, string) (string, bool, error) {
	return testHostKey, true, nil
}

type stubControllerSSHState struct{}

func (stubControllerSSHState) GetSSHServerHostKey(context.Context) (string, error) {
	return testHostKey, nil
}

func (stubControllerSSHState) GetSSHServerHostPublicKey(context.Context) ([]byte, error) {
	return nil, nil
}

func (stubControllerSSHState) GetPublicKeysForUser(context.Context, user.Name) ([]coressh.PublicKey, error) {
	return nil, nil
}

func (stubDomainServicesGetter) ServicesForModel(context.Context, model.UUID) (services.DomainServices, error) {
	return nil, errors.NotImplementedf("unexpected ServicesForModel call")
}

type stubTunnelTracker struct{}

func (stubTunnelTracker) RequestTunnel(context.Context, internalTunneler.RequestArgs) (*gossh.Client, error) {
	return nil, errors.NotImplementedf("unexpected RequestTunnel call")
}

func (stubTunnelTracker) AuthenticateTunnel(string, string) (string, error) {
	return "", errors.NotImplementedf("unexpected AuthenticateTunnel call")
}

func (stubTunnelTracker) PushTunnel(context.Context, string, net.Conn) error {
	return errors.NotImplementedf("unexpected PushTunnel call")
}
