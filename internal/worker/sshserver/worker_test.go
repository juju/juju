// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"context"
	"testing"
	"time"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/clock"
	"github.com/juju/tc"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/workertest"

	"github.com/juju/juju/controller"
	coremachine "github.com/juju/juju/core/machine"
	coressh "github.com/juju/juju/core/ssh"
	"github.com/juju/juju/core/user"
	virtualhostname "github.com/juju/juju/core/virtualhostname"
	"github.com/juju/juju/core/watcher/watchertest"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

type workerSuite struct {
	testhelpers.IsolationSuite
}

func TestWorkerSuite(t *testing.T) {
	testhelpers.PrintGoroutineLeaks(t, func(t *testing.T) {
		tc.Run(t, &workerSuite{})
	})
}

type startedServer struct {
	config ServerWorkerConfig
	worker worker.Worker
}

func newServerWrapperWorkerConfig(
	c *tc.C, ctrl *gomock.Controller, modifier func(*ServerWrapperWorkerConfig),
) *ServerWrapperWorkerConfig {
	cfg := &ServerWrapperWorkerConfig{
		NewServerWorker:         func(ServerWorkerConfig) (worker.Worker, error) { return nil, nil },
		ControllerConfigService: NewMockControllerConfigService(ctrl),
		SSHService:              stubSSHService{jumpHostKey: testHostKey, virtualHostKey: testHostKey},
		Logger:                  loggertesting.WrapCheckLog(c),
		Metrics:                 NewMetricsCollector(),
	}
	setMockServerDependencies(ctrl, cfg)

	modifier(cfg)

	return cfg
}

func setMockServerDependencies(ctrl *gomock.Controller, cfg *ServerWrapperWorkerConfig) {
	cfg.Authenticator = NewMockAuthenticator(ctrl)
	cfg.Authorizer = NewMockAuthorizer(ctrl)
	cfg.ServerFactory = newTerminatingServerFactory(
		NewMockProxyFactory(ctrl),
		stubSSHService{jumpHostKey: testHostKey, virtualHostKey: testHostKey},
		clock.WallClock,
	)
	cfg.Clock = clock.WallClock
}

func (s *workerSuite) TestValidate(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	cfg := newServerWrapperWorkerConfig(c, ctrl, func(cfg *ServerWrapperWorkerConfig) {})
	c.Assert(cfg.Validate(), tc.IsNil)

	// Test no Metrics.
	cfg = newServerWrapperWorkerConfig(
		c,
		ctrl,
		func(cfg *ServerWrapperWorkerConfig) {
			cfg.Metrics = nil
		},
	)
	c.Assert(cfg.Validate(), tc.ErrorMatches, ".*missing Metrics.*")

	// Test no Logger.
	cfg = newServerWrapperWorkerConfig(
		c,
		ctrl,
		func(cfg *ServerWrapperWorkerConfig) {
			cfg.Logger = nil
		},
	)
	c.Assert(cfg.Validate(), tc.ErrorMatches, ".*is required.*")

	// Test no NewServerWorker.
	cfg = newServerWrapperWorkerConfig(
		c,
		ctrl,
		func(cfg *ServerWrapperWorkerConfig) {
			cfg.NewServerWorker = nil
		},
	)
	c.Assert(cfg.Validate(), tc.ErrorMatches, ".*is required.*")

	// Test no ServerFactory.
	cfg = newServerWrapperWorkerConfig(
		c,
		ctrl,
		func(cfg *ServerWrapperWorkerConfig) {
			cfg.ServerFactory = nil
		},
	)
	c.Assert(cfg.Validate(), tc.ErrorMatches, ".*is required.*")

	// Test no SSHService.
	cfg = newServerWrapperWorkerConfig(
		c,
		ctrl,
		func(cfg *ServerWrapperWorkerConfig) {
			cfg.SSHService = nil
		},
	)
	c.Assert(cfg.Validate(), tc.ErrorMatches, ".*is required.*")

	// Test no Clock.
	cfg = newServerWrapperWorkerConfig(
		c,
		ctrl,
		func(cfg *ServerWrapperWorkerConfig) {
			cfg.Clock = nil
		},
	)
	c.Assert(cfg.Validate(), tc.ErrorMatches, ".*Clock is required.*")
}

