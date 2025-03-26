// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradestepsmachine

import (
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/upgrades"
	version "github.com/juju/juju/internal/version"
)

type manifoldSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&manifoldSuite{})

func (s *manifoldSuite) TestValidateConfig(c *gc.C) {
	cfg := s.getConfig(c)
	c.Check(cfg.Validate(), jc.ErrorIsNil)

	cfg = s.getConfig(c)
	cfg.AgentName = ""
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.APICallerName = ""
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.UpgradeStepsGateName = ""
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.PreUpgradeSteps = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.Logger = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.Clock = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)
}

func (s *manifoldSuite) getConfig(c *gc.C) ManifoldConfig {
	return ManifoldConfig{
		AgentName:            "agent",
		APICallerName:        "api-caller",
		UpgradeStepsGateName: "upgrade-steps-lock",
		PreUpgradeSteps:      func(_ agent.Config, isController bool) error { return nil },
		UpgradeSteps: func(from version.Number, targets []upgrades.Target, context upgrades.Context) error {
			return nil
		},
		Logger: loggertesting.WrapCheckLog(c),
		Clock:  clock.WallClock,
	}
}

var expectedInputs = []string{"agent", "api-caller", "upgrade-steps-lock"}

func (s *manifoldSuite) TestInputs(c *gc.C) {
	c.Assert(Manifold(s.getConfig(c)).Inputs, jc.SameContents, expectedInputs)
}
