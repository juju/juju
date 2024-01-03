// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"
	"github.com/juju/pubsub/v2"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/workertest"
	"github.com/prometheus/client_golang/prometheus"
	gc "gopkg.in/check.v1"

	corecontroller "github.com/juju/juju/controller"
	"github.com/juju/juju/core/presence"
	"github.com/juju/juju/internal/pubsub/controller"
	"github.com/juju/juju/internal/worker/lease"
	"github.com/juju/juju/internal/worker/multiwatcher"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/testing"
)

type sharedServerContextSuite struct {
	statetesting.StateSuite

	hub    *pubsub.StructuredHub
	config sharedServerConfig
}

var _ = gc.Suite(&sharedServerContextSuite{})

func (s *sharedServerContextSuite) SetUpTest(c *gc.C) {
	s.StateSuite.SetUpTest(c)

	allWatcherBacking, err := state.NewAllWatcherBacking(s.StatePool)
	c.Assert(err, jc.ErrorIsNil)
	multiWatcherWorker, err := multiwatcher.NewWorker(multiwatcher.Config{
		Clock:                clock.WallClock,
		Logger:               loggo.GetLogger("test"),
		Backing:              allWatcherBacking,
		PrometheusRegisterer: noopRegisterer{},
	})
	c.Assert(err, jc.ErrorIsNil)
	// The worker itself is a coremultiwatcher.Factory.
	s.AddCleanup(func(c *gc.C) { workertest.CleanKill(c, multiWatcherWorker) })

	s.hub = pubsub.NewStructuredHub(nil)

	controllerConfig := testing.FakeControllerConfig()

	s.config = sharedServerConfig{
		statePool:            s.StatePool,
		multiwatcherFactory:  multiWatcherWorker,
		centralHub:           s.hub,
		presence:             presence.New(clock.WallClock),
		leaseManager:         &lease.Manager{},
		controllerConfig:     controllerConfig,
		logger:               loggo.GetLogger("test"),
		dbGetter:             StubDBGetter{},
		serviceFactoryGetter: &StubServiceFactoryGetter{},
		tracerGetter:         &StubTracerGetter{},
		objectStoreGetter:    &StubObjectStoreGetter{},
		machineTag:           names.NewMachineTag("0"),
		dataDir:              c.MkDir(),
		logDir:               c.MkDir(),
	}
}

func (s *sharedServerContextSuite) TestConfigNoStatePool(c *gc.C) {
	s.config.statePool = nil
	err := s.config.validate()
	c.Check(err, jc.ErrorIs, errors.NotValid)
	c.Check(err, gc.ErrorMatches, "nil statePool not valid")
}

func (s *sharedServerContextSuite) TestConfigNoMultiwatcherFactory(c *gc.C) {
	s.config.multiwatcherFactory = nil
	err := s.config.validate()
	c.Check(err, jc.ErrorIs, errors.NotValid)
	c.Check(err, gc.ErrorMatches, "nil multiwatcherFactory not valid")
}

func (s *sharedServerContextSuite) TestConfigNoHub(c *gc.C) {
	s.config.centralHub = nil
	err := s.config.validate()
	c.Check(err, jc.ErrorIs, errors.NotValid)
	c.Check(err, gc.ErrorMatches, "nil centralHub not valid")
}

func (s *sharedServerContextSuite) TestConfigNoPresence(c *gc.C) {
	s.config.presence = nil
	err := s.config.validate()
	c.Check(err, jc.ErrorIs, errors.NotValid)
	c.Check(err, gc.ErrorMatches, "nil presence not valid")
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
	ctx.Close()
}

func (s *sharedServerContextSuite) newContext(c *gc.C) *sharedServerContext {
	ctx, err := newSharedServerContext(s.config)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(*gc.C) { ctx.Close() })
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

type noopRegisterer struct {
	prometheus.Registerer
}

func (noopRegisterer) Register(prometheus.Collector) error {
	return nil
}

func (noopRegisterer) Unregister(prometheus.Collector) bool {
	return true
}
