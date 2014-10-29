// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader_test

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/apiserver/upgrader"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/version"
)

type upgraderSuite struct {
	jujutesting.JujuConnSuite

	// These are raw State objects. Use them for setup and assertions, but
	// should never be touched by the API calls themselves
	rawMachine *state.Machine
	apiMachine *state.Machine
	upgrader   *upgrader.UpgraderAPI
	resources  *common.Resources
	authorizer apiservertesting.FakeAuthorizer
}

var _ = gc.Suite(&upgraderSuite{})

func (s *upgraderSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	// Create a machine to work with
	var err error
	// The first machine created is the only one allowed to
	// JobManageEnviron
	s.apiMachine, err = s.State.AddMachine("quantal", state.JobHostUnits,
		state.JobManageEnviron)
	c.Assert(err, gc.IsNil)
	s.rawMachine, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)

	// The default auth is as the machine agent
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: s.rawMachine.Tag(),
	}
	s.upgrader, err = upgrader.NewUpgraderAPI(s.State, s.resources, s.authorizer)
	c.Assert(err, gc.IsNil)
}

func (s *upgraderSuite) TearDownTest(c *gc.C) {
	if s.resources != nil {
		s.resources.StopAll()
	}
	s.JujuConnSuite.TearDownTest(c)
}

func (s *upgraderSuite) TestWatchAPIVersionNothing(c *gc.C) {
	// Not an error to watch nothing
	results, err := s.upgrader.WatchAPIVersion(params.Entities{})
	c.Assert(err, gc.IsNil)
	c.Check(results.Results, gc.HasLen, 0)
}

func (s *upgraderSuite) TestWatchAPIVersion(c *gc.C) {
	args := params.Entities{
		Entities: []params.Entity{{Tag: s.rawMachine.Tag().String()}},
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

	err = statetesting.SetAgentVersion(s.State, version.MustParse("3.4.567.8"))
	c.Assert(err, gc.IsNil)
	wc.AssertOneChange()
	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *upgraderSuite) TestUpgraderAPIRefusesNonMachineAgent(c *gc.C) {
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = names.NewUnitTag("ubuntu/1")
	anUpgrader, err := upgrader.NewUpgraderAPI(s.State, s.resources, anAuthorizer)
	c.Check(err, gc.NotNil)
	c.Check(anUpgrader, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *upgraderSuite) TestWatchAPIVersionRefusesWrongAgent(c *gc.C) {
	// We are a machine agent, but not the one we are trying to track
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = names.NewMachineTag("12354")
	anUpgrader, err := upgrader.NewUpgraderAPI(s.State, s.resources, anAuthorizer)
	c.Check(err, gc.IsNil)
	args := params.Entities{
		Entities: []params.Entity{{Tag: s.rawMachine.Tag().String()}},
	}
	results, err := anUpgrader.WatchAPIVersion(args)
	// It is not an error to make the request, but the specific item is rejected
	c.Assert(err, gc.IsNil)
	c.Check(results.Results, gc.HasLen, 1)
	c.Check(results.Results[0].NotifyWatcherId, gc.Equals, "")
	c.Assert(results.Results[0].Error, gc.DeepEquals, apiservertesting.ErrUnauthorized)
}

func (s *upgraderSuite) TestToolsNothing(c *gc.C) {
	// Not an error to watch nothing
	results, err := s.upgrader.Tools(params.Entities{})
	c.Assert(err, gc.IsNil)
	c.Check(results.Results, gc.HasLen, 0)
}

func (s *upgraderSuite) TestToolsRefusesWrongAgent(c *gc.C) {
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = names.NewMachineTag("12354")
	anUpgrader, err := upgrader.NewUpgraderAPI(s.State, s.resources, anAuthorizer)
	c.Check(err, gc.IsNil)
	args := params.Entities{
		Entities: []params.Entity{{Tag: s.rawMachine.Tag().String()}},
	}
	results, err := anUpgrader.Tools(args)
	// It is not an error to make the request, but the specific item is rejected
	c.Assert(err, gc.IsNil)
	c.Check(results.Results, gc.HasLen, 1)
	toolResult := results.Results[0]
	c.Assert(toolResult.Error, gc.DeepEquals, apiservertesting.ErrUnauthorized)
}

func (s *upgraderSuite) TestToolsForAgent(c *gc.C) {
	cur := version.Current
	agent := params.Entity{Tag: s.rawMachine.Tag().String()}

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
		url := fmt.Sprintf("https://%s/environment/90168e4c-2f10-4e9c-83c2-feedfacee5a9/tools/%s", s.APIState.Addr(), version.Current)
		c.Check(agentTools.URL, gc.Equals, url)
		c.Check(agentTools.Version, gc.DeepEquals, cur)
	}
	assertTools()
}

func (s *upgraderSuite) TestSetToolsNothing(c *gc.C) {
	// Not an error to watch nothing
	results, err := s.upgrader.SetTools(params.EntitiesVersion{})
	c.Assert(err, gc.IsNil)
	c.Check(results.Results, gc.HasLen, 0)
}

func (s *upgraderSuite) TestSetToolsRefusesWrongAgent(c *gc.C) {
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = names.NewMachineTag("12354")
	anUpgrader, err := upgrader.NewUpgraderAPI(s.State, s.resources, anAuthorizer)
	c.Check(err, gc.IsNil)
	args := params.EntitiesVersion{
		AgentTools: []params.EntityVersion{{
			Tag: s.rawMachine.Tag().String(),
			Tools: &params.Version{
				Version: version.Current,
			},
		}},
	}

	results, err := anUpgrader.SetTools(args)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.DeepEquals, apiservertesting.ErrUnauthorized)
}

