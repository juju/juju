// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pebblelokiconfig

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/canonical/gomock/gomock"
	"github.com/canonical/pebble/client"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/workertest"
	"gopkg.in/yaml.v3"

	"github.com/juju/juju/api/agent/logger"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

func TestWorkerSuite(t *testing.T) {
	testhelpers.PrintGoroutineLeaks(t, func(t *testing.T) {
		tc.Run(t, &workerSuite{})
	})
}

type workerSuite struct {
	testhelpers.IsolationSuite

	agent  *MockAgent
	config *MockConfig
	api    *MockLoggerAPI
	pebble *MockPebbleClient
	nw     *MockNotifyWatcher

	changes chan struct{}

	workerConfig WorkerConfig
}

// expectWatch sets up the WatchControllerLokiConfig expectation. Tests
// must call this (or expectWatchError) instead of relying on a default.
func (s *workerSuite) expectWatch() {
	s.api.EXPECT().WatchControllerLokiConfig(gomock.Any(), gomock.Any()).
		Return(s.nw, nil).AnyTimes()
}

func (s *workerSuite) expectWatchError(err error) {
	s.api.EXPECT().WatchControllerLokiConfig(gomock.Any(), gomock.Any()).
		Return(nil, err).AnyTimes()
}

func (s *workerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.agent = NewMockAgent(ctrl)
	s.config = NewMockConfig(ctrl)
	s.api = NewMockLoggerAPI(ctrl)
	s.pebble = NewMockPebbleClient(ctrl)
	s.nw = NewMockNotifyWatcher(ctrl)

	s.changes = make(chan struct{}, 10)

	tag := names.NewMachineTag("0")
	s.config.EXPECT().Tag().Return(tag).AnyTimes()
	s.config.EXPECT().Controller().Return(names.NewControllerTag("controller-uuid")).AnyTimes()
	s.config.EXPECT().Model().Return(names.NewModelTag("model-uuid")).AnyTimes()
	s.agent.EXPECT().CurrentConfig().Return(s.config).AnyTimes()

	s.nw.EXPECT().Changes().Return(s.changes).AnyTimes()
	s.nw.EXPECT().Kill().AnyTimes()
	s.nw.EXPECT().Wait().Return(nil).AnyTimes()

	s.workerConfig = WorkerConfig{
		Agent:  s.agent,
		API:    s.api,
		Clock:  clock.WallClock,
		Logger: loggertesting.WrapCheckLog(c),
		NewPebbleClient: func(string) (PebbleClient, error) {
			return s.pebble, nil
		},
	}

	c.Cleanup(func() {
		s.agent = nil
		s.config = nil
		s.api = nil
		s.pebble = nil
		s.nw = nil
	})

	return ctrl
}

func (s *workerSuite) TestWorkerConfigValidate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	c.Check(WorkerConfig{}.Validate(), tc.ErrorMatches, "missing agent not valid")

	config := s.workerConfig
	config.API = nil
	c.Check(config.Validate(), tc.ErrorMatches, "missing api not valid")

	config = s.workerConfig
	config.Clock = nil
	c.Check(config.Validate(), tc.ErrorMatches, "missing clock not valid")

	config = s.workerConfig
	config.Logger = nil
	c.Check(config.Validate(), tc.ErrorMatches, "missing logger not valid")

	config = s.workerConfig
	config.NewPebbleClient = nil
	c.Check(config.Validate(), tc.ErrorMatches, "missing NewPebbleClient not valid")

	c.Check(s.workerConfig.Validate(), tc.ErrorIsNil)
}

func (s *workerSuite) TestNewWorkerValidatesConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	config := s.workerConfig
	config.Agent = nil
	w, err := NewWorker(config)
	c.Assert(w, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "missing agent not valid")
}

func (s *workerSuite) TestNewWorkerWatchError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectWatchError(errors.New("watch boom"))

	w, err := NewWorker(s.workerConfig)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)
	err = workertest.CheckKilled(c, w)
	c.Assert(err, tc.ErrorMatches, "watching controller loki config: watch boom")
}

func (s *workerSuite) TestNormalStart(c *tc.C) {
	defer s.setupMocks(c).Finish()

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)
	workertest.CheckAlive(c, w)
}

