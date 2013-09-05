// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger_test

import (
	gc "launchpad.net/gocheck"

	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
	"launchpad.net/juju-core/state/apiserver/logger"
	apiservertesting "launchpad.net/juju-core/state/apiserver/testing"
	statetesting "launchpad.net/juju-core/state/testing"
)

type loggerSuite struct {
	jujutesting.JujuConnSuite

	// These are raw State objects. Use them for setup and assertions, but
	// should never be touched by the API calls themselves
	rawMachine *state.Machine
	logger     logger.LoggerAPI
	resources  *common.Resources
	authorizer apiservertesting.FakeAuthorizer
}

var _ = gc.Suite(&loggerSuite{})

func (s *loggerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.resources = common.NewResources()

	// Create a machine to work with
	var err error
	s.rawMachine, err = s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	err = s.rawMachine.SetPassword("test-password")
	c.Assert(err, gc.IsNil)

	// The default auth is as the machine agent
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag:          s.rawMachine.Tag(),
		LoggedIn:     true,
		Manager:      false,
		MachineAgent: true,
		Client:       false,
	}
	s.logger, err = logger.NewLoggerAPI(s.State, s.resources, s.authorizer)
	c.Assert(err, gc.IsNil)
}

func (s *loggerSuite) TearDownTest(c *gc.C) {
	s.resources.StopAll()
	s.JujuConnSuite.TearDownTest(c)
}

func (s *loggerSuite) TestNewLoggerAPIRefusesNonAgent(c *gc.C) {
	// We aren't even a machine agent
	anAuthorizer := s.authorizer
	anAuthorizer.UnitAgent = false
	anAuthorizer.MachineAgent = false
	endPoint, err := logger.NewLoggerAPI(s.State, s.resources, anAuthorizer)
	c.Assert(endPoint, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *loggerSuite) TestNewLoggerAPIAcceptsUnitAgent(c *gc.C) {
	// We aren't even a machine agent
	anAuthorizer := s.authorizer
	anAuthorizer.UnitAgent = true
	anAuthorizer.MachineAgent = false
	endPoint, err := logger.NewLoggerAPI(s.State, s.resources, anAuthorizer)
	c.Assert(err, gc.IsNil)
	c.Assert(endPoint, gc.NotNil)
}

func (s *loggerSuite) TestWatchLoggingConfigNothing(c *gc.C) {
	// Not an error to watch nothing
	results := s.logger.WatchLoggingConfig(params.Entities{})
	c.Assert(results.Results, gc.HasLen, 0)
}

func (s *loggerSuite) TestWatchLoggingConfig(c *gc.C) {
	args := params.Entities{
		Entities: []params.Entity{{Tag: s.rawMachine.Tag()}},
	}
	results := s.logger.WatchLoggingConfig(args)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].NotifyWatcherId, gc.Not(gc.Equals), "")
	c.Assert(results.Results[0].Error, gc.IsNil)
	resource := s.resources.Get(results.Results[0].NotifyWatcherId)
	c.Assert(resource, gc.NotNil)

	w := resource.(state.NotifyWatcher)
	wc := statetesting.NewNotifyWatcherC(c, s.State, w)
	wc.AssertNoChange()

	envConfig, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	loggingConfig := envConfig.LoggingConfig()
	newConfig, err := envConfig.Apply(
		map[string]interface{}{"logging-config": loggingConfig + ":juju.log.test=DEBUG"})
	c.Assert(err, gc.IsNil)
	err = s.State.SetEnvironConfig(newConfig)
	c.Assert(err, gc.IsNil)

	wc.AssertOneChange()
	statetesting.AssertStop(c, w)
	wc.AssertClosed()
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

// func (s *loggerSuite) TestToolsForAgent(c *gc.C) {
// 	cur := version.Current
// 	agent := params.Entity{Tag: s.rawMachine.Tag()}

// 	// The machine must have its existing tools set before we query for the
// 	// next tools. This is so that we can grab Arch and Series without
// 	// having to pass it in again
// 	err := s.rawMachine.SetAgentTools(&tools.Tools{
// 		URL:     "",
// 		Version: version.Current,
// 	})
// 	c.Assert(err, gc.IsNil)

// 	args := params.Entities{Entities: []params.Entity{agent}}
// 	results, err := s.logger.Tools(args)
// 	c.Assert(err, gc.IsNil)
// 	c.Assert(results.Results, gc.HasLen, 1)
// 	c.Assert(results.Results[0].Error, gc.IsNil)
// 	agentTools := results.Results[0].Tools
// 	c.Assert(agentTools.URL, gc.Not(gc.Equals), "")
// 	c.Assert(agentTools.Version, gc.DeepEquals, cur)
// }

// func (s *loggerSuite) TestSetToolsNothing(c *gc.C) {
// 	// Not an error to watch nothing
// 	results, err := s.logger.SetTools(params.SetAgentsTools{})
// 	c.Assert(err, gc.IsNil)
// 	c.Assert(results.Results, gc.HasLen, 0)
// }

