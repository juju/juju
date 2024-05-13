// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodelconfigmanager_test

import (
	"context"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	dt "github.com/juju/worker/v4/dependency/testing"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/caas"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/worker/caasmodelconfigmanager"
	"github.com/juju/juju/internal/worker/caasmodelconfigmanager/mocks"
)

var _ = gc.Suite(&manifoldSuite{})

type manifoldSuite struct {
	testing.IsolationSuite
	config caasmodelconfigmanager.ManifoldConfig
}

func (s *manifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.config = s.validConfig(c)
}

func (s *manifoldSuite) validConfig(c *gc.C) caasmodelconfigmanager.ManifoldConfig {
	return caasmodelconfigmanager.ManifoldConfig{
		APICallerName: "api-caller",
		BrokerName:    "broker",
		NewWorker: func(config caasmodelconfigmanager.Config) (worker.Worker, error) {
			return nil, nil
		},
		NewFacade: func(caller base.APICaller) (caasmodelconfigmanager.Facade, error) {
			return nil, nil
		},
		Logger: loggertesting.WrapCheckLog(c),
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
	c.Check(err, jc.ErrorIs, errors.NotValid)
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
	w, err := manifold.Start(context.Background(), dt.StubGetter(map[string]interface{}{
		"api-caller": struct{ base.APICaller }{APICaller: &mockAPICaller{}},
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