// --- Reconciliation tests ---

func (s *workerSuite) TestReconcileOnInitialEvent(c *tc.C) {
	defer s.setupMocks(c).Finish()

	done := make(chan struct{})
	lokiConfig := logger.ControllerLokiConfig{
		Endpoint: "https://loki.example.com/loki/api/v1/push",
		CACert:   "ca-cert",
	}
	s.api.EXPECT().GetControllerLokiConfig(gomock.Any(), gomock.Any()).
		Return(lokiConfig, nil)
	s.pebble.EXPECT().AddLayer(gomock.Any()).
		DoAndReturn(func(opts *client.AddLayerOptions) error {
			c.Check(opts.Combine, tc.IsTrue)
			c.Check(opts.Label, tc.Equals, "juju-loki-log-forwarding")
			var layer layerYAML
			err := yaml.Unmarshal(opts.LayerData, &layer)
			c.Assert(err, tc.ErrorIsNil)
			target, ok := layer.LogTargets["juju-loki"]
			c.Assert(ok, tc.IsTrue)
			c.Check(target.Type, tc.Equals, "loki")
			c.Check(target.Location, tc.Equals, "https://loki.example.com/loki/api/v1/push")
			c.Check(target.Services, tc.DeepEquals, []string{"container-agent"})
			c.Check(target.Override, tc.Equals, "replace")
			c.Check(target.Labels["juju_controller"], tc.Equals, "controller-uuid")
			c.Check(target.Labels["juju_model"], tc.Equals, "model-uuid")
			c.Check(target.Labels["juju_agent"], tc.Equals, "machine-0")
			close(done)
			return nil
		})
	s.pebble.EXPECT().CloseIdleConnections().AnyTimes()

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	s.sendChange()
	s.waitChan(c, done)
}

func (s *workerSuite) TestReconcileEmptyEndpoint(c *tc.C) {
	defer s.setupMocks(c).Finish()

	done := make(chan struct{})
	s.api.EXPECT().GetControllerLokiConfig(gomock.Any(), gomock.Any()).
		Return(logger.ControllerLokiConfig{}, nil)
	s.pebble.EXPECT().AddLayer(gomock.Any()).
		DoAndReturn(func(opts *client.AddLayerOptions) error {
			var layer layerYAML
			err := yaml.Unmarshal(opts.LayerData, &layer)
			c.Assert(err, tc.ErrorIsNil)
			target := layer.LogTargets["juju-loki"]
			c.Check(target.Override, tc.Equals, "replace")
			c.Check(target.Type, tc.Equals, "loki")
			c.Check(target.Location, tc.Equals, "http://0.0.0.0:0")
			c.Check(target.Services, tc.DeepEquals, []string{"-all"})
			c.Check(target.Labels, tc.IsNil)
			close(done)
			return nil
		})
	s.pebble.EXPECT().CloseIdleConnections().AnyTimes()

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	s.sendChange()
	s.waitChan(c, done)
}

func (s *workerSuite) TestReconcileLokiConfigNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	done := make(chan struct{})
	// The API client restores params.CodeNotFound errors into a
	// coreerrors.NotFound-satisfying error before returning them to the
	// worker, so simulate that translated error here.
	s.api.EXPECT().GetControllerLokiConfig(gomock.Any(), gomock.Any()).
		Return(logger.ControllerLokiConfig{}, errors.NewNotFound(errors.New("loki config not found"), ""))
	s.pebble.EXPECT().AddLayer(gomock.Any()).
		DoAndReturn(func(opts *client.AddLayerOptions) error {
			var layer layerYAML
			err := yaml.Unmarshal(opts.LayerData, &layer)
			c.Assert(err, tc.ErrorIsNil)
			target := layer.LogTargets["juju-loki"]
			c.Check(target.Override, tc.Equals, "replace")
			c.Check(target.Type, tc.Equals, "loki")
			c.Check(target.Location, tc.Equals, "http://0.0.0.0:0")
			c.Check(target.Services, tc.DeepEquals, []string{"-all"})
			c.Check(target.Labels, tc.IsNil)
			close(done)
			return nil
		})
	s.pebble.EXPECT().CloseIdleConnections().AnyTimes()

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	s.sendChange()
	s.waitChan(c, done)
}

