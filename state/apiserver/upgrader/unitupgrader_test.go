// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/errors"
	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
	apiservertesting "launchpad.net/juju-core/state/apiserver/testing"
	"launchpad.net/juju-core/state/apiserver/upgrader"
	statetesting "launchpad.net/juju-core/state/testing"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/version"
)

type unitUpgraderSuite struct {
	jujutesting.JujuConnSuite

	// These are raw State objects. Use them for setup and assertions, but
	// should never be touched by the API calls themselves
	rawMachine *state.Machine
	rawUnit    *state.Unit
	upgrader   *upgrader.UnitUpgraderAPI
	resources  *common.Resources
	authorizer apiservertesting.FakeAuthorizer

	// Tools for the assigned machine.
	fakeTools *tools.Tools
}

var _ = gc.Suite(&unitUpgraderSuite{})

func (s *unitUpgraderSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.resources = common.NewResources()

	// Create a machine and unit to work with
	var err error
	_, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	svc := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	s.rawUnit, err = svc.AddUnit()
	c.Assert(err, gc.IsNil)
	// Assign the unit to the machine.
	s.rawMachine, err = s.rawUnit.AssignToCleanMachine()
	c.Assert(err, gc.IsNil)

	// The default auth is as the unit agent
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag:       s.rawUnit.Tag(),
		LoggedIn:  true,
		UnitAgent: true,
	}
	s.upgrader, err = upgrader.NewUnitUpgraderAPI(s.State, s.resources, s.authorizer, s.DataDir())
	c.Assert(err, gc.IsNil)
}

func (s *unitUpgraderSuite) TearDownTest(c *gc.C) {
	if s.resources != nil {
		s.resources.StopAll()
	}
	s.JujuConnSuite.TearDownTest(c)
}

func (s *unitUpgraderSuite) TestWatchAPIVersionNothing(c *gc.C) {
	// Not an error to watch nothing
	results, err := s.upgrader.WatchAPIVersion(params.Entities{})
	c.Assert(err, gc.IsNil)
	c.Check(results.Results, gc.HasLen, 0)
}

func (s *unitUpgraderSuite) TestWatchAPIVersion(c *gc.C) {
	args := params.Entities{
		Entities: []params.Entity{{Tag: s.rawUnit.Tag()}},
	}
	results, err := s.upgrader.WatchAPIVersion(args)
	c.Assert(err, gc.IsNil)
	c.Check(results.Results, gc.HasLen, 1)
	c.Check(results.Results[0].NotifyWatcherId, gc.Not(gc.Equals), "")
	c.Check(results.Results[0].Error, gc.IsNil)
	resource := s.resources.Get(results.Results[0].NotifyWatcherId)
	c.Check(resource, gc.NotNil)

	w := resource.(state.NotifyWatcher)
	wc := statetesting.NewNotifyWatcherC(c, s.State, w)
	wc.AssertNoChange()

	err = s.rawMachine.SetAgentVersion(version.MustParseBinary("3.4.567.8-quantal-amd64"))
	c.Assert(err, gc.IsNil)
	wc.AssertOneChange()
	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *unitUpgraderSuite) TestUpgraderAPIRefusesNonUnitAgent(c *gc.C) {
	anAuthorizer := s.authorizer
	anAuthorizer.MachineAgent = true
	anAuthorizer.UnitAgent = false
	anUpgrader, err := upgrader.NewUnitUpgraderAPI(s.State, s.resources, anAuthorizer, "")
	c.Check(err, gc.NotNil)
	c.Check(anUpgrader, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *unitUpgraderSuite) TestWatchAPIVersionRefusesWrongAgent(c *gc.C) {
	// We are a unit agent, but not the one we are trying to track
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = "unit-wordpress-12354"
	anUpgrader, err := upgrader.NewUnitUpgraderAPI(s.State, s.resources, anAuthorizer, "")
	c.Check(err, gc.IsNil)
	args := params.Entities{
		Entities: []params.Entity{{Tag: s.rawUnit.Tag()}},
	}
	results, err := anUpgrader.WatchAPIVersion(args)
	// It is not an error to make the request, but the specific item is rejected
	c.Assert(err, gc.IsNil)
	c.Check(results.Results, gc.HasLen, 1)
	c.Check(results.Results[0].NotifyWatcherId, gc.Equals, "")
	c.Assert(results.Results[0].Error, gc.DeepEquals, apiservertesting.ErrUnauthorized)
}

func (s *unitUpgraderSuite) TestToolsNothing(c *gc.C) {
	// Not an error to watch nothing
	results, err := s.upgrader.Tools(params.Entities{})
	c.Assert(err, gc.IsNil)
	c.Check(results.Results, gc.HasLen, 0)
}

func (s *unitUpgraderSuite) TestToolsRefusesWrongAgent(c *gc.C) {
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = "unit-wordpress-12354"
	anUpgrader, err := upgrader.NewUnitUpgraderAPI(s.State, s.resources, anAuthorizer, "")
	c.Check(err, gc.IsNil)
	args := params.Entities{
		Entities: []params.Entity{{Tag: s.rawUnit.Tag()}},
	}
	results, err := anUpgrader.Tools(args)
	// It is not an error to make the request, but the specific item is rejected
	c.Assert(err, gc.IsNil)
	c.Check(results.Results, gc.HasLen, 1)
	toolResult := results.Results[0]
	c.Assert(toolResult.Error, gc.DeepEquals, apiservertesting.ErrUnauthorized)
}

func (s *unitUpgraderSuite) TestToolsForAgent(c *gc.C) {
	agent := params.Entity{Tag: s.rawUnit.Tag()}

	// The machine must have its existing tools set before we query for the
	// next tools. This is so that we can grab Arch and Series without
	// having to pass it in again
	err := s.rawMachine.SetAgentVersion(version.Current)
	c.Assert(err, gc.IsNil)

	args := params.Entities{Entities: []params.Entity{agent}}
	results, err := s.upgrader.Tools(args)
	c.Assert(err, gc.IsNil)
	assertTools := func() {
		c.Check(results.Results, gc.HasLen, 1)
		c.Assert(results.Results[0].Error, gc.IsNil)
		agentTools := results.Results[0].Tools
		c.Check(agentTools.Version.Number, gc.DeepEquals, version.Current.Number)
		c.Assert(agentTools.URL, gc.NotNil)
	}
	assertTools()
}

func (s *unitUpgraderSuite) TestSetToolsNothing(c *gc.C) {
	// Not an error to watch nothing
	results, err := s.upgrader.SetTools(params.EntitiesVersion{})
	c.Assert(err, gc.IsNil)
	c.Check(results.Results, gc.HasLen, 0)
}

func (s *unitUpgraderSuite) TestSetToolsRefusesWrongAgent(c *gc.C) {
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = "unit-wordpress-12354"
	anUpgrader, err := upgrader.NewUnitUpgraderAPI(s.State, s.resources, anAuthorizer, "")
	c.Check(err, gc.IsNil)
	args := params.EntitiesVersion{
		AgentTools: []params.EntityVersion{{
			Tag: s.rawUnit.Tag(),
			Tools: &params.Version{
				Version: version.Current,
			},
		}},
	}

	results, err := anUpgrader.SetTools(args)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.DeepEquals, apiservertesting.ErrUnauthorized)
}

