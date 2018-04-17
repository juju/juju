// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/pubsub"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/clock"
	gc "gopkg.in/check.v1"

	corecontroller "github.com/juju/juju/controller"
	"github.com/juju/juju/core/presence"
	"github.com/juju/juju/pubsub/controller"
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

	s.hub = pubsub.NewStructuredHub(nil)
	s.config = sharedServerConfig{
		statePool:  s.StatePool,
		centralHub: s.hub,
		presence:   presence.New(clock.WallClock),
		logger:     loggo.GetLogger("test"),
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

func (s *sharedServerContextSuite) TestControllerConfigChanged(c *gc.C) {
	ctx := s.newContext(c)

	msg := controller.ConfigChangedMessage{
		corecontroller.Config{
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
}
