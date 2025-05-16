// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiaddresssetter

import (
	"context"
	stdtesting "testing"

	"github.com/juju/tc"
	"github.com/juju/testing"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	gomock "go.uber.org/mock/gomock"

	controller "github.com/juju/juju/controller"
	"github.com/juju/juju/core/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type manifoldConfigSuite struct {
	testing.IsolationSuite

	config ManifoldConfig
}

func TestManifoldConfigSuite(t *stdtesting.T) { tc.Run(t, &manifoldConfigSuite{}) }
func (s *manifoldConfigSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.config = validConfig(c)
}

func (s *manifoldConfigSuite) TestMissingDomainServicesName(c *tc.C) {
	s.config.DomainServicesName = ""
	s.checkNotValid(c, "empty DomainServicesName not valid")
}

func (s *manifoldConfigSuite) TestMissingGetDomainServices(c *tc.C) {
	s.config.GetDomainServices = nil
	s.checkNotValid(c, "nil GetDomainServices not valid")
}

func (s *manifoldConfigSuite) TestMissingNewWorker(c *tc.C) {
	s.config.NewWorker = nil
	s.checkNotValid(c, "nil NewWorker not valid")
}

func (s *manifoldConfigSuite) TestMissingLogger(c *tc.C) {
	s.config.Logger = nil
	s.checkNotValid(c, "nil Logger not valid")
}

func (s *manifoldConfigSuite) checkNotValid(c *tc.C, expect string) {
	err := s.config.Validate()
	c.Check(err, tc.ErrorMatches, expect)
	c.Check(err, tc.ErrorIs, errors.NotValid)
}

func validConfig(c *tc.C) ManifoldConfig {
	return ManifoldConfig{
		DomainServicesName: "domain-services",
		GetDomainServices:  GetDomainServices,
		NewWorker:          func(Config) (worker.Worker, error) { return noWorker{}, nil },
		Logger:             loggertesting.WrapCheckLog(c),
	}
}

type manifoldSuite struct {
	testing.IsolationSuite

	domainServices          *MockDomainServices
	controllerConfigService *MockControllerConfigService
}

func TestManifoldSuite(t *stdtesting.T) { tc.Run(t, &manifoldSuite{}) }
func (s *manifoldSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.domainServices = NewMockDomainServices(ctrl)
	s.controllerConfigService = NewMockControllerConfigService(ctrl)

	return ctrl
}

func (s *manifoldSuite) TestStartSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.domainServices.EXPECT().Application().Return(noService{})
	s.domainServices.EXPECT().Network().Return(noService{})
	s.domainServices.EXPECT().ControllerNode().Return(noService{})
	s.domainServices.EXPECT().ControllerConfig().Return(s.controllerConfigService)
	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(controller.Config{
		"api-port":            1234,
		"controller-api-port": 4321,
	}, nil)

	cfg := ManifoldConfig{
		DomainServicesName: "domain-services",
		GetDomainServices: func(getter dependency.Getter, name string) (DomainServices, error) {
			return s.domainServices, nil
		},
		NewWorker: func(cfg Config) (worker.Worker, error) {
			if err := cfg.Validate(); err != nil {
				return nil, err
			}
			return noWorker{}, nil
		},
		Logger: loggertesting.WrapCheckLog(c),
	}

	w, err := Manifold(cfg).Start(context.Background(), noGetter{})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(w, tc.NotNil)
}

type noGetter struct {
	dependency.Getter
}

type noService struct {
	ApplicationService
	ControllerNodeService
	NetworkService
}

type noWorker struct {
	worker.Worker
}