func (s *unitUpgraderSuite) TestSetTools(c *gc.C) {
	cur := version.Current
	_, err := s.rawUnit.AgentTools()
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)
	args := params.EntitiesVersion{
		AgentTools: []params.EntityVersion{{
			Tag: s.rawUnit.Tag(),
			Tools: &params.Version{
				Version: cur,
			}},
		},
	}
	results, err := s.upgrader.SetTools(args)
	c.Assert(err, gc.IsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
	// Check that the new value actually got set, we must Refresh because
	// it was set on a different Machine object
	err = s.rawUnit.Refresh()
	c.Assert(err, gc.IsNil)
	realTools, err := s.rawUnit.AgentTools()
	c.Assert(err, gc.IsNil)
	c.Check(realTools.Version.Arch, gc.Equals, cur.Arch)
	c.Check(realTools.Version.Series, gc.Equals, cur.Series)
	c.Check(realTools.Version.Major, gc.Equals, cur.Major)
	c.Check(realTools.Version.Minor, gc.Equals, cur.Minor)
	c.Check(realTools.Version.Patch, gc.Equals, cur.Patch)
	c.Check(realTools.Version.Build, gc.Equals, cur.Build)
	c.Check(realTools.URL, gc.Equals, "")
}

func (s *unitUpgraderSuite) TestDesiredVersionNothing(c *gc.C) {
	// Not an error to watch nothing
	results, err := s.upgrader.DesiredVersion(params.Entities{})
	c.Assert(err, gc.IsNil)
	c.Check(results.Results, gc.HasLen, 0)
}

func (s *unitUpgraderSuite) TestDesiredVersionRefusesWrongAgent(c *gc.C) {
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = "unit-wordpress-12354"
	anUpgrader, err := upgrader.NewUnitUpgraderAPI(s.State, s.resources, anAuthorizer, "")
	c.Check(err, gc.IsNil)
	args := params.Entities{
		Entities: []params.Entity{{Tag: s.rawUnit.Tag()}},
	}
	results, err := anUpgrader.DesiredVersion(args)
	// It is not an error to make the request, but the specific item is rejected
	c.Assert(err, gc.IsNil)
	c.Check(results.Results, gc.HasLen, 1)
	toolResult := results.Results[0]
	c.Assert(toolResult.Error, gc.DeepEquals, apiservertesting.ErrUnauthorized)
}

func (s *unitUpgraderSuite) TestDesiredVersionNoticesMixedAgents(c *gc.C) {
	err := s.rawMachine.SetAgentVersion(version.Current)
	c.Assert(err, gc.IsNil)
	args := params.Entities{Entities: []params.Entity{
		{Tag: s.rawUnit.Tag()},
		{Tag: "unit-wordpress-12345"},
	}}
	results, err := s.upgrader.DesiredVersion(args)
	c.Assert(err, gc.IsNil)
	c.Check(results.Results, gc.HasLen, 2)
	c.Assert(results.Results[0].Error, gc.IsNil)
	agentVersion := results.Results[0].Version
	c.Assert(agentVersion, gc.NotNil)
	c.Check(*agentVersion, gc.DeepEquals, version.Current.Number)

	c.Assert(results.Results[1].Error, gc.DeepEquals, apiservertesting.ErrUnauthorized)
	c.Assert(results.Results[1].Version, gc.IsNil)

}

func (s *unitUpgraderSuite) TestDesiredVersionForAgent(c *gc.C) {
	err := s.rawMachine.SetAgentVersion(version.Current)
	c.Assert(err, gc.IsNil)
	args := params.Entities{Entities: []params.Entity{{Tag: s.rawUnit.Tag()}}}
	results, err := s.upgrader.DesiredVersion(args)
	c.Assert(err, gc.IsNil)
	c.Check(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
	agentVersion := results.Results[0].Version
	c.Assert(agentVersion, gc.NotNil)
	c.Check(*agentVersion, gc.DeepEquals, version.Current.Number)
}
