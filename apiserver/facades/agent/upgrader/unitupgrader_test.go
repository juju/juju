// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/agent/upgrader"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/instance"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	jujuversion "github.com/juju/juju/version"
)

type unitUpgraderSuite struct {
	jujutesting.ApiServerSuite

	// These are raw State objects. Use them for setup and assertions, but
	// should never be touched by the API calls themselves
	rawMachine *state.Machine
	rawUnit    *state.Unit
	upgrader   *upgrader.UnitUpgraderAPI
	resources  *common.Resources
	authorizer apiservertesting.FakeAuthorizer
}

var _ = gc.Suite(&unitUpgraderSuite{})

func (s *unitUpgraderSuite) SetUpTest(c *gc.C) {
	s.ApiServerSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	// Create a machine and unit to work with
	st := s.ControllerModel(c).State()
	machine, err := st.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	arch := arch.DefaultArchitecture
	hwChar := &instance.HardwareCharacteristics{
		Arch: &arch,
	}
	instId := instance.Id("i-host-machine")
	err = machine.SetProvisioned(instId, "", "fake-nonce", hwChar)
	c.Assert(err, jc.ErrorIsNil)

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	app := f.MakeApplication(c, &factory.ApplicationParams{
		Name:  "wordpress",
		Charm: f.MakeCharm(c, &factory.CharmParams{Name: "wordpress"}),
	})
	s.rawUnit, err = app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	// Assign the unit to the machine.
	s.rawMachine, err = s.rawUnit.AssignToCleanMachine()
	c.Assert(err, jc.ErrorIsNil)

	// The default auth is as the unit agent
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: s.rawUnit.Tag(),
	}
	s.upgrader, err = upgrader.NewUnitUpgraderAPI(facadetest.Context{
		State_:     st,
		Resources_: s.resources,
		Auth_:      s.authorizer,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *unitUpgraderSuite) TearDownTest(c *gc.C) {
	if s.resources != nil {
		s.resources.StopAll()
	}
	s.ApiServerSuite.TearDownTest(c)
}

func (s *unitUpgraderSuite) TestWatchAPIVersionNothing(c *gc.C) {
	// Not an error to watch nothing
	results, err := s.upgrader.WatchAPIVersion(context.Background(), params.Entities{})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results.Results, gc.HasLen, 0)
}

func (s *unitUpgraderSuite) TestWatchAPIVersion(c *gc.C) {
	args := params.Entities{
		Entities: []params.Entity{{Tag: s.rawUnit.Tag().String()}},
	}
	results, err := s.upgrader.WatchAPIVersion(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results.Results, gc.HasLen, 1)
	c.Check(results.Results[0].NotifyWatcherId, gc.Not(gc.Equals), "")
	c.Check(results.Results[0].Error, gc.IsNil)
	resource := s.resources.Get(results.Results[0].NotifyWatcherId)
	c.Check(resource, gc.NotNil)

	w := resource.(state.NotifyWatcher)
	wc := statetesting.NewNotifyWatcherC(c, w)
	wc.AssertNoChange()

	err = s.rawMachine.SetAgentVersion(version.MustParseBinary("3.4.567.8-ubuntu-amd64"))
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()
	workertest.CleanKill(c, w)
	wc.AssertClosed()
}

func (s *unitUpgraderSuite) TestUpgraderAPIRefusesNonUnitAgent(c *gc.C) {
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = names.NewMachineTag("7")
	anUpgrader, err := upgrader.NewUnitUpgraderAPI(facadetest.Context{
		State_:     s.ControllerModel(c).State(),
		Resources_: s.resources,
		Auth_:      anAuthorizer,
	})
	c.Check(err, gc.NotNil)
	c.Check(anUpgrader, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *unitUpgraderSuite) TestWatchAPIVersionRefusesWrongAgent(c *gc.C) {
	// We are a unit agent, but not the one we are trying to track
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = names.NewUnitTag("wordpress/12354")
	anUpgrader, err := upgrader.NewUnitUpgraderAPI(facadetest.Context{
		State_:     s.ControllerModel(c).State(),
		Resources_: s.resources,
		Auth_:      anAuthorizer,
	})
	c.Check(err, jc.ErrorIsNil)
	args := params.Entities{
		Entities: []params.Entity{{Tag: s.rawUnit.Tag().String()}},
	}
	results, err := anUpgrader.WatchAPIVersion(context.Background(), args)
	// It is not an error to make the request, but the specific item is rejected
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results.Results, gc.HasLen, 1)
	c.Check(results.Results[0].NotifyWatcherId, gc.Equals, "")
	c.Assert(results.Results[0].Error, gc.DeepEquals, apiservertesting.ErrUnauthorized)
}

func (s *unitUpgraderSuite) TestToolsNothing(c *gc.C) {
	// Not an error to watch nothing
	results, err := s.upgrader.Tools(context.Background(), params.Entities{})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results.Results, gc.HasLen, 0)
}

func (s *unitUpgraderSuite) TestToolsRefusesWrongAgent(c *gc.C) {
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = names.NewUnitTag("wordpress/12354")
	anUpgrader, err := upgrader.NewUnitUpgraderAPI(facadetest.Context{
		State_:     s.ControllerModel(c).State(),
		Resources_: s.resources,
		Auth_:      anAuthorizer,
	})
	c.Check(err, jc.ErrorIsNil)
	args := params.Entities{
		Entities: []params.Entity{{Tag: s.rawUnit.Tag().String()}},
	}
	results, err := anUpgrader.Tools(context.Background(), args)
	// It is not an error to make the request, but the specific item is rejected
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	toolResult := results.Results[0]
	c.Assert(toolResult.Error, gc.DeepEquals, apiservertesting.ErrUnauthorized)
}