func (s *workerSuite) TestReconcileGetError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.api.EXPECT().GetControllerLokiConfig(gomock.Any(), gomock.Any()).
		Return(logger.ControllerLokiConfig{}, errors.New("get boom"))

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.sendChange()

	err := workertest.CheckKilled(c, w)
	c.Assert(err, tc.ErrorMatches, "getting controller loki config: get boom")
}

func (s *workerSuite) TestReconcileAddLayerPersistentErrorKillsWorker(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.api.EXPECT().GetControllerLokiConfig(gomock.Any(), gomock.Any()).
		Return(logger.ControllerLokiConfig{Endpoint: "https://loki.example.com"}, nil)
	// AddLayer always fails with a non-incompatible transient error.
	// retryAttempts is 3, so we expect 3 calls.
	s.pebble.EXPECT().AddLayer(gomock.Any()).
		Return(errors.New("connection refused")).Times(3)
	s.pebble.EXPECT().CloseIdleConnections().AnyTimes()

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.sendChange()

	err := workertest.CheckKilled(c, w)
	c.Assert(err, tc.ErrorMatches, "(?s).*connection refused.*")
}

func (s *workerSuite) TestReconcilePebbleIncompatible(c *tc.C) {
	defer s.setupMocks(c).Finish()

	done := make(chan struct{})
	s.api.EXPECT().GetControllerLokiConfig(gomock.Any(), gomock.Any()).
		Return(logger.ControllerLokiConfig{Endpoint: "https://loki.example.com"}, nil)
	s.pebble.EXPECT().AddLayer(gomock.Any()).
		DoAndReturn(func(opts *client.AddLayerOptions) error {
			close(done)
			return &client.Error{
				StatusCode: 400,
				Message:    `cannot add layer: invalid layer: unknown section "log-targets"`,
			}
		})
	s.pebble.EXPECT().CloseIdleConnections().AnyTimes()

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	s.sendChange()
	s.waitChan(c, done)

	// The worker should stay alive (incompatible is not fatal).
	workertest.CheckAlive(c, w)
}

func (s *workerSuite) TestReconcilePebbleIncompatibleSkipsSubsequent(c *tc.C) {
	defer s.setupMocks(c).Finish()

	done := make(chan struct{})
	s.api.EXPECT().GetControllerLokiConfig(gomock.Any(), gomock.Any()).
		Return(logger.ControllerLokiConfig{Endpoint: "https://loki.example.com"}, nil)
	s.pebble.EXPECT().AddLayer(gomock.Any()).
		DoAndReturn(func(opts *client.AddLayerOptions) error {
			close(done)
			return &client.Error{
				StatusCode: 400,
				Message:    `unknown section "log-targets"`,
			}
		})
	s.pebble.EXPECT().CloseIdleConnections().AnyTimes()

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	s.sendChange()
	s.waitChan(c, done)
	workertest.CheckAlive(c, w)

	// Send more changes — no additional AddLayer calls should happen.
	s.sendChange()
	s.sendChange()
	time.Sleep(100 * time.Millisecond)
	workertest.CheckAlive(c, w)
}

func (s *workerSuite) TestReconcileTransientErrorRetried(c *tc.C) {
	defer s.setupMocks(c).Finish()

	done := make(chan struct{})
	var mu sync.Mutex
	callCount := 0
	lokiConfig := logger.ControllerLokiConfig{Endpoint: "https://loki.example.com"}
	s.api.EXPECT().GetControllerLokiConfig(gomock.Any(), gomock.Any()).
		Return(lokiConfig, nil).AnyTimes()
	s.pebble.EXPECT().AddLayer(gomock.Any()).
		DoAndReturn(func(opts *client.AddLayerOptions) error {
			mu.Lock()
			callCount++
			mu.Unlock()
			if callCount == 1 {
				return errors.New("transient socket error")
			}
			close(done)
			return nil
		}).Times(2)
	s.pebble.EXPECT().CloseIdleConnections().AnyTimes()

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	s.sendChange()
	s.waitChan(c, done)
}

