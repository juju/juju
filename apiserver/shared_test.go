// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/pubsub"
	jc "github.com/juju/testing/checkers"
	"github.com/prometheus/client_golang/prometheus"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/worker.v1/workertest"

	corecontroller "github.com/juju/juju/controller"
	"github.com/juju/juju/core/cache"
	"github.com/juju/juju/core/presence"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/pubsub/controller"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/lease"
	"github.com/juju/juju/worker/modelcache"
)

type sharedServerContextSuite struct {
	statetesting.StateSuite

	hub    *pubsub.StructuredHub
	config sharedServerConfig
}

var _ = gc.Suite(&sharedServerContextSuite{})

func (s *sharedServerContextSuite) SetUpTest(c *gc.C) {
	s.StateSuite.SetUpTest(c)

	modelCache, err := modelcache.NewWorker(modelcache.Config{
		Logger:               loggo.GetLogger("test"),
		StatePool:            s.StatePool,
		PrometheusRegisterer: noopRegisterer{},
		Cleanup:              func() {},
	})
	s.AddCleanup(func(c *gc.C) { workertest.CleanKill(c, modelCache) })
	c.Assert(err, jc.ErrorIsNil)
	var controller *cache.Controller
	err = modelcache.ExtractCacheController(modelCache, &controller)
	c.Assert(err, jc.ErrorIsNil)

	s.hub = pubsub.NewStructuredHub(nil)
	s.config = sharedServerConfig{
		statePool:    s.StatePool,
		controller:   controller,
		centralHub:   s.hub,
		presence:     presence.New(clock.WallClock),
		leaseManager: &lease.Manager{},
		logger:       loggo.GetLogger("test"),
	}
}

func (s *sharedServerContextSuite) TestConfigNoStatePool(c *gc.C) {
	s.config.statePool = nil
	err := s.config.validate()
	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, "nil statePool not valid")
}

func (s *sharedServerContextSuite) TestConfigNoHub(c *gc.C) {
	s.config.centralHub = nil
	err := s.config.validate()
	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, "nil centralHub not valid")
}

func (s *sharedServerContextSuite) TestConfigNoPresence(c *gc.C) {
	s.config.presence = nil
	err := s.config.validate()
	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, "nil presence not valid")
}

func (s *sharedServerContextSuite) TestConfigNoLeaseManager(c *gc.C) {
	s.config.leaseManager = nil
	err := s.config.validate()
	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, "nil leaseManager not valid")
}

func (s *sharedServerContextSuite) TestNewCallsConfigValidate(c *gc.C) {
	s.config.statePool = nil
	ctx, err := newSharedServerContex(s.config)
	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, "nil statePool not valid")
	c.Check(ctx, gc.IsNil)
}

func (s *sharedServerContextSuite) TestValidConfig(c *gc.C) {
	ctx, err := newSharedServerContex(s.config)
	c.Assert(err, jc.ErrorIsNil)
	// Normally you wouldn't directly access features.
	c.Assert(ctx.features, gc.HasLen, 0)
	ctx.Close()
}

func (s *sharedServerContextSuite) newContext(c *gc.C) *sharedServerContext {
	ctx, err := newSharedServerContex(s.config)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(*gc.C) { ctx.Close() })
	return ctx
}

type stubHub struct {
	*pubsub.StructuredHub

	published []string
}

func (s *stubHub) Publish(topic string, data interface{}) (<-chan struct{}, error) {
	s.published = append(s.published, topic)
	return nil, nil
}

func (s *sharedServerContextSuite) TestControllerConfigChanged(c *gc.C) {
	stub := &stubHub{StructuredHub: s.hub}
	s.config.centralHub = stub
	ctx := s.newContext(c)

	msg := controller.ConfigChangedMessage{
		Config: corecontroller.Config{
			corecontroller.Features: []string{"foo", "bar"},
		},
	}

	done, err := s.hub.Publish(controller.ConfigChanged, msg)
	c.Assert(err, jc.ErrorIsNil)

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatalf("handler didn't")
	}

	c.Check(ctx.featureEnabled("foo"), jc.IsTrue)
	c.Check(ctx.featureEnabled("bar"), jc.IsTrue)
	c.Check(ctx.featureEnabled("baz"), jc.IsFalse)
	c.Check(stub.published, gc.HasLen, 0)
}

func (s *sharedServerContextSuite) TestAddingOldPresenceFeature(c *gc.C) {
	// Adding the feature.OldPresence to the feature list will cause
	// a message to be published on the hub to request an apiserver restart.
	stub := &stubHub{StructuredHub: s.hub}
	s.config.centralHub = stub
	s.newContext(c)

	msg := controller.ConfigChangedMessage{
		Config: corecontroller.Config{
			corecontroller.Features: []string{"foo", "bar", feature.OldPresence},
		},
	}
	done, err := s.hub.Publish(controller.ConfigChanged, msg)
	c.Assert(err, jc.ErrorIsNil)

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatalf("handler didn't")
	}

	c.Check(stub.published, jc.DeepEquals, []string{"apiserver.restart"})
}

func (s *sharedServerContextSuite) TestRemovingOldPresenceFeature(c *gc.C) {
	err := s.State.UpdateControllerConfig(map[string]interface{}{
		"features": []string{feature.OldPresence},
	}, nil)
	c.Assert(err, jc.ErrorIsNil)
	// Removing the feature.OldPresence to the feature list will cause
	// a message to be published on the hub to request an apiserver restart.
	stub := &stubHub{StructuredHub: s.hub}
	s.config.centralHub = stub
	s.newContext(c)

	msg := controller.ConfigChangedMessage{
		Config: corecontroller.Config{
			corecontroller.Features: []string{"foo", "bar"},
		},
	}
	done, err := s.hub.Publish(controller.ConfigChanged, msg)
	c.Assert(err, jc.ErrorIsNil)

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatalf("handler didn't")
	}

	c.Check(stub.published, jc.DeepEquals, []string{"apiserver.restart"})
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
