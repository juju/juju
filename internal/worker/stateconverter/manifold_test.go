// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateconverter_test

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/api/base"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/stateconverter"
	"github.com/juju/juju/internal/worker/stateconverter/mocks"
)

var _ = tc.Suite(&manifoldConfigSuite{})

type manifoldConfigSuite struct {
	machiner *mocks.MockMachiner
	agent    *mocks.MockAgent
	config   *mocks.MockConfig
	getter   *mocks.MockGetter
}

func (s *manifoldConfigSuite) TestValidateAgentNameFail(c *tc.C) {
	cfg := stateconverter.ManifoldConfig{}
	err := cfg.Validate()
	c.Assert(err.Error(), tc.Equals, errors.NotValidf("empty AgentName").Error())
}

func (s *manifoldConfigSuite) TestValidateAPICallerFail(c *tc.C) {
	cfg := stateconverter.ManifoldConfig{
		AgentName: "machine-2",
	}
	err := cfg.Validate()
	c.Assert(err.Error(), tc.Equals, errors.NotValidf("empty APICallerName").Error())
}

func (s *manifoldConfigSuite) TestValidateLoggerFail(c *tc.C) {
	cfg := stateconverter.ManifoldConfig{
		AgentName:     "machine-2",
		APICallerName: "machiner",
	}
	err := cfg.Validate()
	c.Assert(err.Error(), tc.Equals, errors.NotValidf("nil Logger").Error())
}

func (s *manifoldConfigSuite) TestValidateSuccess(c *tc.C) {
	cfg := stateconverter.ManifoldConfig{
		AgentName:     "machine-2",
		APICallerName: "machiner",
		Logger:        loggertesting.WrapCheckLog(c),
	}
	err := cfg.Validate()
	c.Assert(err, tc.ErrorIsNil)
}

func (s *manifoldConfigSuite) TestManifoldStart(c *tc.C) {
	defer s.setupMocks(c).Finish()

	done := make(chan any)
	cfg := stateconverter.ManifoldConfig{
		AgentName:     "agent-name",
		APICallerName: "machiner",
		Logger:        loggertesting.WrapCheckLog(c),
		NewMachinerAPI: func(_ base.APICaller) stateconverter.Machiner {
			return s.machiner
		},
	}
	gomock.InOrder(
		s.getter.EXPECT().Get(cfg.AgentName, gomock.Any()).SetArg(1, s.agent).Return(nil),
		s.agent.EXPECT().CurrentConfig().Return(s.config),
		s.config.EXPECT().Tag().Return(names.NewMachineTag("3")),
		s.machiner.EXPECT().Machine(gomock.Any(), names.NewMachineTag("3")).DoAndReturn(func(_ context.Context, _ names.MachineTag) (stateconverter.Machine, error) {
			close(done)
			return nil, errors.New("nope")
		}),
	)
	manifold := stateconverter.Manifold(cfg)
	w, err := manifold.Start(context.Background(), s.getter)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(w, tc.NotNil)
	select {
	case <-done:
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for calls")
	}
	err = workertest.CheckKill(c, w)
	c.Assert(err, tc.ErrorMatches, `nope`)
}

func (s *manifoldConfigSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.agent = mocks.NewMockAgent(ctrl)
	s.config = mocks.NewMockConfig(ctrl)
	s.getter = mocks.NewMockGetter(ctrl)
	s.machiner = mocks.NewMockMachiner(ctrl)
	return ctrl
}
