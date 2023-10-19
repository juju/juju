// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradesteps

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/state"
	jujutesting "github.com/juju/juju/testing"
)

type manifoldSuite struct {
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
	cfg.OpenStateForUpgrade = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.PreUpgradeSteps = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.Logger = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)
}

func (s *manifoldSuite) getConfig(c *gc.C) ManifoldConfig {
	return ManifoldConfig{
		AgentName:            "agent",
		APICallerName:        "api-caller",
		UpgradeStepsGateName: "upgrade-steps-lock",
		OpenStateForUpgrade:  func() (*state.StatePool, SystemState, error) { return nil, nil, nil },
		PreUpgradeSteps:      func(_ agent.Config, isController, isCaas bool) error { return nil },
		Logger:               jujutesting.NewCheckLogger(c),
	}
}

var expectedInputs = []string{"agent", "api-caller", "upgrade-steps-lock"}

func (s *manifoldSuite) TestInputs(c *gc.C) {
	c.Assert(Manifold(s.getConfig(c)).Inputs, jc.SameContents, expectedInputs)
}