func (s *workerSuite) TestNewPebbleClientErrorRetried(c *tc.C) {
	defer s.setupMocks(c).Finish()

	done := make(chan struct{})
	var mu sync.Mutex
	callCount := 0
	s.workerConfig.NewPebbleClient = func(string) (PebbleClient, error) {
		mu.Lock()
		callCount++
		mu.Unlock()
		if callCount == 1 {
			return nil, errors.New("cannot create client")
		}
		return s.pebble, nil
	}
	lokiConfig := logger.ControllerLokiConfig{Endpoint: "https://loki.example.com"}
	s.api.EXPECT().GetControllerLokiConfig(gomock.Any(), gomock.Any()).
		Return(lokiConfig, nil).AnyTimes()
	s.pebble.EXPECT().AddLayer(gomock.Any()).
		DoAndReturn(func(opts *client.AddLayerOptions) error {
			close(done)
			return nil
		})
	s.pebble.EXPECT().CloseIdleConnections().AnyTimes()

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	s.sendChange()
	s.waitChan(c, done)
}

func (s *workerSuite) TestWatcherChannelClosed(c *tc.C) {
	defer s.setupMocks(c).Finish()

	close(s.changes)

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	err := workertest.CheckKilled(c, w)
	c.Assert(err, tc.ErrorMatches, "loki config watcher channel closed")
}

// --- BuildLayerYAML tests ---

func (s *workerSuite) TestBuildLayerYAML(c *tc.C) {
	defer s.setupMocks(c).Finish()

	lokiConfig := logger.ControllerLokiConfig{
		Endpoint: "https://loki.example.com/loki/api/v1/push",
		CACert:   "ca-cert",
	}
	data, err := BuildLayerYAML(
		lokiConfig,
		names.NewMachineTag("0"),
		names.NewControllerTag("controller-uuid"),
		names.NewModelTag("model-uuid"),
	)
	c.Assert(err, tc.ErrorIsNil)

	var layer layerYAML
	err = yaml.Unmarshal(data, &layer)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(layer.Summary, tc.Equals, "Juju Loki log forwarding")
	target, ok := layer.LogTargets["juju-loki"]
	c.Assert(ok, tc.IsTrue)
	c.Check(target.Type, tc.Equals, "loki")
	c.Check(target.Location, tc.Equals, "https://loki.example.com/loki/api/v1/push")
	c.Check(target.Services, tc.DeepEquals, []string{"container-agent"})
	c.Check(target.Override, tc.Equals, "replace")
	c.Check(target.Labels["juju_controller"], tc.Equals, "controller-uuid")
	c.Check(target.Labels["juju_model"], tc.Equals, "model-uuid")
	c.Check(target.Labels["juju_agent"], tc.Equals, "machine-0")
}

func (s *workerSuite) TestBuildLayerYAMLNoReservedLabels(c *tc.C) {
	defer s.setupMocks(c).Finish()

	data, err := BuildLayerYAML(
		logger.ControllerLokiConfig{Endpoint: "https://loki.example.com"},
		names.NewMachineTag("0"),
		names.NewControllerTag("controller-uuid"),
		names.NewModelTag("model-uuid"),
	)
	c.Assert(err, tc.ErrorIsNil)

	var layer layerYAML
	err = yaml.Unmarshal(data, &layer)
	c.Assert(err, tc.ErrorIsNil)

	for k := range layer.LogTargets["juju-loki"].Labels {
		c.Check(strings.HasPrefix(k, "pebble_"), tc.IsFalse)
	}
}

func (s *workerSuite) TestBuildLayerYAMLUsesCorrectServiceName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	data, err := BuildLayerYAML(
		logger.ControllerLokiConfig{Endpoint: "https://loki.example.com"},
		names.NewMachineTag("0"),
		names.NewControllerTag("controller-uuid"),
		names.NewModelTag("model-uuid"),
	)
	c.Assert(err, tc.ErrorIsNil)

	var layer layerYAML
	err = yaml.Unmarshal(data, &layer)
	c.Assert(err, tc.ErrorIsNil)
	target := layer.LogTargets["juju-loki"]
	c.Check(target.Services, tc.DeepEquals, []string{"container-agent"})
}

