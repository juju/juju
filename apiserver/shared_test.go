// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/model"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/lease"
)

type sharedServerContextSuite struct {
	controllerConfigService ControllerConfigService
}

func TestSharedServerContextSuite(t *stdtesting.T) {
	tc.Run(t, &sharedServerContextSuite{})
}
func (s *sharedServerContextSuite) TestConfigNoStatePool(c *tc.C) {
	defer s.setupMocks(c).Finish()

	config := s.newConfig(c)

	err := config.validate()
	c.Check(err, tc.ErrorIs, errors.NotValid)
	c.Check(err, tc.ErrorMatches, "nil statePool not valid")
}

func (s *sharedServerContextSuite) TestConfigNoLeaseManager(c *tc.C) {
	defer s.setupMocks(c).Finish()

	config := s.newConfig(c)

	config.leaseManager = nil
	err := config.validate()
	c.Check(err, tc.ErrorIs, errors.NotValid)
	c.Check(err, tc.ErrorMatches, "nil leaseManager not valid")
}

func (s *sharedServerContextSuite) TestConfigNoControllerConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	config := s.newConfig(c)

	config.controllerConfig = nil
	err := config.validate()
	c.Check(err, tc.ErrorIs, errors.NotValid)
	c.Check(err, tc.ErrorMatches, "nil controllerConfig not valid")
}

func (s *sharedServerContextSuite) TestNewCallsConfigValidate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	config := s.newConfig(c)

	ctx, err := newSharedServerContext(config)
	c.Check(err, tc.ErrorIs, errors.NotValid)
	c.Check(err, tc.ErrorMatches, "nil statePool not valid")
	c.Check(ctx, tc.IsNil)
}

func (s *sharedServerContextSuite) TestValidConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	config := s.newConfig(c)

	ctx, err := newSharedServerContext(config)
	c.Assert(err, tc.ErrorIsNil)
	// Normally you wouldn't directly access features.
	c.Assert(ctx.features, tc.HasLen, 0)
}

func (s *sharedServerContextSuite) newConfig(c *tc.C) sharedServerConfig {
	controllerConfig := testing.FakeControllerConfig()

	return sharedServerConfig{
		leaseManager:            &lease.Manager{},
		controllerConfig:        controllerConfig,
		controllerConfigService: s.controllerConfigService,
		logger:                  loggertesting.WrapCheckLog(c),
		dbGetter:                StubDBGetter{},
		dbDeleter:               StubDBDeleter{},
		domainServicesGetter:    &StubDomainServicesGetter{},
		tracerGetter:            &StubTracerGetter{},
		objectStoreGetter:       &StubObjectStoreGetter{},
		machineTag:              names.NewMachineTag("0"),
		dataDir:                 c.MkDir(),
		logDir:                  c.MkDir(),
		controllerUUID:          testing.ControllerTag.Id(),
		controllerModelUUID:     model.UUID(testing.ModelTag.Id()),
	}
}

func (s *sharedServerContextSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.controllerConfigService = NewMockControllerConfigService(ctrl)
	return ctrl
}
