// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradedatabase_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/juju/v2/state"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/v2/worker/upgradedatabase"
	. "github.com/juju/juju/v2/worker/upgradedatabase/mocks"
)

type manifoldSuite struct {
	baseSuite
}

var _ = gc.Suite(&manifoldSuite{})

func (s *manifoldSuite) TestValidateConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.getConfig()
	c.Check(cfg.Validate(), jc.ErrorIsNil)

	cfg.UpgradeDBGateName = ""
	c.Check(cfg.Validate(), jc.Satisfies, errors.IsNotValid)

	cfg = s.getConfig()
	cfg.Logger = nil
	c.Check(cfg.Validate(), jc.Satisfies, errors.IsNotValid)

	cfg = s.getConfig()
	cfg.OpenState = nil
	c.Check(cfg.Validate(), jc.Satisfies, errors.IsNotValid)

	cfg = s.getConfig()
	cfg.Clock = nil
	c.Check(cfg.Validate(), jc.Satisfies, errors.IsNotValid)
}

func (s *manifoldSuite) getConfig() upgradedatabase.ManifoldConfig {
	return upgradedatabase.ManifoldConfig{
		AgentName:         "agent-name",
		UpgradeDBGateName: "upgrade-database-lock",
		Logger:            s.logger,
		OpenState:         func() (*state.StatePool, error) { return nil, nil },
		Clock:             clock.WallClock,
	}
}

func (s *manifoldSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.logger = NewMockLogger(ctrl)
	s.ignoreLogging(c)

	return ctrl
}
