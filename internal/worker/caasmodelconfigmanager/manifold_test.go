// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodelconfigmanager_test

import (
	"context"
	"testing"
	"time"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"
	dt "github.com/juju/worker/v5/dependency/testing"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/watcher"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/caasmodelconfigmanager"
	"github.com/juju/juju/internal/worker/caasmodelconfigmanager/mocks"
)

func TestManifoldSuite(t *testing.T) {
	tc.Run(t, &manifoldSuite{})
}

type manifoldSuite struct {
	testhelpers.IsolationSuite
	config caasmodelconfigmanager.ManifoldConfig
}

func (s *manifoldSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.config = s.validConfig(c)
}

type mockControllerConfigService struct {
	caasmodelconfigmanager.ControllerConfigService
}

func (m *mockControllerConfigService) ControllerConfig(context.Context) (controller.Config, error) {
	return controller.Config{}, nil
}

func (m *mockControllerConfigService) WatchControllerConfig(context.Context) (watcher.StringsWatcher, error) {
	return nil, nil
}

func (s *manifoldSuite) validConfig(c *tc.C) caasmodelconfigmanager.ManifoldConfig {
	return caasmodelconfigmanager.ManifoldConfig{
		DomainServicesName: "domain-services",
		BrokerName:         "broker",
		ModelUUID:          "ffffffff-ffff-ffff-ffff-ffffffffffff",
		GetDomainServices: func(getter dependency.Getter, name string) (caasmodelconfigmanager.ControllerConfigService, error) {
			return &mockControllerConfigService{}, nil
		},
		NewWorker: func(config caasmodelconfigmanager.Config) (worker.Worker, error) {
			return nil, nil
		},
		Logger: loggertesting.WrapCheckLog(c),
		Clock:  testclock.NewClock(time.Time{}),
	}
}

func (s *manifoldSuite) TestValid(c *tc.C) {
	c.Check(s.config.Validate(), tc.ErrorIsNil)
}

func (s *manifoldSuite) TestMissingDomainServicesName(c *tc.C) {
	s.config.DomainServicesName = ""
	s.checkNotValid(c, "empty DomainServicesName not valid")
}

func (s *manifoldSuite) TestMissingBrokerName(c *tc.C) {
	s.config.BrokerName = ""
	s.checkNotValid(c, "empty BrokerName not valid")
}

func (s *manifoldSuite) TestMissingModelUUID(c *tc.C) {
	s.config.ModelUUID = ""
	s.checkNotValid(c, "empty ModelUUID not valid")
}

func (s *manifoldSuite) TestMissingGetDomainServices(c *tc.C) {
	s.config.GetDomainServices = nil
	s.checkNotValid(c, "nil GetDomainServices not valid")
}

func (s *manifoldSuite) TestMissingNewWorker(c *tc.C) {
	s.config.NewWorker = nil
	s.checkNotValid(c, "nil NewWorker not valid")
}

func (s *manifoldSuite) TestMissingLogger(c *tc.C) {
	s.config.Logger = nil
	s.checkNotValid(c, "nil Logger not valid")
}

func (s *manifoldSuite) TestMissingClock(c *tc.C) {
	s.config.Clock = nil
	s.checkNotValid(c, "nil Clock not valid")
}

func (s *manifoldSuite) checkNotValid(c *tc.C, expect string) {
	err := s.config.Validate()
	c.Check(err, tc.ErrorMatches, expect)
	c.Check(err, tc.ErrorIs, errors.NotValid)
}

func (s *manifoldSuite) TestStart(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	called := false
	mockFacade := mocks.NewMockFacade(ctrl)
	s.config.GetDomainServices = func(getter dependency.Getter, name string) (caasmodelconfigmanager.ControllerConfigService, error) {
		return mockFacade, nil
	}
	s.config.NewWorker = func(config caasmodelconfigmanager.Config) (worker.Worker, error) {
		called = true
		mc := tc.NewMultiChecker()
		mc.AddExpr(`_.Facade`, tc.NotNil)
		mc.AddExpr(`_.Broker`, tc.NotNil)
		mc.AddExpr(`_.Logger`, tc.NotNil)
		mc.AddExpr(`_.RegistryFunc`, tc.NotNil)
		mc.AddExpr(`_.Clock`, tc.NotNil)
		c.Check(config, mc, caasmodelconfigmanager.Config{
			ModelTag: names.NewModelTag("ffffffff-ffff-ffff-ffff-ffffffffffff"),
		})
		return nil, nil
	}
	manifold := caasmodelconfigmanager.Manifold(s.config)
	w, err := manifold.Start(c.Context(), dt.StubGetter(map[string]any{
		"broker": struct{ caas.Broker }{},
	}))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(w, tc.IsNil)
	c.Assert(called, tc.IsTrue)
}
