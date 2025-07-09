// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machineconverter

import (
	context "context"
	stdtesting "testing"
	"time"

	"github.com/juju/errors"
	names "github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/dependency"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"

	agent "github.com/juju/juju/agent"
	watcher "github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testing"
)

type manifoldConfigSuite struct {
	machineClient *MockMachineClient
	agentClient   *MockAgentClient
	agent         *MockAgent
	config        *MockConfig
	getter        *MockGetter
}

func TestManifoldConfigSuite(t *stdtesting.T) {
	tc.Run(t, &manifoldConfigSuite{})
}

func (s *manifoldConfigSuite) TestValidateAgentNameFail(c *tc.C) {
	cfg := ManifoldConfig{}
	err := cfg.Validate()
	c.Assert(err, tc.ErrorIs, errors.NotValid)
}

func (s *manifoldConfigSuite) TestValidateAPICallerFail(c *tc.C) {
	cfg := ManifoldConfig{
		AgentName: "machine-2",
	}
	err := cfg.Validate()
	c.Assert(err, tc.ErrorIs, errors.NotValid)
}

func (s *manifoldConfigSuite) TestValidateLoggerFail(c *tc.C) {
	cfg := ManifoldConfig{
		AgentName:     "machine-2",
		APICallerName: "api-caller",
	}
	err := cfg.Validate()
	c.Assert(err, tc.ErrorIs, errors.NotValid)
}

func (s *manifoldConfigSuite) TestValidateMachineClientFail(c *tc.C) {
	cfg := ManifoldConfig{
		AgentName:     "machine-2",
		APICallerName: "api-caller",
		Logger:        loggertesting.WrapCheckLog(c),
	}
	err := cfg.Validate()
	c.Assert(err, tc.ErrorIs, errors.NotValid)
}

func (s *manifoldConfigSuite) TestValidateAgentClientFail(c *tc.C) {
	cfg := ManifoldConfig{
		AgentName:     "machine-2",
		APICallerName: "api-caller",
		Logger:        loggertesting.WrapCheckLog(c),
		NewMachineClient: func(dependency.Getter, string) (MachineClient, error) {
			return s.machineClient, nil
		},
	}
	err := cfg.Validate()
	c.Assert(err, tc.ErrorIs, errors.NotValid)
}

func (s *manifoldConfigSuite) TestValidateSuccess(c *tc.C) {
	cfg := ManifoldConfig{
		AgentName:     "machine-2",
		APICallerName: "api-caller",
		Logger:        loggertesting.WrapCheckLog(c),
		NewMachineClient: func(dependency.Getter, string) (MachineClient, error) {
			return s.machineClient, nil
		},
		NewAgentClient: func(dependency.Getter, string) (AgentClient, error) {
			return s.agentClient, nil
		},
		NewConverter: func(cfg Config) (watcher.NotifyHandler, error) {
			return nil, nil
		},
	}
	err := cfg.Validate()
	c.Assert(err, tc.ErrorIsNil)
}

func (s *manifoldConfigSuite) TestManifoldStart(c *tc.C) {
	defer s.setupMocks(c).Finish()

	done := make(chan struct{})

	s.getter.EXPECT().Get("agent-name", gomock.Any()).DoAndReturn(func(_ string, a any) error {
		if _, ok := a.(*agent.Agent); !ok {
			return errors.NotValidf("expected *MockAgent, got %T", a)
		}
		x := a.(*agent.Agent)
		*x = s.agent
		return nil
	})

	cfg := ManifoldConfig{
		AgentName:     "agent-name",
		APICallerName: "api-caller",
		Logger:        loggertesting.WrapCheckLog(c),
		NewMachineClient: func(dependency.Getter, string) (MachineClient, error) {
			return s.machineClient, nil
		},
		NewAgentClient: func(dependency.Getter, string) (AgentClient, error) {
			return s.agentClient, nil
		},
		NewConverter: func(cfg Config) (watcher.NotifyHandler, error) {
			defer close(done)
			return &testHandler{
				w: watchertest.NewMockNotifyWatcher(make(<-chan struct{})),
			}, nil
		},
	}

	manifold := Manifold(cfg)
	w, err := manifold.Start(c.Context(), s.getter)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	select {
	case <-done:
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for calls")
	}
	workertest.CleanKill(c, w)
}

func (s *manifoldConfigSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.config = NewMockConfig(ctrl)
	s.config.EXPECT().Tag().Return(names.NewMachineTag("3")).AnyTimes()

	s.agent = NewMockAgent(ctrl)
	s.agent.EXPECT().CurrentConfig().Return(s.config).AnyTimes()

	s.getter = NewMockGetter(ctrl)
	s.machineClient = NewMockMachineClient(ctrl)
	s.agentClient = NewMockAgentClient(ctrl)

	c.Cleanup(func() {
		s.agent = nil
		s.config = nil
		s.getter = nil
		s.machineClient = nil
		s.agentClient = nil
	})

	return ctrl
}

type testHandler struct {
	w watcher.NotifyWatcher
}

func (h *testHandler) SetUp(ctx context.Context) (watcher.NotifyWatcher, error) {
	return h.w, nil
}

func (h *testHandler) Handle(ctx context.Context) error {
	return nil
}

func (h *testHandler) TearDown() error {
	h.w.Kill()
	return h.w.Wait()
}
