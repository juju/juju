// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/pubsub/v2"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	corecontroller "github.com/juju/juju/controller"
	"github.com/juju/juju/core/model"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/pubsub/controller"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/lease"
	statetesting "github.com/juju/juju/state/testing"
)

type sharedServerContextSuite struct {
	statetesting.StateSuite

	hub    *pubsub.StructuredHub
	config sharedServerConfig
}

var _ = gc.Suite(&sharedServerContextSuite{})

func (s *sharedServerContextSuite) SetUpTest(c *gc.C) {
	s.StateSuite.SetUpTest(c)

	s.hub = pubsub.NewStructuredHub(nil)

	controllerConfig := testing.FakeControllerConfig()

	s.config = sharedServerConfig{
		statePool:            s.StatePool,
		centralHub:           s.hub,
		leaseManager:         &lease.Manager{},
		controllerConfig:     controllerConfig,
		logger:               loggertesting.WrapCheckLog(c),
		dbGetter:             StubDBGetter{},
		dbDeleter:            StubDBDeleter{},
		domainServicesGetter: &StubDomainServicesGetter{},
		tracerGetter:         &StubTracerGetter{},
		objectStoreGetter:    &StubObjectStoreGetter{},
		machineTag:           names.NewMachineTag("0"),
		dataDir:              c.MkDir(),
		logDir:               c.MkDir(),
		controllerUUID:       testing.ControllerTag.Id(),
		controllerModelUUID:  model.UUID(testing.ModelTag.Id()),
	}
}

func (s *sharedServerContextSuite) TestConfigNoStatePool(c *gc.C) {
	s.config.statePool = nil
	err := s.config.validate()
	c.Check(err, jc.ErrorIs, errors.NotValid)
	c.Check(err, gc.ErrorMatches, "nil statePool not valid")
}

func (s *sharedServerContextSuite) TestConfigNoHub(c *gc.C) {
	s.config.centralHub = nil
	err := s.config.validate()
	c.Check(err, jc.ErrorIs, errors.NotValid)
	c.Check(err, gc.ErrorMatches, "nil centralHub not valid")
}

func (s *sharedServerContextSuite) TestConfigNoLeaseManager(c *gc.C) {
	s.config.leaseManager = nil
	err := s.config.validate()
	c.Check(err, jc.ErrorIs, errors.NotValid)
	c.Check(err, gc.ErrorMatches, "nil leaseManager not valid")
}

func (s *sharedServerContextSuite) TestConfigNoControllerConfig(c *gc.C) {
	s.config.controllerConfig = nil
	err := s.config.validate()
	c.Check(err, jc.ErrorIs, errors.NotValid)
	c.Check(err, gc.ErrorMatches, "nil controllerConfig not valid")
}

func (s *sharedServerContextSuite) TestNewCallsConfigValidate(c *gc.C) {
	s.config.statePool = nil
	ctx, err := newSharedServerContext(s.config)
	c.Check(err, jc.ErrorIs, errors.NotValid)
	c.Check(err, gc.ErrorMatches, "nil statePool not valid")
	c.Check(ctx, gc.IsNil)
}

func (s *sharedServerContextSuite) TestValidConfig(c *gc.C) {
	ctx, err := newSharedServerContext(s.config)
	c.Assert(err, jc.ErrorIsNil)
	// Normally you wouldn't directly access features.
	c.Assert(ctx.features, gc.HasLen, 0)
}

func (s *sharedServerContextSuite) newContext(c *gc.C) *sharedServerContext {
	ctx, err := newSharedServerContext(s.config)
	c.Assert(err, jc.ErrorIsNil)
	return ctx
}

type stubHub struct {
	*pubsub.StructuredHub

	published []string
}

func (s *stubHub) Publish(topic string, data interface{}) (func(), error) {
	s.published = append(s.published, topic)
	return func() {}, nil
}

func (s *sharedServerContextSuite) TestControllerConfigChanged(c *gc.C) {
	stub := &stubHub{StructuredHub: s.hub}
	s.config.centralHub = stub
	ctx := s.newContext(c)

	msg := controller.ConfigChangedMessage{
		Config: corecontroller.Config{
			corecontroller.Features: "foo,bar",
		},
	}

	done, err := s.hub.Publish(controller.ConfigChanged, msg)
	c.Assert(err, jc.ErrorIsNil)

	select {
	case <-pubsub.Wait(done):
	case <-time.After(testing.LongWait):
		c.Fatalf("handler didn't")
	}

	c.Check(ctx.featureEnabled("foo"), jc.IsTrue)
	c.Check(ctx.featureEnabled("bar"), jc.IsTrue)
	c.Check(ctx.featureEnabled("baz"), jc.IsFalse)
	c.Check(stub.published, gc.HasLen, 0)
}
