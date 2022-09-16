// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateconverter_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/worker/stateconverter"
	"github.com/juju/juju/worker/stateconverter/mocks"
)

var _ = gc.Suite(&manifoldConfigSuite{})

type manifoldConfigSuite struct {
	machiner *mocks.MockMachiner
	agent    *mocks.MockAgent
	config   *mocks.MockConfig
	context  *mocks.MockContext
}

func (s *manifoldConfigSuite) TestValidateAgentNameFail(c *gc.C) {
	cfg := stateconverter.ManifoldConfig{}
	err := cfg.Validate()
	c.Assert(err.Error(), gc.Equals, errors.NotValidf("empty AgentName").Error())
}

func (s *manifoldConfigSuite) TestValidateAPICallerFail(c *gc.C) {
	cfg := stateconverter.ManifoldConfig{
		AgentName: "machine-2",
	}
	err := cfg.Validate()
	c.Assert(err.Error(), gc.Equals, errors.NotValidf("empty APICallerName").Error())
}

func (s *manifoldConfigSuite) TestValidateLoggerFail(c *gc.C) {
	cfg := stateconverter.ManifoldConfig{
		AgentName:     "machine-2",
		APICallerName: "machiner",
	}
	err := cfg.Validate()
	c.Assert(err.Error(), gc.Equals, errors.NotValidf("nil Logger").Error())
}

func (s *manifoldConfigSuite) TestValidateSuccess(c *gc.C) {
	cfg := stateconverter.ManifoldConfig{
		AgentName:     "machine-2",
		APICallerName: "machiner",
		Logger:        &fakeLogger{},
	}
	err := cfg.Validate()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *manifoldConfigSuite) TestManifoldStart(c *gc.C) {
	defer s.setupMocks(c).Finish()
	cfg := stateconverter.ManifoldConfig{
		AgentName:     "agent-name",
		APICallerName: "machiner",
		Logger:        &fakeLogger{},
		NewMachinerAPI: func(_ base.APICaller) stateconverter.Machiner {
			return s.machiner
		},
	}
	s.config.EXPECT().Tag().Return(names.NewMachineTag("3"))
	s.agent.EXPECT().CurrentConfig().Return(s.config)
	s.context.EXPECT().Get(cfg.AgentName, gomock.Any()).SetArg(1, s.agent).Return(nil)

	manifold := stateconverter.Manifold(cfg)
	w, err := manifold.Start(s.context)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(w, gc.NotNil)
}

func (s *manifoldConfigSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.agent = mocks.NewMockAgent(ctrl)
	s.config = mocks.NewMockConfig(ctrl)
	s.context = mocks.NewMockContext(ctrl)
	s.machiner = mocks.NewMockMachiner(ctrl)
	return ctrl
}

type fakeLogger struct{}

func (l *fakeLogger) Debugf(format string, args ...interface{})    {}
func (l *fakeLogger) Criticalf(format string, args ...interface{}) {}
func (l *fakeLogger) Tracef(format string, args ...interface{})    {}