func (s *workerSuite) TestSSHServerWrapperWorkerCanBeKilled(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	serverWorker := workertest.NewErrorWorker(nil)
	defer workertest.DirtyKill(c, serverWorker)

	ch := make(chan []string)
	controllerConfigWatcher := watchertest.NewMockStringsWatcher(ch)
	defer workertest.DirtyKill(c, controllerConfigWatcher)

	controllerConfigService := NewMockControllerConfigService(ctrl)
	controllerConfigService.EXPECT().WatchControllerConfig(gomock.Any()).Return(controllerConfigWatcher, nil)

	// Expect config to be called just the once.
	ctrlCfg := controller.Config{
		controller.SSHServerPort:               22,
		controller.SSHMaxConcurrentConnections: 10,
	}
	controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(ctrlCfg, nil).Times(1)

	cfg := ServerWrapperWorkerConfig{
		ControllerConfigService: controllerConfigService,
		SSHService:              stubSSHService{jumpHostKey: testHostKey, virtualHostKey: testHostKey},
		Logger:                  loggertesting.WrapCheckLog(c),
		Metrics:                 NewMetricsCollector(),
		NewServerWorker: func(swc ServerWorkerConfig) (worker.Worker, error) {
			c.Check(swc.JumpHostKey, tc.Equals, testHostKey)
			return serverWorker, nil
		},
	}
	setMockServerDependencies(ctrl, &cfg)
	w, err := NewServerWrapperWorker(cfg)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	// Check all workers alive properly.
	workertest.CheckAlive(c, w)
	workertest.CheckAlive(c, serverWorker)
	workertest.CheckAlive(c, controllerConfigWatcher)
	// Kill the wrapper worker.
	workertest.CleanKill(c, w)

	// Check all workers killed.
	c.Check(workertest.CheckKilled(c, w), tc.ErrorIsNil)
	c.Check(workertest.CheckKilled(c, serverWorker), tc.ErrorIsNil)
	c.Check(workertest.CheckKilled(c, controllerConfigWatcher), tc.ErrorIsNil)
}

func (s *workerSuite) TestSSHServerWrapperWorkerRestartsServerWorker(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ch := make(chan []string)
	controllerConfigWatcher := watchertest.NewMockStringsWatcher(ch)
	defer workertest.DirtyKill(c, controllerConfigWatcher)

	controllerConfigService := NewMockControllerConfigService(ctrl)
	controllerConfigService.EXPECT().WatchControllerConfig(gomock.Any()).Return(controllerConfigWatcher, nil)

	// Expect first call to have max concurrent connections of 10 and called once on worker startup.
	controllerConfigService.EXPECT().
		ControllerConfig(gomock.Any()).
		Return(
			controller.Config{
				controller.SSHServerPort:               22,
				controller.SSHMaxConcurrentConnections: 10,
			},
			nil,
		).
		Times(1)
	// The second call will be made if the worker receives changes on the watcher
	// and should should show no change and avoid restarting the worker.
	controllerConfigService.EXPECT().
		ControllerConfig(gomock.Any()).
		Return(
			controller.Config{
				controller.SSHServerPort:               22,
				controller.SSHMaxConcurrentConnections: 10,
			},
			nil,
		).
		Times(1)
	// On the third call, we're updating the max concurrent connections and should
	// see it restart the worker.
	controllerConfigService.EXPECT().
		ControllerConfig(gomock.Any()).
		Return(
			controller.Config{
				controller.SSHServerPort:               22,
				controller.SSHMaxConcurrentConnections: 15,
			},
			nil,
		).
		Times(1)

	servers := make(chan startedServer, 2)
	cfg := ServerWrapperWorkerConfig{
		ControllerConfigService: controllerConfigService,
		SSHService:              stubSSHService{jumpHostKey: testHostKey, virtualHostKey: testHostKey},
		Logger:                  loggertesting.WrapCheckLog(c),
		Metrics:                 NewMetricsCollector(),
		NewServerWorker: func(swc ServerWorkerConfig) (worker.Worker, error) {
			c.Check(swc.JumpHostKey, tc.Equals, testHostKey)
			w := workertest.NewErrorWorker(nil)
			servers <- startedServer{config: swc, worker: w}
			return w, nil
		},
	}
	setMockServerDependencies(ctrl, &cfg)
	w, err := NewServerWrapperWorker(cfg)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	first := <-servers
	defer workertest.DirtyKill(c, first.worker)
	c.Check(first.config.Port, tc.Equals, 22)
	c.Check(first.config.MaxConcurrentConnections, tc.Equals, 10)

	// Check all workers alive properly.
	workertest.CheckAlive(c, w)
	workertest.CheckAlive(c, first.worker)
	workertest.CheckAlive(c, controllerConfigWatcher)

	// Unchanged config: the server is not replaced.
	ch <- nil
	workertest.CheckAlive(c, w)
	workertest.CheckAlive(c, first.worker)
	c.Check(servers, tc.HasLen, 0)

	// Changed config: the server is replaced and the wrapper stays up.
	ch <- nil
	second := <-servers
	defer workertest.DirtyKill(c, second.worker)
	c.Check(second.config.Port, tc.Equals, 22)
	c.Check(second.config.MaxConcurrentConnections, tc.Equals, 15)

	c.Check(workertest.CheckKilled(c, first.worker), tc.ErrorIsNil)
	workertest.CheckAlive(c, w)
	workertest.CheckAlive(c, second.worker)
	workertest.CheckAlive(c, controllerConfigWatcher)

	workertest.CleanKill(c, w)
	c.Check(workertest.CheckKilled(c, second.worker), tc.ErrorIsNil)
}