func (s *upgraderSuite) TestSetTools(c *gc.C) {
	cur := version.Current
	_, err := s.rawMachine.AgentTools()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	args := params.EntitiesVersion{
		AgentTools: []params.EntityVersion{{
			Tag: s.rawMachine.Tag().String(),
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
	err = s.rawMachine.Refresh()
	c.Assert(err, gc.IsNil)
	realTools, err := s.rawMachine.AgentTools()
	c.Assert(err, gc.IsNil)
	c.Check(realTools.Version.Arch, gc.Equals, cur.Arch)
	c.Check(realTools.Version.Series, gc.Equals, cur.Series)
	c.Check(realTools.Version.Major, gc.Equals, cur.Major)
	c.Check(realTools.Version.Minor, gc.Equals, cur.Minor)
	c.Check(realTools.Version.Patch, gc.Equals, cur.Patch)
	c.Check(realTools.Version.Build, gc.Equals, cur.Build)
	c.Check(realTools.URL, gc.Equals, "")
}

func (s *upgraderSuite) TestDesiredVersionNothing(c *gc.C) {
	// Not an error to watch nothing
	results, err := s.upgrader.DesiredVersion(params.Entities{})
	c.Assert(err, gc.IsNil)
	c.Check(results.Results, gc.HasLen, 0)
}

func (s *upgraderSuite) TestDesiredVersionRefusesWrongAgent(c *gc.C) {
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = names.NewMachineTag("12354")
	anUpgrader, err := upgrader.NewUpgraderAPI(s.State, s.resources, anAuthorizer)
	c.Check(err, gc.IsNil)
	args := params.Entities{
		Entities: []params.Entity{{Tag: s.rawMachine.Tag().String()}},
	}
	results, err := anUpgrader.DesiredVersion(args)
	// It is not an error to make the request, but the specific item is rejected
	c.Assert(err, gc.IsNil)
	c.Check(results.Results, gc.HasLen, 1)
	toolResult := results.Results[0]
	c.Assert(toolResult.Error, gc.DeepEquals, apiservertesting.ErrUnauthorized)
}

func (s *upgraderSuite) TestDesiredVersionNoticesMixedAgents(c *gc.C) {
	args := params.Entities{Entities: []params.Entity{
		{Tag: s.rawMachine.Tag().String()},
		{Tag: "machine-12345"},
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

func (s *upgraderSuite) TestDesiredVersionForAgent(c *gc.C) {
	args := params.Entities{Entities: []params.Entity{{Tag: s.rawMachine.Tag().String()}}}
	results, err := s.upgrader.DesiredVersion(args)
	c.Assert(err, gc.IsNil)
	c.Check(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
	agentVersion := results.Results[0].Version
	c.Assert(agentVersion, gc.NotNil)
	c.Check(*agentVersion, gc.DeepEquals, version.Current.Number)
}

func (s *upgraderSuite) bumpDesiredAgentVersion(c *gc.C) version.Number {
	// In order to call SetEnvironAgentVersion we have to first SetTools on
	// all the existing machines
	s.apiMachine.SetAgentVersion(version.Current)
	s.rawMachine.SetAgentVersion(version.Current)
	newer := version.Current
	newer.Patch++
	err := s.State.SetEnvironAgentVersion(newer.Number)
	c.Assert(err, gc.IsNil)
	cfg, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	vers, ok := cfg.AgentVersion()
	c.Assert(ok, jc.IsTrue)
	c.Check(vers, gc.Equals, newer.Number)
	return newer.Number
}

func (s *upgraderSuite) TestDesiredVersionUnrestrictedForAPIAgents(c *gc.C) {
	newVersion := s.bumpDesiredAgentVersion(c)
	// Grab a different Upgrader for the apiMachine
	authorizer := apiservertesting.FakeAuthorizer{
		Tag: s.apiMachine.Tag(),
	}
	upgraderAPI, err := upgrader.NewUpgraderAPI(s.State, s.resources, authorizer)
	c.Assert(err, gc.IsNil)
	args := params.Entities{Entities: []params.Entity{{Tag: s.apiMachine.Tag().String()}}}
	results, err := upgraderAPI.DesiredVersion(args)
	c.Assert(err, gc.IsNil)
	c.Check(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
	agentVersion := results.Results[0].Version
	c.Assert(agentVersion, gc.NotNil)
	c.Check(*agentVersion, gc.DeepEquals, newVersion)
}

func (s *upgraderSuite) TestDesiredVersionRestrictedForNonAPIAgents(c *gc.C) {
	newVersion := s.bumpDesiredAgentVersion(c)
	c.Assert(newVersion, gc.Not(gc.Equals), version.Current.Number)
	args := params.Entities{Entities: []params.Entity{{Tag: s.rawMachine.Tag().String()}}}
	results, err := s.upgrader.DesiredVersion(args)
	c.Assert(err, gc.IsNil)
	c.Check(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
	agentVersion := results.Results[0].Version
	c.Assert(agentVersion, gc.NotNil)
	c.Check(*agentVersion, gc.DeepEquals, version.Current.Number)
}
