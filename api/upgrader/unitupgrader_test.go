// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v2"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/upgrader"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/watcher/watchertest"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/tools"
)

type unitUpgraderSuite struct {
	jujutesting.JujuConnSuite

	stateAPI api.Connection

	// These are raw State objects. Use them for setup and assertions, but
	// should never be touched by the API calls themselves
	rawMachine *state.Machine
	rawUnit    *state.Unit

	st *upgrader.State
}

var _ = gc.Suite(&unitUpgraderSuite{})

func (s *unitUpgraderSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	s.rawMachine, _, _, s.rawUnit = s.addMachineApplicationCharmAndUnit(c, "wordpress")
	password, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = s.rawUnit.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)
	s.stateAPI = s.OpenAPIAs(c, s.rawUnit.Tag(), password)

	// Create the upgrader facade.
	s.st = s.stateAPI.Upgrader()
	c.Assert(s.st, gc.NotNil)

	s.WaitForModelWatchersIdle(c, s.State.ModelUUID())
}

func (s *unitUpgraderSuite) addMachineApplicationCharmAndUnit(c *gc.C, appName string) (*state.Machine, *state.Application, *state.Charm, *state.Unit) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	charm := s.AddTestingCharm(c, appName)
	app := s.AddTestingApplication(c, appName, charm)
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)
	return machine, app, charm, unit
}

func (s *unitUpgraderSuite) TestSetVersionWrongUnit(c *gc.C) {
	err := s.st.SetVersion("unit-wordpress-42", testing.CurrentVersion(c))
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(err, jc.Satisfies, params.IsCodeUnauthorized)
}

func (s *unitUpgraderSuite) TestSetVersionNotUnit(c *gc.C) {
	err := s.st.SetVersion("foo-42", testing.CurrentVersion(c))
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(err, jc.Satisfies, params.IsCodeUnauthorized)
}

func (s *unitUpgraderSuite) TestSetVersion(c *gc.C) {
	current := testing.CurrentVersion(c)
	agentTools, err := s.rawUnit.AgentTools()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(agentTools, gc.IsNil)
	err = s.st.SetVersion(s.rawUnit.Tag().String(), current)
	c.Assert(err, jc.ErrorIsNil)
	s.rawUnit.Refresh()
	agentTools, err = s.rawUnit.AgentTools()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(agentTools.Version, gc.Equals, current)
}

func (s *unitUpgraderSuite) TestToolsWrongUnit(c *gc.C) {
	tools, err := s.st.Tools("unit-wordpress-42")
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(err, jc.Satisfies, params.IsCodeUnauthorized)
	c.Assert(tools, gc.IsNil)
}

func (s *unitUpgraderSuite) TestToolsNotUnit(c *gc.C) {
	tools, err := s.st.Tools("foo-42")
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(err, jc.Satisfies, params.IsCodeUnauthorized)
	c.Assert(tools, gc.IsNil)
}

func (s *unitUpgraderSuite) TestTools(c *gc.C) {
	current := testing.CurrentVersion(c)
	curTools := &tools.Tools{Version: current, URL: ""}
	curTools.Version.Minor++
	s.rawMachine.SetAgentVersion(current)
	// UnitUpgrader.Tools returns the *desired* set of tools, not the currently
	// running set. We want to be upgraded to cur.Version
	stateToolsList, err := s.st.Tools(s.rawUnit.Tag().String())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(stateToolsList, gc.HasLen, 1)
	stateTools := stateToolsList[0]
	c.Check(stateTools.Version.Number, gc.DeepEquals, current.Number)
	c.Assert(stateTools.URL, gc.NotNil)
}

func (s *unitUpgraderSuite) TestWatchAPIVersion(c *gc.C) {
	w, err := s.st.WatchAPIVersion(s.rawUnit.Tag().String())
	c.Assert(err, jc.ErrorIsNil)
	wc := watchertest.NewNotifyWatcherC(c, w, s.BackingState.StartSync)
	defer wc.AssertStops()

	// Initial event
	wc.AssertOneChange()
	vers := version.MustParseBinary("10.20.34-quantal-amd64")
	err = s.rawMachine.SetAgentVersion(vers)
	c.Assert(err, jc.ErrorIsNil)

	// One change noticing the new version
	wc.AssertOneChange()
	vers = version.MustParseBinary("10.20.35-quantal-amd64")
	err = s.rawMachine.SetAgentVersion(vers)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()
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
	current := testing.CurrentVersion(c)
	curTools := &tools.Tools{Version: current, URL: ""}
	curTools.Version.Minor++
	s.rawMachine.SetAgentVersion(current)
	// UnitUpgrader.DesiredVersion returns the *desired* set of tools, not the
	// currently running set. We want to be upgraded to cur.Version
	stateVersion, err := s.st.DesiredVersion(s.rawUnit.Tag().String())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(stateVersion, gc.Equals, current.Number)
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
