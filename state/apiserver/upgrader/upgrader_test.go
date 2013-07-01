// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader_test

import (
	"time"

	. "launchpad.net/gocheck"

	"launchpad.net/juju-core/errors"
	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	apitesting "launchpad.net/juju-core/state/apiserver/testing"
	"launchpad.net/juju-core/state/apiserver/upgrader"
	"launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/version"
)

type upgraderSuite struct {
	jujutesting.JujuConnSuite

	// These are raw State objects. Use them for setup and assertions, but
	// should never be touched by the API calls themselves
	rawMachine *state.Machine
	upgrader   *upgrader.UpgraderAPI
	resources  apitesting.FakeResourceRegistry
	authorizer apitesting.FakeAuthorizer
}

var _ = Suite(&upgraderSuite{})

func (s *upgraderSuite) SetUpTest(c *C) {
	s.JujuConnSuite.SetUpTest(c)
	s.resources = make(apitesting.FakeResourceRegistry)

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
		for _, resource := range s.resources {
			resource.Stop()
		}
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
	c.Check(results.Results[0].EntityWatcherId, Not(Equals), "")
	resource, ok := s.resources[results.Results[0].EntityWatcherId]
	c.Check(ok, checkers.IsTrue)
	defer func() {
		err := resource.Stop()
		c.Assert(err, IsNil)
	}()
	// Check that the watcher returns an initial event
	channel := resource.(*state.EnvironConfigWatcher).Changes()
	// Should use helpers from state/watcher_test.go when generalised
	select {
	case _, ok := <-channel:
		c.Assert(ok, Equals, true)
	case <-time.After(50 * time.Millisecond):
		c.Fatal("timeout waiting for entity watcher")
	}
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
	c.Check(results.Results[0].EntityWatcherId, Equals, "")
	c.Assert(results.Results[0].Error, NotNil)
	err = *results.Results[0].Error
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
	c.Check(toolResult.Tag, Equals, s.rawMachine.Tag())
	c.Assert(toolResult.Error, NotNil)
	err = *toolResult.Error
	c.Check(err, ErrorMatches, "permission denied")
}

func (s *upgraderSuite) TestToolsForAgentNoArchOrSeries(c *C) {
	// You must pass the Arch and Series for the Tools request
	// The various ways you can pass something wrong
	permutations := []struct {
		Arch   string
		Series string
	}{
		{"", ""},
		{"", "value"},
		{"value", ""},
	}
	agents := make([]params.Agent, len(permutations))
	args := params.Agents{Agents: agents}
	for i, p := range permutations {
		agents[i].Tag = s.rawMachine.Tag()
		agents[i].Arch = p.Arch
		agents[i].Series = p.Series
	}
	results, err := s.upgrader.Tools(args)
	c.Assert(err, IsNil)
	c.Check(results.Tools, HasLen, len(permutations))
	for i, toolResult := range results.Tools {
		c.Logf("result item %d", i)
		c.Check(toolResult.Tag, Equals, s.rawMachine.Tag())
		c.Check(toolResult.Error, NotNil)
		err = *toolResult.Error
		c.Check(err, ErrorMatches, "invalid request")
	}
}

func (s *upgraderSuite) TestToolsForAgent(c *C) {
	cur := version.Current
	agent := params.Agent{
		Tag:    s.rawMachine.Tag(),
		Arch:   cur.Arch,
		Series: cur.Series,
	}

	args := params.Agents{Agents: []params.Agent{agent}}
	results, err := s.upgrader.Tools(args)
	c.Assert(err, IsNil)
	c.Check(results.Tools, HasLen, 1)
	toolResult := results.Tools[0]
	c.Check(toolResult.Tag, Equals, s.rawMachine.Tag())
	c.Assert(toolResult.Error, IsNil)
	c.Check(toolResult.Major, Equals, cur.Major)
	c.Check(toolResult.Minor, Equals, cur.Minor)
	c.Check(toolResult.Patch, Equals, cur.Patch)
	c.Check(toolResult.Build, Equals, cur.Build)
	c.Check(toolResult.Arch, Equals, cur.Arch)
	c.Check(toolResult.Series, Equals, cur.Series)
	c.Check(toolResult.URL, Not(Equals), "")
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
	err = *results.Results[0].Error
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
