// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/errors"
	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/api/upgrader"
	statetesting "launchpad.net/juju-core/state/testing"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/version"
)

type unitUpgraderSuite struct {
	jujutesting.JujuConnSuite

	stateAPI *api.State

	// These are raw State objects. Use them for setup and assertions, but
	// should never be touched by the API calls themselves
	rawMachine *state.Machine
	rawUnit    *state.Unit

	st *upgrader.State
}

var _ = gc.Suite(&unitUpgraderSuite{})

func (s *unitUpgraderSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	s.rawMachine, _, _, s.rawUnit = s.addMachineServiceCharmAndUnit(c, "wordpress")
	password, err := utils.RandomPassword()
	c.Assert(err, gc.IsNil)
	err = s.rawUnit.SetPassword(password)
	c.Assert(err, gc.IsNil)
	s.stateAPI = s.OpenAPIAs(c, s.rawUnit.Tag(), password)

	// Create the upgrader facade.
	s.st = s.stateAPI.Upgrader()
	c.Assert(s.st, gc.NotNil)
}

func (s *unitUpgraderSuite) addMachineServiceCharmAndUnit(c *gc.C, serviceName string) (*state.Machine, *state.Service, *state.Charm, *state.Unit) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	charm := s.AddTestingCharm(c, serviceName)
	service := s.AddTestingService(c, serviceName, charm)
	unit, err := service.AddUnit()
	c.Assert(err, gc.IsNil)
	err = unit.AssignToMachine(machine)
	c.Assert(err, gc.IsNil)
	return machine, service, charm, unit
}

func (s *unitUpgraderSuite) TestSetVersionWrongUnit(c *gc.C) {
	err := s.st.SetVersion("unit-wordpress-42", version.Current)
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(err, jc.Satisfies, params.IsCodeUnauthorized)
}

func (s *unitUpgraderSuite) TestSetVersionNotUnit(c *gc.C) {
	err := s.st.SetVersion("foo-42", version.Current)
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(err, jc.Satisfies, params.IsCodeUnauthorized)
}

func (s *unitUpgraderSuite) TestSetVersion(c *gc.C) {
	cur := version.Current
	agentTools, err := s.rawUnit.AgentTools()
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)
	c.Assert(agentTools, gc.IsNil)
	err = s.st.SetVersion(s.rawUnit.Tag(), cur)
	c.Assert(err, gc.IsNil)
	s.rawUnit.Refresh()
	agentTools, err = s.rawUnit.AgentTools()
	c.Assert(err, gc.IsNil)
	c.Check(agentTools.Version, gc.Equals, cur)
}

func (s *unitUpgraderSuite) TestToolsWrongUnit(c *gc.C) {
	tools, _, err := s.st.Tools("unit-wordpress-42")
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(err, jc.Satisfies, params.IsCodeUnauthorized)
	c.Assert(tools, gc.IsNil)
}

func (s *unitUpgraderSuite) TestToolsNotUnit(c *gc.C) {
	tools, _, err := s.st.Tools("foo-42")
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(err, jc.Satisfies, params.IsCodeUnauthorized)
	c.Assert(tools, gc.IsNil)
}

func (s *unitUpgraderSuite) TestTools(c *gc.C) {
	cur := version.Current
	curTools := &tools.Tools{Version: cur, URL: ""}
	curTools.Version.Minor++
	s.rawMachine.SetAgentVersion(cur)
	// UnitUpgrader.Tools returns the *desired* set of tools, not the currently
	// running set. We want to be upgraded to cur.Version
	stateTools, _, err := s.st.Tools(s.rawUnit.Tag())
	c.Assert(err, gc.IsNil)
	c.Check(stateTools.Version.Number, gc.DeepEquals, version.Current.Number)
	c.Assert(stateTools.URL, gc.NotNil)
}

func (s *unitUpgraderSuite) TestWatchAPIVersion(c *gc.C) {
	w, err := s.st.WatchAPIVersion(s.rawUnit.Tag())
	c.Assert(err, gc.IsNil)
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewNotifyWatcherC(c, s.BackingState, w)
	// Initial event
	wc.AssertOneChange()
	vers := version.MustParseBinary("10.20.34-quantal-amd64")
	err = s.rawMachine.SetAgentVersion(vers)
	c.Assert(err, gc.IsNil)
	// One change noticing the new version
	wc.AssertOneChange()
	vers = version.MustParseBinary("10.20.35-quantal-amd64")
	err = s.rawMachine.SetAgentVersion(vers)
	c.Assert(err, gc.IsNil)
	wc.AssertOneChange()
	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *unitUpgraderSuite) TestWatchAPIVersionWrongUnit(c *gc.C) {
	_, err := s.st.WatchAPIVersion("unit-wordpress-42")
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(err, jc.Satisfies, params.IsCodeUnauthorized)
}

func (s *unitUpgraderSuite) TestWatchAPIVersionNotUnit(c *gc.C) {
	_, err := s.st.WatchAPIVersion("foo-42")
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(err, jc.Satisfies, params.IsCodeUnauthorized)
}

func (s *unitUpgraderSuite) TestDesiredVersion(c *gc.C) {
	cur := version.Current
	curTools := &tools.Tools{Version: cur, URL: ""}
	curTools.Version.Minor++
	s.rawMachine.SetAgentVersion(cur)
	// UnitUpgrader.DesiredVersion returns the *desired* set of tools, not the
	// currently running set. We want to be upgraded to cur.Version
	stateVersion, err := s.st.DesiredVersion(s.rawUnit.Tag())
	c.Assert(err, gc.IsNil)
	c.Assert(stateVersion, gc.Equals, cur.Number)
}

func (s *unitUpgraderSuite) TestDesiredVersionWrongUnit(c *gc.C) {
	_, err := s.st.DesiredVersion("unit-wordpress-42")
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(err, jc.Satisfies, params.IsCodeUnauthorized)
}

func (s *unitUpgraderSuite) TestDesiredVersionNotUnit(c *gc.C) {
	_, err := s.st.DesiredVersion("foo-42")
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(err, jc.Satisfies, params.IsCodeUnauthorized)
}