// --- ResolvePebbleSocket tests ---

func (s *workerSuite) TestResolvePebbleSocketConfigured(c *tc.C) {
	defer s.setupMocks(c).Finish()
	c.Check(ResolvePebbleSocket("/custom/socket"), tc.Equals, "/custom/socket")
}

func (s *workerSuite) TestResolvePebbleSocketEnv(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.PatchEnvironment("PEBBLE_SOCKET", "/env/socket")
	c.Check(ResolvePebbleSocket(""), tc.Equals, "/env/socket")
}

func (s *workerSuite) TestResolvePebbleSocketDefault(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.PatchEnvironment("PEBBLE_SOCKET", "")
	c.Check(ResolvePebbleSocket(""), tc.Equals, "/var/lib/pebble/default/.pebble.socket")
}

// --- IsIncompatiblePebbleError tests ---

func (s *workerSuite) TestIsIncompatiblePebbleError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"400 log-targets", &client.Error{StatusCode: 400, Message: `unknown section "log-targets"`}, true},
		{"400 unknown section", &client.Error{StatusCode: 400, Message: "unknown section foo"}, true},
		{"400 other", &client.Error{StatusCode: 400, Message: "some other error"}, false},
		{"500 log-targets", &client.Error{StatusCode: 500, Message: "log-targets error"}, false},
		{"generic", errors.New("some error"), false},
		{"nil", nil, false},
	}
	for _, test := range tests {
		c.Logf("test: %s", test.name)
		c.Check(IsIncompatiblePebbleError(test.err), tc.Equals, test.expected)
	}
}

// --- ManifoldConfig.Validate tests ---

func (s *workerSuite) TestManifoldConfigValidate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	c.Check(ManifoldConfig{}.Validate(), tc.ErrorMatches, "empty AgentName not valid")

	config := ManifoldConfig{
		AgentName:     "agent",
		APICallerName: "",
	}
	c.Check(config.Validate(), tc.ErrorMatches, "empty APICallerName not valid")

	config = ManifoldConfig{
		AgentName:     "agent",
		APICallerName: "api-caller",
	}
	c.Check(config.Validate(), tc.ErrorMatches, "missing Clock not valid")

	config = ManifoldConfig{
		AgentName:     "agent",
		APICallerName: "api-caller",
		Clock:         clock.WallClock,
	}
	c.Check(config.Validate(), tc.ErrorMatches, "missing Logger not valid")

	config = ManifoldConfig{
		AgentName:     "agent",
		APICallerName: "api-caller",
		Clock:         clock.WallClock,
		Logger:        loggertesting.WrapCheckLog(c),
	}
	c.Check(config.Validate(), tc.ErrorMatches, "missing NewPebbleClient not valid")

	config.NewPebbleClient = func(string) (PebbleClient, error) {
		return nil, nil
	}
	c.Check(config.Validate(), tc.ErrorIsNil)
}

// --- Helpers ---

func (s *workerSuite) newWorker(c *tc.C) worker.Worker {
	s.expectWatch()
	c.Assert(s.workerConfig.Validate(), tc.ErrorIsNil)
	w, err := NewWorker(s.workerConfig)
	c.Assert(err, tc.ErrorIsNil)
	return w
}

func (s *workerSuite) sendChange() {
	s.changes <- struct{}{}
}

func (s *workerSuite) waitChan(c *tc.C, ch chan struct{}) {
	select {
	case <-ch:
	case <-time.After(testhelpers.LongWait):
		c.Fatalf("timed out waiting for channel signal")
	}
}

// layerYAML mirrors the pebbleLayer struct for test assertions.
type layerYAML struct {
	Summary     string                   `yaml:"summary,omitempty"`
	Description string                   `yaml:"description,omitempty"`
	LogTargets  map[string]logTargetYAML `yaml:"log-targets,omitempty"`
}

type logTargetYAML struct {
	Override string            `yaml:"override,omitempty"`
	Type     string            `yaml:"type"`
	Location string            `yaml:"location"`
	Services []string          `yaml:"services,omitempty"`
	Labels   map[string]string `yaml:"labels,omitempty"`
}
