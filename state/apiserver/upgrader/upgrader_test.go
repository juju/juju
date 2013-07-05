// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader_test

import (
	. "launchpad.net/gocheck"

	"launchpad.net/juju-core/errors"
	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
	apitesting "launchpad.net/juju-core/state/apiserver/testing"
	"launchpad.net/juju-core/state/apiserver/upgrader"
	statetesting "launchpad.net/juju-core/state/testing"
	"launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/version"
)

type upgraderSuite struct {
	jujutesting.JujuConnSuite

	// These are raw State objects. Use them for setup and assertions, but
	// should never be touched by the API calls themselves
	rawMachine *state.Machine
	upgrader   *upgrader.UpgraderAPI
	resources  *common.Resources
	authorizer apitesting.FakeAuthorizer
}

var _ = Suite(&upgraderSuite{})

func (s *upgraderSuite) SetUpTest(c *C) {
	s.JujuConnSuite.SetUpTest(c)
	s.resources = common.NewResources()

	// Create a machine to work with
	var err error
	s.rawMachine, err = s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	err = s.rawMachine.SetPassword("test-password")
	c.Assert(err, IsNil)

	// The default auth is as the machine agent
	s.authorizer = apitesting.FakeAuthorizer{
		Tag:          s.rawMachine.Tag(),
		LoggedIn:     true,
		Manager:      false,
		MachineAgent: true,
		Client:       false,
	}
	s.upgrader, err = upgrader.NewUpgraderAPI(s.State, s.resources, s.authorizer)
	c.Assert(err, IsNil)
}

func (s *upgraderSuite) TearDownTest(c *C) {
	if s.resources != nil {
		s.resources.StopAll()
	}
	s.JujuConnSuite.TearDownTest(c)
}

func (s *upgraderSuite) TestWatchAPIVersionNothing(c *C) {
	// Not an error to watch nothing
	results, err := s.upgrader.WatchAPIVersion(params.Agents{})
	c.Assert(err, IsNil)
	c.Check(results.Results, HasLen, 0)
}

func (s *upgraderSuite) TestWatchAPIVersion(c *C) {
	args := params.Agents{
		Agents: []params.Agent{params.Agent{Tag: s.rawMachine.Tag()}},
	}
	results, err := s.upgrader.WatchAPIVersion(args)
	c.Assert(err, IsNil)
	c.Check(results.Results, HasLen, 1)
	c.Check(results.Results[0].NotifyWatcherId, Not(Equals), "")
	resource := s.resources.Get(results.Results[0].NotifyWatcherId)
	c.Check(resource, NotNil)

	w := resource.(state.NotifyWatcher)
	wc := statetesting.NewNotifyWatcherC(c, s.State, w)
	// The initial change is consumed in the WatchAPIVersion call
	wc.AssertNoChange()
	// Setting the AgentVersion without changing it doesn't trigger an update
	ver := version.Current.Number
	err = statetesting.SetAgentVersion(s.State, ver)
	c.Assert(err, IsNil)
	wc.AssertNoChange()
	ver.Minor += 1
	err = statetesting.SetAgentVersion(s.State, ver)
	c.Assert(err, IsNil)
	wc.AssertOneChange()
	statetesting.AssertStop(c, w)
	wc.AssertClosed()
	c.Errorf("Just failing it all")
}

func (s *upgraderSuite) TestUpgraderAPIRefusesNonAgent(c *C) {
	// We aren't even a machine agent
	anAuthorizer := s.authorizer
	anAuthorizer.MachineAgent = false
	anUpgrader, err := upgrader.NewUpgraderAPI(s.State, s.resources, anAuthorizer)
	c.Check(err, NotNil)
	c.Check(anUpgrader, IsNil)
	c.Assert(err, ErrorMatches, "permission denied")
}

func (s *upgraderSuite) TestWatchAPIVersionRefusesWrongAgent(c *C) {
	// We are a machine agent, but not the one we are trying to track
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = "machine-12354"
	anUpgrader, err := upgrader.NewUpgraderAPI(s.State, s.resources, anAuthorizer)
	c.Check(err, IsNil)
	args := params.Agents{
		Agents: []params.Agent{params.Agent{Tag: s.rawMachine.Tag()}},
	}
	results, err := anUpgrader.WatchAPIVersion(args)
	// It is not an error to make the request, but the specific item is rejected
	c.Assert(err, IsNil)
	c.Check(results.Results, HasLen, 1)
	c.Check(results.Results[0].NotifyWatcherId, Equals, "")
	c.Assert(results.Results[0].Error, NotNil)
	err = results.Results[0].Error
	c.Check(err, ErrorMatches, "permission denied")
}

func (s *upgraderSuite) TestToolsNothing(c *C) {
	// Not an error to watch nothing
	results, err := s.upgrader.Tools(params.Agents{})
	c.Assert(err, IsNil)
	c.Check(results.Tools, HasLen, 0)
}