func (s *workerSuite) TestSSHServerWrapperWorkerRestartsServerWorkerOnPortChange(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ch := make(chan []string)
	controllerConfigWatcher := watchertest.NewMockStringsWatcher(ch)
	defer workertest.DirtyKill(c, controllerConfigWatcher)

	controllerConfigService := NewMockControllerConfigService(ctrl)
	controllerConfigService.EXPECT().WatchControllerConfig(gomock.Any()).Return(controllerConfigWatcher, nil)

	// First call on startup.
	controllerConfigService.EXPECT().
		ControllerConfig(gomock.Any()).
		Return(
			controller.Config{
				controller.SSHServerPort:               22,
				controller.SSHMaxConcurrentConnections: 10,
			},
			nil,
		).
		Times(1)
	// Second call after first watcher event: unchanged.
	controllerConfigService.EXPECT().
		ControllerConfig(gomock.Any()).
		Return(
			controller.Config{
				controller.SSHServerPort:               22,
				controller.SSHMaxConcurrentConnections: 10,
			},
			nil,
		).
		Times(1)
	// Third call after second watcher event: port changed.
	controllerConfigService.EXPECT().
		ControllerConfig(gomock.Any()).
		Return(
			controller.Config{
				controller.SSHServerPort:               23,
				controller.SSHMaxConcurrentConnections: 10,
			},
			nil,
		).
		Times(1)

	servers := make(chan startedServer, 2)
	cfg := ServerWrapperWorkerConfig{
		ControllerConfigService: controllerConfigService,
		SSHService:              stubSSHService{jumpHostKey: testHostKey, virtualHostKey: testHostKey},
		Logger:                  loggertesting.WrapCheckLog(c),
		Metrics:                 NewMetricsCollector(),
		NewServerWorker: func(swc ServerWorkerConfig) (worker.Worker, error) {
			c.Check(swc.JumpHostKey, tc.Equals, testHostKey)
			w := workertest.NewErrorWorker(nil)
			servers <- startedServer{config: swc, worker: w}
			return w, nil
		},
	}
	setMockServerDependencies(ctrl, &cfg)
	w, err := NewServerWrapperWorker(cfg)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	first := <-servers
	defer workertest.DirtyKill(c, first.worker)
	c.Check(first.config.Port, tc.Equals, 22)

	workertest.CheckAlive(c, w)
	workertest.CheckAlive(c, first.worker)
	workertest.CheckAlive(c, controllerConfigWatcher)

	// First change: no restart expected.
	ch <- nil
	workertest.CheckAlive(c, w)
	workertest.CheckAlive(c, first.worker)
	c.Check(servers, tc.HasLen, 0)

	// Second change: port changed, server replaced and the wrapper stays up.
	ch <- nil
	second := <-servers
	defer workertest.DirtyKill(c, second.worker)
	c.Check(second.config.Port, tc.Equals, 23)
	c.Check(second.config.MaxConcurrentConnections, tc.Equals, 10)

	c.Check(workertest.CheckKilled(c, first.worker), tc.ErrorIsNil)
	workertest.CheckAlive(c, w)
	workertest.CheckAlive(c, second.worker)

	workertest.CleanKill(c, w)
	c.Check(workertest.CheckKilled(c, second.worker), tc.ErrorIsNil)
}

func (s *workerSuite) TestSSHServerWrapperWorkerConfigWatcherClosed(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	serverWorker := workertest.NewErrorWorker(nil)
	defer workertest.DirtyKill(c, serverWorker)

	ch := make(chan []string)
	controllerConfigWatcher := watchertest.NewMockStringsWatcher(ch)
	defer workertest.DirtyKill(c, controllerConfigWatcher)

	controllerConfigService := NewMockControllerConfigService(ctrl)
	controllerConfigService.EXPECT().WatchControllerConfig(gomock.Any()).Return(controllerConfigWatcher, nil)
	controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(controller.Config{
		controller.SSHServerPort:               22,
		controller.SSHMaxConcurrentConnections: 10,
	}, nil).Times(1)

	cfg := ServerWrapperWorkerConfig{
		ControllerConfigService: controllerConfigService,
		SSHService:              stubSSHService{jumpHostKey: testHostKey, virtualHostKey: testHostKey},
		Logger:                  loggertesting.WrapCheckLog(c),
		Metrics:                 NewMetricsCollector(),
		NewServerWorker: func(swc ServerWorkerConfig) (worker.Worker, error) {
			return serverWorker, nil
		},
	}
	setMockServerDependencies(ctrl, &cfg)
	w, err := NewServerWrapperWorker(cfg)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	workertest.CheckAlive(c, w)
	close(ch)

	err = workertest.CheckKilled(c, w)
	c.Check(err, tc.ErrorMatches, "controller config watcher closed")
}

