// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner_test

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	dt "github.com/juju/worker/v4/dependency/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/caas"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/worker/caasapplicationprovisioner"
)

type ManifoldSuite struct {
	testing.IsolationSuite
	config caasapplicationprovisioner.ManifoldConfig
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.config = s.validConfig(c)
}

func (s *ManifoldSuite) validConfig(c *gc.C) caasapplicationprovisioner.ManifoldConfig {
	return caasapplicationprovisioner.ManifoldConfig{
		APICallerName: "api-caller",
		BrokerName:    "broker",
		ClockName:     "clock",
		NewWorker: func(config caasapplicationprovisioner.Config) (worker.Worker, error) {
			return nil, nil
		},
		Logger: loggertesting.WrapCheckLog(c),
	}
}

func (s *ManifoldSuite) TestValid(c *gc.C) {
	c.Check(s.config.Validate(), jc.ErrorIsNil)
}

func (s *ManifoldSuite) TestMissingAPICallerName(c *gc.C) {
	s.config.APICallerName = ""
	s.checkNotValid(c, "empty APICallerName not valid")
}

func (s *ManifoldSuite) TestMissingBrokerName(c *gc.C) {
	s.config.BrokerName = ""
	s.checkNotValid(c, "empty BrokerName not valid")
}

func (s *ManifoldSuite) TestMissingClockName(c *gc.C) {
	s.config.ClockName = ""
	s.checkNotValid(c, "empty ClockName not valid")
}

func (s *ManifoldSuite) TestMissingNewWorker(c *gc.C) {
	s.config.NewWorker = nil
	s.checkNotValid(c, "nil NewWorker not valid")
}

func (s *ManifoldSuite) TestMissingLogger(c *gc.C) {
	s.config.Logger = nil
	s.checkNotValid(c, "nil Logger not valid")
}

func (s *ManifoldSuite) checkNotValid(c *gc.C, expect string) {
	err := s.config.Validate()
	c.Check(err, gc.ErrorMatches, expect)
	c.Check(err, jc.ErrorIs, errors.NotValid)
}

func (s *ManifoldSuite) TestStart(c *gc.C) {
	called := false
	s.config.NewWorker = func(config caasapplicationprovisioner.Config) (worker.Worker, error) {
		called = true
		mc := jc.NewMultiChecker()
		mc.AddExpr(`_.Facade`, gc.NotNil)
		mc.AddExpr(`_.Broker`, gc.NotNil)
		mc.AddExpr(`_.Clock`, gc.NotNil)
		mc.AddExpr(`_.Logger`, gc.NotNil)
		mc.AddExpr(`_.NewAppWorker`, gc.NotNil)
		mc.AddExpr(`_.UnitFacade`, gc.NotNil)
		c.Check(config, mc, caasapplicationprovisioner.Config{
			ModelTag: names.NewModelTag("ffffffff-ffff-ffff-ffff-ffffffffffff"),
		})
		return nil, nil
	}
	manifold := caasapplicationprovisioner.Manifold(s.config)
	w, err := manifold.Start(context.Background(), dt.StubGetter(map[string]interface{}{
		"api-caller": struct{ base.APICaller }{&mockAPICaller{}},
		"broker":     struct{ caas.Broker }{},
		"clock":      struct{ clock.Clock }{},
	}))
	c.Assert(w, gc.IsNil)
	c.Assert(err, jc.ErrorIsNil)
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
