// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradestepsagent

import (
	context "context"
	"maps"
	"testing"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"
	dt "github.com/juju/worker/v5/dependency/testing"
	"github.com/juju/worker/v5/workertest"
	gomock "go.uber.org/mock/gomock"

	"github.com/juju/juju/agent"
	base "github.com/juju/juju/api/base"
	"github.com/juju/juju/core/logger"
	version "github.com/juju/juju/core/semversion"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/upgrades"
	"github.com/juju/juju/internal/upgradesteps"
	"github.com/juju/juju/internal/worker/gate"
)

type manifoldSuite struct {
	testhelpers.IsolationSuite

	agent        *MockAgent
	agentConfig  *MockConfig
	apiCaller    *MockAPICaller
	statusSetter *MockStatusSetter
}

func TestManifoldSuite(t *testing.T) {
	tc.Run(t, &manifoldSuite{})
}

func (s *manifoldSuite) TestValidateConfig(c *tc.C) {
	cfg := s.getConfig(c)
	c.Check(cfg.Validate(), tc.ErrorIsNil)

	cfg = s.getConfig(c)
	cfg.AgentName = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.APICallerName = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.UpgradeStepsGateName = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.PreUpgradeSteps = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.UpgradeSteps = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.NewAgentStatusSetter = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.Logger = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.Clock = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)
}

func (s *manifoldSuite) getConfig(c *tc.C) ManifoldConfig {
	return ManifoldConfig{
		AgentName:            "agent",
		APICallerName:        "api-caller",
		UpgradeStepsGateName: "upgrade-steps-lock",
		PreUpgradeSteps:      func(agent.Config) error { return nil },
		UpgradeSteps: func(from version.Number, targets []upgrades.Target, context upgrades.Context) error {
			return nil
		},
		NewAgentStatusSetter: func(ctx context.Context, a base.APICaller) (upgradesteps.StatusSetter, error) {
			return s.statusSetter, nil
		},
		NewAgentWorker: func(l1 gate.Lock, a1 agent.Agent, a2 base.APICaller, pusf upgrades.PreUpgradeStepsFunc, usf upgrades.UpgradeStepsFunc, ss upgradesteps.StatusSetter, l2 logger.Logger, c clock.Clock) worker.Worker {
			return workertest.NewErrorWorker(nil)
		},
		Logger: loggertesting.WrapCheckLog(c),
		Clock:  clock.WallClock,
	}
}

var expectedInputs = []string{"agent", "api-caller", "upgrade-steps-lock"}

func (s *manifoldSuite) TestInputs(c *tc.C) {
	c.Assert(Manifold(s.getConfig(c)).Inputs, tc.SameContents, expectedInputs)
}

func (s *manifoldSuite) TestStart(c *tc.C) {
	defer s.setupMocks(c).Finish()

	w, err := Manifold(s.getConfig(c)).Start(c.Context(), s.newGetter(nil))
	c.Assert(err, tc.ErrorIsNil)

	workertest.CheckAlive(c, w)

	workertest.CleanKill(c, w)
}

func (s *manifoldSuite) newGetter(overlay map[string]any) dependency.Getter {
	resources := map[string]any{
		"agent":              s.agent,
		"api-caller":         s.apiCaller,
		"upgrade-steps-lock": gate.NewLock(),
	}
	maps.Copy(resources, overlay)
	return dt.StubGetter(resources)
}

func (s *manifoldSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.agent = NewMockAgent(ctrl)
	s.agentConfig = NewMockConfig(ctrl)
	s.apiCaller = NewMockAPICaller(ctrl)
	s.statusSetter = NewMockStatusSetter(ctrl)

	c.Cleanup(func() {
		s.agent = nil
		s.agentConfig = nil
		s.apiCaller = nil
		s.statusSetter = nil
	})

	return ctrl
}
