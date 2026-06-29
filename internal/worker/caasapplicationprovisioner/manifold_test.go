// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner_test

import (
	"testing"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"
	dt "github.com/juju/worker/v5/dependency/testing"

	"github.com/juju/juju/caas"
	agentpasswordservice "github.com/juju/juju/domain/agentpassword/service"
	applicationservice "github.com/juju/juju/domain/application/service"
	controllerconfigservice "github.com/juju/juju/domain/controllerconfig/service"
	controllernodeservice "github.com/juju/juju/domain/controllernode/service"
	modelservice "github.com/juju/juju/domain/model/service"
	modelconfigservice "github.com/juju/juju/domain/modelconfig/service"
	removalservice "github.com/juju/juju/domain/removal/service"
	resourceservice "github.com/juju/juju/domain/resource/service"
	statusservice "github.com/juju/juju/domain/status/service"
	storageprovisioningservice "github.com/juju/juju/domain/storageprovisioning/service"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/caasapplicationprovisioner"
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
		BrokerName:         "broker",
		ClockName:          "clock",
		DomainServicesName: "domain-services",
		GetDomainServices: func(getter dependency.Getter, name string) (services.DomainServices, error) {
			return &mockDomainServices{}, nil
		},
		NewWorker: func(config caasapplicationprovisioner.Config) (worker.Worker, error) {
			return nil, nil
		},
		Logger: loggertesting.WrapCheckLog(c),
	}
}

func (s *ManifoldSuite) TestValid(c *tc.C) {
	c.Check(s.config.Validate(), tc.ErrorIsNil)
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

func (s *ManifoldSuite) TestMissingGetDomainServices(c *tc.C) {
	s.config.GetDomainServices = nil
	s.checkNotValid(c, "nil GetDomainServices not valid")
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
	ds := &mockDomainServices{
		appSvc:        &applicationservice.WatchableService{},
		ctrlConfigSvc: &controllerconfigservice.WatchableService{},
		ctrlNodeSvc:   &controllernodeservice.WatchableService{},
		modelCfgSvc:   &modelconfigservice.WatchableService{},
		modelInfoSvc:  &modelservice.ProviderModelService{},
		removalSvc:    &removalservice.WatchableService{},
		resourceSvc:   &resourceservice.Service{},
		statusSvc:     &statusservice.LeadershipService{},
		agentPwdSvc:   &agentpasswordservice.Service{},
		storageSvc:    &storageprovisioningservice.Service{},
	}
	s.config.GetDomainServices = func(getter dependency.Getter, name string) (services.DomainServices, error) {
		return ds, nil
	}

	called := false
	s.config.NewWorker = func(config caasapplicationprovisioner.Config) (worker.Worker, error) {
		called = true
		return nil, nil
	}
	manifold := caasapplicationprovisioner.Manifold(s.config)
	w, err := manifold.Start(c.Context(), dt.StubGetter(map[string]any{
		"broker":          struct{ caas.Broker }{},
		"clock":           struct{ clock.Clock }{},
		"domain-services": ds,
	}))
	c.Assert(w, tc.IsNil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(called, tc.IsTrue)
}

// mockDomainServices implements services.DomainServices for testing.
// It embeds the interface so that unimplemented methods return zero values.
// Only the methods called by start() are explicitly implemented.
type mockDomainServices struct {
	services.DomainServices
	appSvc        *applicationservice.WatchableService
	ctrlConfigSvc *controllerconfigservice.WatchableService
	ctrlNodeSvc   *controllernodeservice.WatchableService
	modelCfgSvc   *modelconfigservice.WatchableService
	modelInfoSvc  *modelservice.ProviderModelService
	removalSvc    *removalservice.WatchableService
	resourceSvc   *resourceservice.Service
	statusSvc     *statusservice.LeadershipService
	agentPwdSvc   *agentpasswordservice.Service
	storageSvc    *storageprovisioningservice.Service
}

func (m *mockDomainServices) Application() *applicationservice.WatchableService { return m.appSvc }
func (m *mockDomainServices) ControllerConfig() *controllerconfigservice.WatchableService {
	return m.ctrlConfigSvc
}
func (m *mockDomainServices) ControllerNode() *controllernodeservice.WatchableService {
	return m.ctrlNodeSvc
}
func (m *mockDomainServices) Config() *modelconfigservice.WatchableService  { return m.modelCfgSvc }
func (m *mockDomainServices) ModelInfo() *modelservice.ProviderModelService { return m.modelInfoSvc }
func (m *mockDomainServices) Removal() *removalservice.WatchableService     { return m.removalSvc }
func (m *mockDomainServices) Resource() *resourceservice.Service            { return m.resourceSvc }
func (m *mockDomainServices) Status() *statusservice.LeadershipService      { return m.statusSvc }
func (m *mockDomainServices) AgentPassword() *agentpasswordservice.Service  { return m.agentPwdSvc }
func (m *mockDomainServices) StorageProvisioning() *storageprovisioningservice.Service {
	return m.storageSvc
}
