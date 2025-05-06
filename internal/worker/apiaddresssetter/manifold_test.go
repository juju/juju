// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiaddresssetter

import (
	"context"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	controller "github.com/juju/juju/controller"
	"github.com/juju/juju/core/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type manifoldConfigSuite struct {
	testing.IsolationSuite

	config ManifoldConfig
}

var _ = gc.Suite(&manifoldConfigSuite{})

func (s *manifoldConfigSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.config = validConfig(c)
}

func (s *manifoldConfigSuite) TestMissingDomainServicesName(c *gc.C) {
	s.config.DomainServicesName = ""
	s.checkNotValid(c, "empty DomainServicesName not valid")
}

func (s *manifoldConfigSuite) TestMissingGetControllerConfigService(c *gc.C) {
	s.config.GetControllerConfigService = nil
	s.checkNotValid(c, "nil GetControllerConfigService not valid")
}

func (s *manifoldConfigSuite) TestMissingGetApplicationService(c *gc.C) {
	s.config.GetApplicationService = nil
	s.checkNotValid(c, "nil GetApplicationService not valid")
}

func (s *manifoldConfigSuite) TestMissingGetNetworkService(c *gc.C) {
	s.config.GetNetworkService = nil
	s.checkNotValid(c, "nil GetNetworkService not valid")
}

func (s *manifoldConfigSuite) TestMissingNewWorker(c *gc.C) {
	s.config.NewWorker = nil
	s.checkNotValid(c, "nil NewWorker not valid")
}

func (s *manifoldConfigSuite) TestMissingLogger(c *gc.C) {
	s.config.Logger = nil
	s.checkNotValid(c, "nil Logger not valid")
}

func (s *manifoldConfigSuite) checkNotValid(c *gc.C, expect string) {
	err := s.config.Validate()
	c.Check(err, gc.ErrorMatches, expect)
	c.Check(err, jc.ErrorIs, errors.NotValid)
}

func validConfig(c *gc.C) ManifoldConfig {
	return ManifoldConfig{
		DomainServicesName:         "domain-services",
		GetControllerConfigService: GetControllerConfigService,
		GetApplicationService:      GetApplicationService,
		GetControllerNodeService:   GetControllerNodeService,
		GetNetworkService:          GetNetworkService,
		NewWorker:                  func(Config) (worker.Worker, error) { return noWorker{}, nil },
		Logger:                     loggertesting.WrapCheckLog(c),
	}
}

type manifoldSuite struct {
	testing.IsolationSuite

	controllerConfigService *MockControllerConfigService
}

var _ = gc.Suite(&manifoldSuite{})

func (s *manifoldSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.controllerConfigService = NewMockControllerConfigService(ctrl)

	return ctrl
}

func (s *manifoldSuite) TestStartSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(controller.Config{
		"api-port":            1234,
		"controller-api-port": 4321,
	}, nil)

	cfg := ManifoldConfig{
		DomainServicesName: "domain-services",
		GetControllerConfigService: func(getter dependency.Getter, name string) (ControllerConfigService, error) {
			return s.controllerConfigService, nil
		},
		GetApplicationService: func(getter dependency.Getter, name string) (ApplicationService, error) {
			return noService{}, nil
		},
		GetControllerNodeService: func(getter dependency.Getter, name string) (ControllerNodeService, error) {
			return noService{}, nil
		},
		GetNetworkService: func(getter dependency.Getter, name string) (NetworkService, error) {
			return noService{}, nil
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
	c.Assert(err, jc.ErrorIsNil)
	c.Check(w, gc.NotNil)
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
