// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner_test

import (
	"testing"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	dt "github.com/juju/worker/v4/dependency/testing"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/caas"
	applicationservice "github.com/juju/juju/domain/application/service"
	statusservice "github.com/juju/juju/domain/status/service"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/caasapplicationprovisioner"
	"github.com/juju/juju/internal/worker/caasapplicationprovisioner/mocks"
)

type ManifoldSuite struct {
	testhelpers.IsolationSuite
	config caasapplicationprovisioner.ManifoldConfig
}

func TestManifoldSuite(t *testing.T) {
	tc.Run(t, &ManifoldSuite{})
}

func (s *ManifoldSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.config = s.validConfig(c)
}

func (s *ManifoldSuite) validConfig(c *tc.C) caasapplicationprovisioner.ManifoldConfig {
	return caasapplicationprovisioner.ManifoldConfig{
		APICallerName:      "api-caller",
		BrokerName:         "broker",
		ClockName:          "clock",
		DomainServicesName: "domain-services",
		NewWorker: func(config caasapplicationprovisioner.Config) (worker.Worker, error) {
			return nil, nil
		},
		Logger: loggertesting.WrapCheckLog(c),
	}
}

func (s *ManifoldSuite) TestValid(c *tc.C) {
	c.Check(s.config.Validate(), tc.ErrorIsNil)
}

func (s *ManifoldSuite) TestMissingAPICallerName(c *tc.C) {
	s.config.APICallerName = ""
	s.checkNotValid(c, "empty APICallerName not valid")
}

func (s *ManifoldSuite) TestMissingDomainServicesName(c *tc.C) {
	s.config.DomainServicesName = ""
	s.checkNotValid(c, "empty DomainServicesName not valid")
}

func (s *ManifoldSuite) TestMissingBrokerName(c *tc.C) {
	s.config.BrokerName = ""
	s.checkNotValid(c, "empty BrokerName not valid")
}

func (s *ManifoldSuite) TestMissingClockName(c *tc.C) {
	s.config.ClockName = ""
	s.checkNotValid(c, "empty ClockName not valid")
}

func (s *ManifoldSuite) TestMissingNewWorker(c *tc.C) {
	s.config.NewWorker = nil
	s.checkNotValid(c, "nil NewWorker not valid")
}

func (s *ManifoldSuite) TestMissingLogger(c *tc.C) {
	s.config.Logger = nil
	s.checkNotValid(c, "nil Logger not valid")
}

func (s *ManifoldSuite) checkNotValid(c *tc.C, expect string) {
	err := s.config.Validate()
	c.Check(err, tc.ErrorMatches, expect)
	c.Check(err, tc.ErrorIs, errors.NotValid)
}

func (s *ManifoldSuite) TestStart(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockDomainServices := mocks.NewMockModelDomainServices(ctrl)
	mockDomainServices.EXPECT().Application().Return(&applicationservice.WatchableService{})
	mockDomainServices.EXPECT().Status().Return(&statusservice.LeadershipService{})

	called := false
	s.config.NewWorker = func(config caasapplicationprovisioner.Config) (worker.Worker, error) {
		called = true
		mc := tc.NewMultiChecker()
		mc.AddExpr(`_.Facade`, tc.NotNil)
		mc.AddExpr(`_.Broker`, tc.NotNil)
		mc.AddExpr(`_.Clock`, tc.NotNil)
		mc.AddExpr(`_.Logger`, tc.NotNil)
		mc.AddExpr(`_.NewAppWorker`, tc.NotNil)
		mc.AddExpr(`_.UnitFacade`, tc.NotNil)
		mc.AddExpr(`_.ApplicationService`, tc.NotNil)
		mc.AddExpr(`_.StatusService`, tc.NotNil)
		c.Check(config, mc, caasapplicationprovisioner.Config{
			ModelTag: names.NewModelTag("ffffffff-ffff-ffff-ffff-ffffffffffff"),
		})
		return nil, nil
	}
	manifold := caasapplicationprovisioner.Manifold(s.config)
	w, err := manifold.Start(c.Context(), dt.StubGetter(map[string]any{
		"api-caller":      struct{ base.APICaller }{&mockAPICaller{}},
		"broker":          struct{ caas.Broker }{},
		"clock":           struct{ clock.Clock }{},
		"domain-services": mockDomainServices,
	}))
	c.Assert(w, tc.IsNil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(called, tc.IsTrue)
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

type mockDomainServices struct {
	services.ModelDomainServices
}

func (*mockDomainServices) Application() *applicationservice.WatchableService {
	return nil
}