func (s *upgraderSuite) TestToolsRefusesWrongAgent(c *C) {
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = "machine-12354"
	anUpgrader, err := upgrader.NewUpgraderAPI(s.State, s.resources, anAuthorizer)
	c.Check(err, IsNil)
	args := params.Agents{
		Agents: []params.Agent{params.Agent{Tag: s.rawMachine.Tag()}},
	}
	results, err := anUpgrader.Tools(args)
	// It is not an error to make the request, but the specific item is rejected
	c.Assert(err, IsNil)
	c.Check(results.Tools, HasLen, 1)
	toolResult := results.Tools[0]
	c.Check(toolResult.AgentTools.Tag, Equals, s.rawMachine.Tag())
	c.Assert(toolResult.Error, NotNil)
	err = toolResult.Error
	c.Check(err, ErrorMatches, "permission denied")
}

func (s *upgraderSuite) TestToolsForAgent(c *C) {
	cur := version.Current
	agent := params.Agent{
		Tag: s.rawMachine.Tag(),
	}

	// The machine must have its existing tools set before we query for the
	// next tools. This is so that we can grab Arch and Series without
	// having to pass it in again
	err := s.rawMachine.SetAgentTools(&state.Tools{
		URL:    "",
		Binary: version.Current,
	})
	c.Assert(err, IsNil)

	args := params.Agents{Agents: []params.Agent{agent}}
	results, err := s.upgrader.Tools(args)
	c.Assert(err, IsNil)
	c.Check(results.Tools, HasLen, 1)
	agentTools := results.Tools[0].AgentTools
	c.Assert(results.Tools[0].Error, IsNil)
	c.Check(agentTools.Tag, Equals, s.rawMachine.Tag())
	c.Check(agentTools.Major, Equals, cur.Major)
	c.Check(agentTools.Minor, Equals, cur.Minor)
	c.Check(agentTools.Patch, Equals, cur.Patch)
	c.Check(agentTools.Build, Equals, cur.Build)
	c.Check(agentTools.Arch, Equals, cur.Arch)
	c.Check(agentTools.Series, Equals, cur.Series)
	c.Check(agentTools.URL, Not(Equals), "")
}

func (s *upgraderSuite) TestSetToolsNothing(c *C) {
	// Not an error to watch nothing
	results, err := s.upgrader.SetTools(params.SetAgentTools{})
	c.Assert(err, IsNil)
	c.Check(results.Results, HasLen, 0)
}

func (s *upgraderSuite) TestSetToolsRefusesWrongAgent(c *C) {
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = "machine-12354"
	anUpgrader, err := upgrader.NewUpgraderAPI(s.State, s.resources, anAuthorizer)
	c.Check(err, IsNil)
	cur := version.Current
	tools := params.AgentTools{
		Tag:    s.rawMachine.Tag(),
		Arch:   cur.Arch,
		Series: cur.Series,
		Major:  cur.Major,
		Minor:  cur.Minor,
		Patch:  cur.Patch,
		Build:  cur.Build,
		URL:    "",
	}
	args := params.SetAgentTools{AgentTools: []params.AgentTools{tools}}
	results, err := anUpgrader.SetTools(args)
	c.Assert(results.Results, HasLen, 1)
	c.Assert(results.Results[0].Tag, Equals, s.rawMachine.Tag())
	c.Assert(results.Results[0].Error, NotNil)
	err = results.Results[0].Error
	c.Assert(err, ErrorMatches, "permission denied")
}

func (s *upgraderSuite) TestSetTools(c *C) {
	cur := version.Current
	_, err := s.rawMachine.AgentTools()
	c.Assert(err, checkers.Satisfies, errors.IsNotFoundError)
	tools := params.AgentTools{
		Tag:    s.rawMachine.Tag(),
		Arch:   cur.Arch,
		Series: cur.Series,
		Major:  cur.Major,
		Minor:  cur.Minor,
		Patch:  cur.Patch,
		Build:  cur.Build,
		URL:    "",
	}
	args := params.SetAgentTools{AgentTools: []params.AgentTools{tools}}
	results, err := s.upgrader.SetTools(args)
	c.Assert(err, IsNil)
	c.Assert(results.Results, HasLen, 1)
	c.Assert(results.Results[0].Tag, Equals, s.rawMachine.Tag())
	c.Assert(results.Results[0].Error, IsNil)
	// Check that the new value actually got set, we must Refresh because
	// it was set on a different Machine object
	err = s.rawMachine.Refresh()
	c.Assert(err, IsNil)
	realTools, err := s.rawMachine.AgentTools()
	c.Assert(err, IsNil)
	c.Check(realTools.Arch, Equals, cur.Arch)
	c.Check(realTools.Series, Equals, cur.Series)
	c.Check(realTools.Major, Equals, cur.Major)
	c.Check(realTools.Minor, Equals, cur.Minor)
	c.Check(realTools.Patch, Equals, cur.Patch)
	c.Check(realTools.Build, Equals, cur.Build)
	c.Check(realTools.URL, Equals, "")
}