func (s *unitUpgraderSuite) TestToolsForAgent(c *gc.C) {
	agent := params.Entity{Tag: s.rawUnit.Tag().String()}

	// The machine must have its existing tools set before we query for the
	// next tools. This is so that we can grab Arch and OSType without
	// having to pass it in again
	current := testing.CurrentVersion()
	err := s.rawMachine.SetAgentVersion(current)
	c.Assert(err, jc.ErrorIsNil)

	args := params.Entities{Entities: []params.Entity{agent}}
	results, err := s.upgrader.Tools(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	assertTools := func() {
		c.Check(results.Results, gc.HasLen, 1)
		c.Assert(results.Results[0].Error, gc.IsNil)
		agentTools := results.Results[0].ToolsList[0]
		c.Check(agentTools.Version.Number, gc.DeepEquals, jujuversion.Current)
		c.Assert(agentTools.URL, gc.NotNil)
	}
	assertTools()
}

func (s *unitUpgraderSuite) TestSetToolsNothing(c *gc.C) {
	// Not an error to watch nothing
	results, err := s.upgrader.SetTools(context.Background(), params.EntitiesVersion{})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results.Results, gc.HasLen, 0)
}

func (s *unitUpgraderSuite) TestSetToolsRefusesWrongAgent(c *gc.C) {
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = names.NewUnitTag("wordpress/12354")
	anUpgrader, err := upgrader.NewUnitUpgraderAPI(facadetest.Context{
		State_:     s.ControllerModel(c).State(),
		Resources_: s.resources,
		Auth_:      anAuthorizer,
	})
	c.Check(err, jc.ErrorIsNil)
	args := params.EntitiesVersion{
		AgentTools: []params.EntityVersion{{
			Tag: s.rawUnit.Tag().String(),
			Tools: &params.Version{
				Version: testing.CurrentVersion(),
			},
		}},
	}

	results, err := anUpgrader.SetTools(context.Background(), args)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.DeepEquals, apiservertesting.ErrUnauthorized)
}

func (s *unitUpgraderSuite) TestSetTools(c *gc.C) {
	cur := testing.CurrentVersion()
	_, err := s.rawUnit.AgentTools()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	args := params.EntitiesVersion{
		AgentTools: []params.EntityVersion{{
			Tag: s.rawUnit.Tag().String(),
			Tools: &params.Version{
				Version: cur,
			}},
		},
	}
	results, err := s.upgrader.SetTools(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
	// Check that the new value actually got set, we must Refresh because
	// it was set on a different Machine object
	err = s.rawUnit.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	realTools, err := s.rawUnit.AgentTools()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(realTools.Version.Arch, gc.Equals, cur.Arch)
	c.Check(realTools.Version.Release, gc.Equals, cur.Release)
	c.Check(realTools.Version.Major, gc.Equals, cur.Major)
	c.Check(realTools.Version.Minor, gc.Equals, cur.Minor)
	c.Check(realTools.Version.Patch, gc.Equals, cur.Patch)
	c.Check(realTools.Version.Build, gc.Equals, cur.Build)
	c.Check(realTools.URL, gc.Equals, "")
}

func (s *unitUpgraderSuite) TestDesiredVersionNothing(c *gc.C) {
	// Not an error to watch nothing
	results, err := s.upgrader.DesiredVersion(context.Background(), params.Entities{})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results.Results, gc.HasLen, 0)
}

func (s *unitUpgraderSuite) TestDesiredVersionRefusesWrongAgent(c *gc.C) {
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = names.NewUnitTag("wordpress/12354")
	anUpgrader, err := upgrader.NewUnitUpgraderAPI(facadetest.Context{
		State_:     s.ControllerModel(c).State(),
		Resources_: s.resources,
		Auth_:      anAuthorizer,
	})
	c.Check(err, jc.ErrorIsNil)
	args := params.Entities{
		Entities: []params.Entity{{Tag: s.rawUnit.Tag().String()}},
	}
	results, err := anUpgrader.DesiredVersion(context.Background(), args)
	// It is not an error to make the request, but the specific item is rejected
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results.Results, gc.HasLen, 1)
	toolResult := results.Results[0]
	c.Assert(toolResult.Error, gc.DeepEquals, apiservertesting.ErrUnauthorized)
}

func (s *unitUpgraderSuite) TestDesiredVersionNoticesMixedAgents(c *gc.C) {
	current := testing.CurrentVersion()
	err := s.rawMachine.SetAgentVersion(current)
	c.Assert(err, jc.ErrorIsNil)
	args := params.Entities{Entities: []params.Entity{
		{Tag: s.rawUnit.Tag().String()},
		{Tag: "unit-wordpress-12345"},
	}}
	results, err := s.upgrader.DesiredVersion(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results.Results, gc.HasLen, 2)
	c.Assert(results.Results[0].Error, gc.IsNil)
	agentVersion := results.Results[0].Version
	c.Assert(agentVersion, gc.NotNil)
	c.Check(*agentVersion, gc.DeepEquals, jujuversion.Current)

	c.Assert(results.Results[1].Error, gc.DeepEquals, apiservertesting.ErrUnauthorized)
	c.Assert(results.Results[1].Version, gc.IsNil)

}

func (s *unitUpgraderSuite) TestDesiredVersionForAgent(c *gc.C) {
	current := testing.CurrentVersion()
	err := s.rawMachine.SetAgentVersion(current)
	c.Assert(err, jc.ErrorIsNil)
	args := params.Entities{Entities: []params.Entity{{Tag: s.rawUnit.Tag().String()}}}
	results, err := s.upgrader.DesiredVersion(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
	agentVersion := results.Results[0].Version
	c.Assert(agentVersion, gc.NotNil)
	c.Check(*agentVersion, gc.DeepEquals, jujuversion.Current)
}
