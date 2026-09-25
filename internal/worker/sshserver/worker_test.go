// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/workertest"

	"github.com/juju/juju/controller"
	coremachine "github.com/juju/juju/core/machine"
	coressh "github.com/juju/juju/core/ssh"
	"github.com/juju/juju/core/user"
	virtualhostname "github.com/juju/juju/core/virtualhostname"
	"github.com/juju/juju/core/watcher"
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
	cfg.ProxyFactory = NewMockProxyFactory(ctrl)
	cfg.TunnelTracker = NewMockTunnelTracker(ctrl)
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

	// Test no ProxyFactory.
	cfg = newServerWrapperWorkerConfig(
		c,
		ctrl,
		func(cfg *ServerWrapperWorkerConfig) {
			cfg.ProxyFactory = nil
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

	serverWorker := workertest.NewErrorWorker(nil)
	defer workertest.DirtyKill(c, serverWorker)

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

	var serverStarted int32
	cfg := ServerWrapperWorkerConfig{
		ControllerConfigService: controllerConfigService,
		SSHService:              stubSSHService{jumpHostKey: testHostKey, virtualHostKey: testHostKey},
		Logger:                  loggertesting.WrapCheckLog(c),
		Metrics:                 NewMetricsCollector(),
		NewServerWorker: func(swc ServerWorkerConfig) (worker.Worker, error) {
			atomic.StoreInt32(&serverStarted, 1)
			// The port now comes from the SSH service (stub default), not
			// controller config; this test exercises the max-conns restart.
			c.Check(swc.Port, tc.Equals, controller.DefaultSSHServerPort)
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

	c.Check(atomic.LoadInt32(&serverStarted), tc.Equals, int32(1))

	// Send some changes to restart the server (expect no changes).
	ch <- nil

	workertest.CheckAlive(c, w)

	// Send some changes to restart the server (expect the worker to restart).
	ch <- nil

	err = workertest.CheckKilled(c, w)
	c.Check(err, tc.ErrorMatches, "changes detected, stopping SSH server worker")

	// Check all workers killed.
	c.Check(workertest.CheckKilled(c, w), tc.ErrorMatches, "changes detected, stopping SSH server worker")
	c.Check(workertest.CheckKilled(c, serverWorker), tc.ErrorIsNil)
	c.Check(workertest.CheckKilled(c, controllerConfigWatcher), tc.ErrorIsNil)
}

func (s *workerSuite) TestSSHServerWrapperWorkerRestartsServerWorkerOnPortChange(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	serverWorker := workertest.NewErrorWorker(nil)
	defer workertest.DirtyKill(c, serverWorker)

	ch := make(chan []string)
	controllerConfigWatcher := watchertest.NewMockStringsWatcher(ch)
	defer workertest.DirtyKill(c, controllerConfigWatcher)

	controllerConfigService := NewMockControllerConfigService(ctrl)
	controllerConfigService.EXPECT().WatchControllerConfig(gomock.Any()).Return(controllerConfigWatcher, nil)
	// Controller config is only read at startup here; the port change is driven
	// via the SSH server port watcher below.
	controllerConfigService.EXPECT().
		ControllerConfig(gomock.Any()).
		Return(
			controller.Config{
				controller.SSHMaxConcurrentConnections: 10,
			},
			nil,
		).
		AnyTimes()

	// The SSH server port is owned by the SSH service. Drive changes through a
	// controllable notify watcher and a mutable port value.
	portCh := make(chan struct{})
	portWatcher := watchertest.NewMockNotifyWatcher(portCh)
	defer workertest.DirtyKill(c, portWatcher)
	sshService := &mutablePortSSHService{
		stubSSHService: stubSSHService{jumpHostKey: testHostKey, virtualHostKey: testHostKey},
		portWatcher:    portWatcher,
	}
	sshService.setPort(22)

	cfg := ServerWrapperWorkerConfig{
		ControllerConfigService: controllerConfigService,
		SSHService:              sshService,
		Logger:                  loggertesting.WrapCheckLog(c),
		Metrics:                 NewMetricsCollector(),
		NewServerWorker: func(swc ServerWorkerConfig) (worker.Worker, error) {
			c.Check(swc.Port, tc.Equals, 22)
			c.Check(swc.JumpHostKey, tc.Equals, testHostKey)
			return serverWorker, nil
		},
	}
	setMockServerDependencies(ctrl, &cfg)
	w, err := NewServerWrapperWorker(cfg)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	workertest.CheckAlive(c, w)
	workertest.CheckAlive(c, serverWorker)
	workertest.CheckAlive(c, controllerConfigWatcher)

	// First change: port unchanged, no restart expected.
	portCh <- struct{}{}
	workertest.CheckAlive(c, w)

	// Second change: port changed, restart expected.
	sshService.setPort(23)
	portCh <- struct{}{}
	err = workertest.CheckKilled(c, w)
	c.Check(err, tc.ErrorMatches, "changes detected, stopping SSH server worker")
}

// mutablePortSSHService is a stubSSHService whose port and port watcher can be
// controlled by tests to exercise the SSH server port watch/restart path.
type mutablePortSSHService struct {
	stubSSHService
	mu          sync.Mutex
	currentPort int
	portWatcher watcher.NotifyWatcher
}

func (s *mutablePortSSHService) setPort(port int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.currentPort = port
}

func (s *mutablePortSSHService) GetSSHServerPort(context.Context) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.currentPort, nil
}

func (s *mutablePortSSHService) WatchSSHServerPort(context.Context) (watcher.NotifyWatcher, error) {
	return s.portWatcher, nil
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

	c.Assert(reporter.Report(c.Context()), tc.DeepEquals, map[string]any{
		"workers": map[string]any{
			"ssh-server": map[string]any{
				"test": "test",
			},
		},
	})
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

	port         int
	portErr      error
	portWatcher  watcher.NotifyWatcher
	portWatchErr error
}

func (s stubSSHService) SSHServerHostKey(context.Context) (string, error) {
	return s.jumpHostKey, s.jumpErr
}

func (s stubSSHService) GetSSHServerPort(context.Context) (int, error) {
	if s.port == 0 && s.portErr == nil {
		return controller.DefaultSSHServerPort, nil
	}
	return s.port, s.portErr
}

func (s stubSSHService) WatchSSHServerPort(context.Context) (watcher.NotifyWatcher, error) {
	if s.portWatcher != nil || s.portWatchErr != nil {
		return s.portWatcher, s.portWatchErr
	}
	// Default: a watcher that never fires, kept alive until closed.
	return watchertest.NewMockNotifyWatcher(make(chan struct{})), nil
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