func (s *workerSuite) TestWrapperWorkerReport(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ch := make(chan []string)
	controllerConfigWatcher := watchertest.NewMockStringsWatcher(ch)
	defer workertest.DirtyKill(c, controllerConfigWatcher)

	controllerConfigService := NewMockControllerConfigService(ctrl)
	controllerConfigService.EXPECT().WatchControllerConfig(gomock.Any()).Return(controllerConfigWatcher, nil)

	// Expect first call to have port of 22 and called once on worker startup.
	controllerConfigService.EXPECT().
		ControllerConfig(gomock.Any()).
		Return(
			controller.Config{
				controller.SSHServerPort:               22,
				controller.SSHMaxConcurrentConnections: 10,
			},
			nil,
		).
		Times(1)

	serverWorker := workertest.NewErrorWorker(nil)
	defer workertest.DirtyKill(c, serverWorker)

	cfg := ServerWrapperWorkerConfig{
		ControllerConfigService: controllerConfigService,
		SSHService:              stubSSHService{jumpHostKey: testHostKey, virtualHostKey: testHostKey},
		Logger:                  loggertesting.WrapCheckLog(c),
		Metrics:                 NewMetricsCollector(),
		NewServerWorker: func(swc ServerWorkerConfig) (worker.Worker, error) {
			return &reportWorker{serverWorker}, nil
		},
	}
	setMockServerDependencies(ctrl, &cfg)
	w, err := NewServerWrapperWorker(cfg)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	// Check all workers alive properly.
	workertest.CheckAlive(c, w)
	workertest.CheckAlive(c, serverWorker)
	workertest.CheckAlive(c, controllerConfigWatcher)

	// Check the wrapper worker is a reporter.
	reporter, ok := w.(worker.Reporter)
	c.Assert(ok, tc.IsTrue)

	// The server starts asynchronously, so wait for it to be reported.
	var server map[string]any
	for server == nil {
		workers, _ := reporter.Report(c.Context())["workers"].(map[string]any)
		if s, ok := workers[serverWorkerName].(map[string]any); ok && s[worker.KeyReport] != nil {
			server = s
			break
		}
		select {
		case <-c.Context().Done():
			c.Fatalf("server worker never reported")
		case <-time.After(time.Millisecond):
		}
	}
	c.Check(server[worker.KeyState], tc.Equals, "started")
	c.Check(server[worker.KeyReport], tc.DeepEquals, map[string]any{"test": "test"})
}

// reportWorker is a mock worker that implements the Reporter interface.
type reportWorker struct {
	worker.Worker
}

func (r *reportWorker) Report(ctx context.Context) map[string]any {
	return map[string]any{
		"test": "test",
	}
}

type stubSSHService struct {
	jumpHostKey    string
	virtualHostKey string
	jumpErr        error
	virtualErr     error
	resolveErr     error
	machineErr     error
}

func (s stubSSHService) SSHServerHostKey(context.Context) (string, error) {
	return s.jumpHostKey, s.jumpErr
}

func (s stubSSHService) GetPublicKeysForUser(context.Context, user.Name) ([]coressh.PublicKey, error) {
	return nil, nil
}

func (s stubSSHService) VirtualHostKey(context.Context, virtualhostname.Info) (string, error) {
	return s.virtualHostKey, s.virtualErr
}

func (s stubSSHService) ResolveK8sExecInfo(context.Context, virtualhostname.Info) (string, string, error) {
	return "", "", s.resolveErr
}

func (s stubSSHService) MachineForDestination(context.Context, virtualhostname.Info) (coremachine.Name, error) {
	return "", s.machineErr
}

const testHostKey = `-----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtz
c2gtZWQyNTUxOQAAACBT8UidoqUmpUFFCGEhZhHWGE7VHoJY7LZ7yXzuWlSVYAAA
AIiZq0wRmatMEQAAAAtzc2gtZWQyNTUxOQAAACBT8UidoqUmpUFFCGEhZhHWGE7V
HoJY7LZ7yXzuWlSVYAAAAEBYRsJTytYJUidtOuv3s3tdjyDA+4TSdCz9+hFKjyqz
v1PxSJ2ipSalQUUIYSFmEdYYTtUegljstnvJfO5aVJVgAAAAAAECAwQF
-----END OPENSSH PRIVATE KEY-----
`
