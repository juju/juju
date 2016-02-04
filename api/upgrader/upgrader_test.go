// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader_test

import (
	"fmt"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/upgrader"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/tools"
	"github.com/juju/juju/version"
	"github.com/juju/juju/watcher/watchertest"
)

type machineUpgraderSuite struct {
	testing.JujuConnSuite

	stateAPI api.Connection

	// These are raw State objects. Use them for setup and assertions, but
	// should never be touched by the API calls themselves
	rawMachine *state.Machine

	st *upgrader.State
}

var _ = gc.Suite(&machineUpgraderSuite{})

func (s *machineUpgraderSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.stateAPI, s.rawMachine = s.OpenAPIAsNewMachine(c)
	// Create the upgrader facade.
	s.st = s.stateAPI.Upgrader()
	c.Assert(s.st, gc.NotNil)
}

// Note: This is really meant as a unit-test, this isn't a test that should
//       need all of the setup we have for this test suite
func (s *machineUpgraderSuite) TestNew(c *gc.C) {
	upgrader := upgrader.NewState(s.stateAPI)
	c.Assert(upgrader, gc.NotNil)
}

func (s *machineUpgraderSuite) TestSetVersionWrongMachine(c *gc.C) {
	err := s.st.SetVersion("machine-42", current)
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(err, jc.Satisfies, params.IsCodeUnauthorized)
}

func (s *machineUpgraderSuite) TestSetVersionNotMachine(c *gc.C) {
	err := s.st.SetVersion("foo-42", current)
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(err, jc.Satisfies, params.IsCodeUnauthorized)
}

func (s *machineUpgraderSuite) TestSetVersion(c *gc.C) {
	agentTools, err := s.rawMachine.AgentTools()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(agentTools, gc.IsNil)
	err = s.st.SetVersion(s.rawMachine.Tag().String(), current)
	c.Assert(err, jc.ErrorIsNil)
	s.rawMachine.Refresh()
	agentTools, err = s.rawMachine.AgentTools()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(agentTools.Version, gc.Equals, current)
}

func (s *machineUpgraderSuite) TestToolsWrongMachine(c *gc.C) {
	tools, err := s.st.Tools("machine-42")
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(err, jc.Satisfies, params.IsCodeUnauthorized)
	c.Assert(tools, gc.IsNil)
}

func (s *machineUpgraderSuite) TestToolsNotMachine(c *gc.C) {
	tools, err := s.st.Tools("foo-42")
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(err, jc.Satisfies, params.IsCodeUnauthorized)
	c.Assert(tools, gc.IsNil)
}

func (s *machineUpgraderSuite) TestTools(c *gc.C) {
	curTools := &tools.Tools{Version: current, URL: ""}
	curTools.Version.Minor++
	s.rawMachine.SetAgentVersion(current)
	// Upgrader.Tools returns the *desired* set of tools, not the currently
	// running set. We want to be upgraded to cur.Version
	stateTools, err := s.st.Tools(s.rawMachine.Tag().String())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(stateTools.Version, gc.Equals, current)
	url := fmt.Sprintf("https://%s/model/%s/tools/%s",
		s.stateAPI.Addr(), coretesting.ModelTag.Id(), current)
	c.Assert(stateTools.URL, gc.Equals, url)
}

func (s *machineUpgraderSuite) TestWatchAPIVersion(c *gc.C) {
	w, err := s.st.WatchAPIVersion(s.rawMachine.Tag().String())
	c.Assert(err, jc.ErrorIsNil)
	wc := watchertest.NewNotifyWatcherC(c, w, s.BackingState.StartSync)
	defer wc.AssertStops()

	// Initial event
	wc.AssertOneChange()

	// One change noticing the new version
	vers := version.MustParse("10.20.34")
	err = statetesting.SetAgentVersion(s.BackingState, vers)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Setting the version to the same value doesn't trigger a change
	err = statetesting.SetAgentVersion(s.BackingState, vers)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Another change noticing another new version
	vers = version.MustParse("10.20.35")
	err = statetesting.SetAgentVersion(s.BackingState, vers)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()
}

func (s *machineUpgraderSuite) TestDesiredVersion(c *gc.C) {
	curTools := &tools.Tools{Version: current, URL: ""}
	curTools.Version.Minor++
	s.rawMachine.SetAgentVersion(current)
	// Upgrader.DesiredVersion returns the *desired* set of tools, not the
	// currently running set. We want to be upgraded to cur.Version
	stateVersion, err := s.st.DesiredVersion(s.rawMachine.Tag().String())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(stateVersion, gc.Equals, current.Number)
}
