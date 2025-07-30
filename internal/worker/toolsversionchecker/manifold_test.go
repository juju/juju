// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package toolsversionchecker

import (
	"context"

	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	dt "github.com/juju/worker/v4/dependency/testing"
	gomock "go.uber.org/mock/gomock"

	"github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/model"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

type ManifoldSuite struct {
	testhelpers.IsolationSuite

	mockModelConfigService *MockModelConfigService
	mockModelAgentService  *MockModelAgentService
	mockMachineService     *MockMachineService
}

func (s *ManifoldSuite) getConfig(c *tc.C) ManifoldConfig {
	return ManifoldConfig{
		AgentName:          "agent",
		DomainServicesName: "domain-services",
		GetModelUUID: func(context.Context, dependency.Getter, string) (model.UUID, error) {
			return model.UUID("123"), nil
		},
		GetDomainServices: func(context.Context, dependency.Getter, string, model.UUID) (domainServices, error) {
			return domainServices{
				config:  s.mockModelConfigService,
				agent:   s.mockModelAgentService,
				machine: s.mockMachineService,
			}, nil
		},
		NewWorker: func(VersionCheckerParams) worker.Worker { return nil },
		Logger:    loggertesting.WrapCheckLog(c),
	}
}

func (s *ManifoldSuite) newGetter() dependency.Getter {
	resources := map[string]any{}
	return dt.StubGetter(resources)
}

func (s *ManifoldSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.mockModelConfigService = NewMockModelConfigService(ctrl)
	s.mockModelAgentService = NewMockModelAgentService(ctrl)
	s.mockMachineService = NewMockMachineService(ctrl)

	c.Cleanup(func() {
		s.mockModelConfigService = nil
		s.mockModelAgentService = nil
		s.mockMachineService = nil
	})

	return ctrl
}

func (s *ManifoldSuite) TestValidateConfig(c *tc.C) {
	cfg := s.getConfig(c)
	c.Check(cfg.Validate(), tc.ErrorIsNil)

	cfg = s.getConfig(c)
	cfg.AgentName = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.DomainServicesName = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.GetModelUUID = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.GetDomainServices = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.NewWorker = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.Logger = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)
}

func (s *ManifoldSuite) TestStart(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.getConfig(c)

	_, err := Manifold(cfg).Start(c.Context(), s.newGetter())
	c.Assert(err, tc.ErrorIsNil)
}
