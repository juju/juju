// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodelconfigmanager_test

import (
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3"
	dt "github.com/juju/worker/v3/dependency/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/worker/caasmodelconfigmanager"
	"github.com/juju/juju/worker/caasmodelconfigmanager/mocks"
)

var _ = gc.Suite(&manifoldSuite{})

type manifoldSuite struct {
	testing.IsolationSuite
	config caasmodelconfigmanager.ManifoldConfig
}

func (s *manifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.config = s.validConfig()
}

func (s *manifoldSuite) validConfig() caasmodelconfigmanager.ManifoldConfig {
	return caasmodelconfigmanager.ManifoldConfig{
		APICallerName: "api-caller",
		BrokerName:    "broker",
		NewWorker: func(config caasmodelconfigmanager.Config) (worker.Worker, error) {
			return nil, nil
		},
		NewFacade: func(caller base.APICaller) (caasmodelconfigmanager.Facade, error) {
			return nil, nil
		},
		Logger: loggo.GetLogger("test"),
		Clock:  testclock.NewClock(time.Time{}),
	}
}

func (s *manifoldSuite) TestValid(c *gc.C) {
	c.Check(s.config.Validate(), jc.ErrorIsNil)
}

func (s *manifoldSuite) TestMissingAPICallerName(c *gc.C) {
	s.config.APICallerName = ""
	s.checkNotValid(c, "empty APICallerName not valid")
}

func (s *manifoldSuite) TestMissingBrokerName(c *gc.C) {
	s.config.BrokerName = ""
	s.checkNotValid(c, "empty BrokerName not valid")
}

func (s *manifoldSuite) TestMissingNewFacade(c *gc.C) {
	s.config.NewFacade = nil
	s.checkNotValid(c, "nil NewFacade not valid")
}

func (s *manifoldSuite) TestMissingNewWorker(c *gc.C) {
	s.config.NewWorker = nil
	s.checkNotValid(c, "nil NewWorker not valid")
}

func (s *manifoldSuite) TestMissingLogger(c *gc.C) {
	s.config.Logger = nil
	s.checkNotValid(c, "nil Logger not valid")
}

func (s *manifoldSuite) TestMissingClock(c *gc.C) {
	s.config.Clock = nil
	s.checkNotValid(c, "nil Clock not valid")
}

func (s *manifoldSuite) checkNotValid(c *gc.C, expect string) {
	err := s.config.Validate()
	c.Check(err, gc.ErrorMatches, expect)
	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *manifoldSuite) TestStart(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	called := false
	s.config.NewFacade = func(caller base.APICaller) (caasmodelconfigmanager.Facade, error) {
		return mocks.NewMockFacade(ctrl), nil
	}
	s.config.NewWorker = func(config caasmodelconfigmanager.Config) (worker.Worker, error) {
		called = true
		mc := jc.NewMultiChecker()
		mc.AddExpr(`_.Facade`, gc.NotNil)
		mc.AddExpr(`_.Broker`, gc.NotNil)
		mc.AddExpr(`_.Logger`, gc.NotNil)
		mc.AddExpr(`_.RegistryFunc`, gc.NotNil)
		mc.AddExpr(`_.Clock`, gc.NotNil)
		c.Check(config, mc, caasmodelconfigmanager.Config{
			ModelTag: names.NewModelTag("ffffffff-ffff-ffff-ffff-ffffffffffff"),
		})
		return nil, nil
	}
	manifold := caasmodelconfigmanager.Manifold(s.config)
	w, err := manifold.Start(dt.StubContext(nil, map[string]interface{}{
		"api-caller": struct{ base.APICaller }{&mockAPICaller{}},
		"broker":     struct{ caas.Broker }{},
	}))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(w, gc.IsNil)
	c.Assert(called, jc.IsTrue)
}

type mockAPICaller struct {
	base.APICaller
}

func (*mockAPICaller) BestFacadeVersion(facade string) int {
	return 1
}

func (*mockAPICaller) ModelTag() (names.ModelTag, bool) {
	return names.NewModelTag("ffffffff-ffff-ffff-ffff-ffffffffffff"), true
}
