// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger_test

import (
	"github.com/juju/clock"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/agent/logger"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/watcher/registry"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

type loggerSuite struct {
	statetesting.StateSuite

	// These are raw State objects. Use them for setup and assertions, but
	// should never be touched by the API calls themselves
	rawMachine      *state.Machine
	logger          *logger.LoggerAPI
	watcherRegistry facade.WatcherRegistry
	authorizer      apiservertesting.FakeAuthorizer
}

var _ = gc.Suite(&loggerSuite{})

func (s *loggerSuite) SetUpTest(c *gc.C) {
	s.StateSuite.SetUpTest(c)

	var err error
	s.watcherRegistry, err = registry.NewRegistry(clock.WallClock)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) { workertest.DirtyKill(c, s.watcherRegistry) })

	// Create a machine to work with
	s.rawMachine, err = s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	// The default auth is as the machine agent
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: s.rawMachine.Tag(),
	}

	s.logger, err = s.makeLoggerAPI(s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *loggerSuite) makeLoggerAPI(auth facade.Authorizer) (*logger.LoggerAPI, error) {
	ctx := facadetest.Context{
		Auth_:            auth,
		WatcherRegistry_: s.watcherRegistry,
		State_:           s.State,
	}
	return logger.NewLoggerAPI(ctx)
}

func (s *loggerSuite) TestNewLoggerAPIRefusesNonAgent(c *gc.C) {
	// We aren't even a machine agent
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = names.NewUserTag("some-user")
	endPoint, err := s.makeLoggerAPI(anAuthorizer)
	c.Assert(endPoint, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *loggerSuite) TestNewLoggerAPIAcceptsUnitAgent(c *gc.C) {
	// We aren't even a machine agent
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = names.NewUnitTag("germany/7")
	endPoint, err := s.makeLoggerAPI(anAuthorizer)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(endPoint, gc.NotNil)
}

func (s *loggerSuite) TestNewLoggerAPIAcceptsApplicationAgent(c *gc.C) {
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = names.NewApplicationTag("germany")
	endPoint, err := s.makeLoggerAPI(anAuthorizer)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(endPoint, gc.NotNil)
}

func (s *loggerSuite) TestWatchLoggingConfigNothing(c *gc.C) {
	// Not an error to watch nothing
	results := s.logger.WatchLoggingConfig(params.Entities{})
	c.Assert(results.Results, gc.HasLen, 0)
}

func (s *loggerSuite) setLoggingConfig(c *gc.C, loggingConfig string) {
	attr := map[string]interface{}{
		"logging-config": loggingConfig,
	}
	err := s.Model.UpdateModelConfig(attr, nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *loggerSuite) TestWatchLoggingConfig(c *gc.C) {
	args := params.Entities{
		Entities: []params.Entity{{Tag: s.rawMachine.Tag().String()}},
	}
	results := s.logger.WatchLoggingConfig(args)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].NotifyWatcherId, gc.Not(gc.Equals), "")
	c.Assert(results.Results[0].Error, gc.IsNil)
	resource, err := s.watcherRegistry.Get(results.Results[0].NotifyWatcherId)
	c.Assert(resource, gc.NotNil)
	c.Assert(err, jc.ErrorIsNil)

	_, ok := resource.(state.NotifyWatcher)
	c.Assert(ok, jc.IsTrue)
	// The watcher implementation is tested in the cache package.
}

func (s *loggerSuite) TestWatchLoggingConfigRefusesWrongAgent(c *gc.C) {
	// We are a machine agent, but not the one we are trying to track
	args := params.Entities{
		Entities: []params.Entity{{Tag: "machine-12354"}},
	}
	results := s.logger.WatchLoggingConfig(args)
	// It is not an error to make the request, but the specific item is rejected
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].NotifyWatcherId, gc.Equals, "")
	c.Assert(results.Results[0].Error, gc.DeepEquals, apiservertesting.ErrUnauthorized)
}

func (s *loggerSuite) TestLoggingConfigForNoone(c *gc.C) {
	// Not an error to request nothing, dumb, but not an error.
	results := s.logger.LoggingConfig(params.Entities{})
	c.Assert(results.Results, gc.HasLen, 0)
}

func (s *loggerSuite) TestLoggingConfigRefusesWrongAgent(c *gc.C) {
	args := params.Entities{
		Entities: []params.Entity{{Tag: "machine-12354"}},
	}
	results := s.logger.LoggingConfig(args)
	c.Assert(results.Results, gc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error, gc.DeepEquals, apiservertesting.ErrUnauthorized)
}

func (s *loggerSuite) TestLoggingConfigForAgent(c *gc.C) {
	newLoggingConfig := "<root>=WARN;juju.log.test=DEBUG;unit=INFO"
	s.setLoggingConfig(c, newLoggingConfig)

	args := params.Entities{
		Entities: []params.Entity{{Tag: s.rawMachine.Tag().String()}},
	}
	results := s.logger.LoggingConfig(args)
	c.Assert(results.Results, gc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error, gc.IsNil)
	c.Assert(result.Result, gc.Equals, newLoggingConfig)
}
