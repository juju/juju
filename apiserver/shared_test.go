// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/pubsub/v2"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/model"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/lease"
	statetesting "github.com/juju/juju/state/testing"
)

type sharedServerContextSuite struct {
	statetesting.StateSuite

	hub                     *pubsub.StructuredHub
	controllerConfigService ControllerConfigService
}

var _ = gc.Suite(&sharedServerContextSuite{})

func (s *sharedServerContextSuite) TestConfigNoStatePool(c *gc.C) {
	defer s.setupMocks(c).Finish()

	config := s.newConfig(c)

	config.statePool = nil
	err := config.validate()
	c.Check(err, jc.ErrorIs, errors.NotValid)
	c.Check(err, gc.ErrorMatches, "nil statePool not valid")
}

func (s *sharedServerContextSuite) TestConfigNoHub(c *gc.C) {
	defer s.setupMocks(c).Finish()

	config := s.newConfig(c)

	config.centralHub = nil
	err := config.validate()
	c.Check(err, jc.ErrorIs, errors.NotValid)
	c.Check(err, gc.ErrorMatches, "nil centralHub not valid")
}

func (s *sharedServerContextSuite) TestConfigNoLeaseManager(c *gc.C) {
	defer s.setupMocks(c).Finish()

	config := s.newConfig(c)

	config.leaseManager = nil
	err := config.validate()
	c.Check(err, jc.ErrorIs, errors.NotValid)
	c.Check(err, gc.ErrorMatches, "nil leaseManager not valid")
}

func (s *sharedServerContextSuite) TestConfigNoControllerConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	config := s.newConfig(c)

	config.controllerConfig = nil
	err := config.validate()
	c.Check(err, jc.ErrorIs, errors.NotValid)
	c.Check(err, gc.ErrorMatches, "nil controllerConfig not valid")
}

func (s *sharedServerContextSuite) TestNewCallsConfigValidate(c *gc.C) {
	defer s.setupMocks(c).Finish()

	config := s.newConfig(c)

	config.statePool = nil
	ctx, err := newSharedServerContext(config)
	c.Check(err, jc.ErrorIs, errors.NotValid)
	c.Check(err, gc.ErrorMatches, "nil statePool not valid")
	c.Check(ctx, gc.IsNil)
}

func (s *sharedServerContextSuite) TestValidConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	config := s.newConfig(c)

	ctx, err := newSharedServerContext(config)
	c.Assert(err, jc.ErrorIsNil)
	// Normally you wouldn't directly access features.
	c.Assert(ctx.features, gc.HasLen, 0)
}

func (s *sharedServerContextSuite) newConfig(c *gc.C) sharedServerConfig {
	s.hub = pubsub.NewStructuredHub(nil)

	controllerConfig := testing.FakeControllerConfig()

	return sharedServerConfig{
		statePool:               s.StatePool,
		centralHub:              s.hub,
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

func (s *sharedServerContextSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.controllerConfigService = NewMockControllerConfigService(ctrl)
	return ctrl
}