// func (s *loggerSuite) TestSetToolsRefusesWrongAgent(c *gc.C) {
// 	anAuthorizer := s.authorizer
// 	anAuthorizer.Tag = "machine-12354"
// 	anLogger, err := logger.NewLoggerAPI(s.State, s.resources, anAuthorizer)
// 	c.Assert(err, gc.IsNil)
// 	args := params.SetAgentsTools{
// 		AgentTools: []params.SetAgentTools{{
// 			Tag: s.rawMachine.Tag(),
// 			Tools: &tools.Tools{
// 				Version: version.Current,
// 			},
// 		}},
// 	}

// 	results, err := anLogger.SetTools(args)
// 	c.Assert(results.Results, gc.HasLen, 1)
// 	c.Assert(results.Results[0].Error, gc.DeepEquals, apiservertesting.ErrUnauthorized)
// }

// func (s *loggerSuite) TestSetTools(c *gc.C) {
// 	cur := version.Current
// 	_, err := s.rawMachine.AgentTools()
// 	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)
// 	args := params.SetAgentsTools{
// 		AgentTools: []params.SetAgentTools{{
// 			Tag: s.rawMachine.Tag(),
// 			Tools: &tools.Tools{
// 				Version: cur,
// 			}},
// 		},
// 	}
// 	results, err := s.logger.SetTools(args)
// 	c.Assert(err, gc.IsNil)
// 	c.Assert(results.Results, gc.HasLen, 1)
// 	c.Assert(results.Results[0].Error, gc.IsNil)
// 	// Check that the new value actually got set, we must Refresh because
// 	// it was set on a different Machine object
// 	err = s.rawMachine.Refresh()
// 	c.Assert(err, gc.IsNil)
// 	realTools, err := s.rawMachine.AgentTools()
// 	c.Assert(err, gc.IsNil)
// 	c.Assert(realTools.Version.Arch, gc.Equals, cur.Arch)
// 	c.Assert(realTools.Version.Series, gc.Equals, cur.Series)
// 	c.Assert(realTools.Version.Major, gc.Equals, cur.Major)
// 	c.Assert(realTools.Version.Minor, gc.Equals, cur.Minor)
// 	c.Assert(realTools.Version.Patch, gc.Equals, cur.Patch)
// 	c.Assert(realTools.Version.Build, gc.Equals, cur.Build)
// 	c.Assert(realTools.URL, gc.Equals, "")
// }

// func (s *loggerSuite) TestDesiredVersionNothing(c *gc.C) {
// 	// Not an error to watch nothing
// 	results, err := s.logger.DesiredVersion(params.Entities{})
// 	c.Assert(err, gc.IsNil)
// 	c.Assert(results.Results, gc.HasLen, 0)
// }

// func (s *loggerSuite) TestDesiredVersionRefusesWrongAgent(c *gc.C) {
// 	anAuthorizer := s.authorizer
// 	anAuthorizer.Tag = "machine-12354"
// 	anLogger, err := logger.NewLoggerAPI(s.State, s.resources, anAuthorizer)
// 	c.Assert(err, gc.IsNil)
// 	args := params.Entities{
// 		Entities: []params.Entity{{Tag: s.rawMachine.Tag()}},
// 	}
// 	results, err := anLogger.DesiredVersion(args)
// 	// It is not an error to make the request, but the specific item is rejected
// 	c.Assert(err, gc.IsNil)
// 	c.Assert(results.Results, gc.HasLen, 1)
// 	toolResult := results.Results[0]
// 	c.Assert(toolResult.Error, gc.DeepEquals, apiservertesting.ErrUnauthorized)
// }

// func (s *loggerSuite) TestDesiredVersionNoticesMixedAgents(c *gc.C) {
// 	args := params.Entities{Entities: []params.Entity{
// 		{Tag: s.rawMachine.Tag()},
// 		{Tag: "machine-12345"},
// 	}}
// 	results, err := s.logger.DesiredVersion(args)
// 	c.Assert(err, gc.IsNil)
// 	c.Assert(results.Results, gc.HasLen, 2)
// 	c.Assert(results.Results[0].Error, gc.IsNil)
// 	agentVersion := results.Results[0].Version
// 	c.Assert(agentVersion, gc.NotNil)
// 	c.Assert(*agentVersion, gc.DeepEquals, version.Current.Number)

// 	c.Assert(results.Results[1].Error, gc.DeepEquals, apiservertesting.ErrUnauthorized)
// 	c.Assert(results.Results[1].Version, gc.IsNil)

// }

// func (s *loggerSuite) TestDesiredVersionForAgent(c *gc.C) {
// 	args := params.Entities{Entities: []params.Entity{{Tag: s.rawMachine.Tag()}}}
// 	results, err := s.logger.DesiredVersion(args)
// 	c.Assert(err, gc.IsNil)
// 	c.Assert(results.Results, gc.HasLen, 1)
// 	c.Assert(results.Results[0].Error, gc.IsNil)
// 	agentVersion := results.Results[0].Version
// 	c.Assert(agentVersion, gc.NotNil)
// 	c.Assert(*agentVersion, gc.DeepEquals, version.Current.Number)
// }
