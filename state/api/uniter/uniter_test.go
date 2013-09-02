// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

// TODO: Possibly split this into multiple *_test.go modules with
// separate suites, because it'll grow quite large.

import (
	stdtesting "testing"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/api/uniter"
	statetesting "launchpad.net/juju-core/state/testing"
	coretesting "launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
)

func TestAll(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type uniterSuite struct {
	testing.JujuConnSuite
	st      *api.State
	machine *state.Machine
	service *state.Service
	charm   *state.Charm
	unit    *state.Unit

	uniter *uniter.State
}

var _ = gc.Suite(&uniterSuite{})

func (s *uniterSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	// Create a machine, a service and add a unit so we can log in as
	// its agent.
	var err error
	s.machine, err = s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	s.charm = s.AddTestingCharm(c, "wordpress")
	s.service, err = s.State.AddService("wordpress", s.charm)
	c.Assert(err, gc.IsNil)
	s.unit, err = s.service.AddUnit()
	c.Assert(err, gc.IsNil)
	err = s.unit.AssignToMachine(s.machine)
	c.Assert(err, gc.IsNil)
	err = s.unit.SetPassword("password")
	c.Assert(err, gc.IsNil)
	s.st = s.OpenAPIAs(c, s.unit.Tag(), "password")

	// Create the uniter API facade.
	s.uniter = s.st.Uniter()
	c.Assert(s.uniter, gc.NotNil)
}

func (s *uniterSuite) TearDownTest(c *gc.C) {
	err := s.st.Close()
	c.Assert(err, gc.IsNil)
	s.JujuConnSuite.TearDownTest(c)
}

func (s *uniterSuite) TestUnitAndUnitTag(c *gc.C) {
	unit, err := s.uniter.Unit("unit-foo-42")
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(params.ErrCode(err), gc.Equals, params.CodeUnauthorized)
	c.Assert(unit, gc.IsNil)

	unit, err = s.uniter.Unit("unit-wordpress-0")
	c.Assert(err, gc.IsNil)
	c.Assert(unit.Tag(), gc.Equals, "unit-wordpress-0")
}

func (s *uniterSuite) TestSetStatus(c *gc.C) {
	unit, err := s.uniter.Unit("unit-wordpress-0")
	c.Assert(err, gc.IsNil)

	status, info, err := s.unit.Status()
	c.Assert(err, gc.IsNil)
	c.Assert(status, gc.Equals, params.StatusPending)
	c.Assert(info, gc.Equals, "")

	err = unit.SetStatus(params.StatusStarted, "blah")
	c.Assert(err, gc.IsNil)

	status, info, err = s.unit.Status()
	c.Assert(err, gc.IsNil)
	c.Assert(status, gc.Equals, params.StatusStarted)
	c.Assert(info, gc.Equals, "blah")
}

func (s *uniterSuite) TestEnsureDead(c *gc.C) {
	c.Assert(s.unit.Life(), gc.Equals, state.Alive)

	unit, err := s.uniter.Unit("unit-wordpress-0")
	c.Assert(err, gc.IsNil)

	err = unit.EnsureDead()
	c.Assert(err, gc.IsNil)

	err = s.unit.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(s.unit.Life(), gc.Equals, state.Dead)

	err = unit.EnsureDead()
	c.Assert(err, gc.IsNil)
	err = s.unit.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(s.unit.Life(), gc.Equals, state.Dead)

	err = s.unit.Remove()
	c.Assert(err, gc.IsNil)
	err = s.unit.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)

	err = unit.EnsureDead()
	c.Assert(err, gc.ErrorMatches, `unit "wordpress/0" not found`)
	c.Assert(params.ErrCode(err), gc.Equals, params.CodeNotFound)
}

func (s *uniterSuite) TestDestroy(c *gc.C) {
	c.Assert(s.unit.Life(), gc.Equals, state.Alive)

	unit, err := s.uniter.Unit("unit-wordpress-0")
	c.Assert(err, gc.IsNil)

	err = unit.Destroy()
	c.Assert(err, gc.IsNil)

	err = s.unit.Refresh()
	c.Assert(err, gc.ErrorMatches, `unit "wordpress/0" not found`)
}

func (s *uniterSuite) TestDestroyAllSubordinates(c *gc.C) {
	c.Assert(s.unit.Life(), gc.Equals, state.Alive)

	unit, err := s.uniter.Unit("unit-wordpress-0")
	c.Assert(err, gc.IsNil)

	// Call without subordinates - no change.
	err = unit.DestroyAllSubordinates()
	c.Assert(err, gc.IsNil)

	// Add a couple of subordinates and try again.
	_, loggingSub := s.addRelatedService(c, "wordpress", "logging", s.unit)
	_, monitoringSub := s.addRelatedService(c, "wordpress", "monitoring", s.unit)
	c.Assert(loggingSub.Life(), gc.Equals, state.Alive)
	c.Assert(monitoringSub.Life(), gc.Equals, state.Alive)

	err = unit.DestroyAllSubordinates()
	c.Assert(err, gc.IsNil)

	// Verify they got destroyed.
	err = loggingSub.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(loggingSub.Life(), gc.Equals, state.Dying)
	err = monitoringSub.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(monitoringSub.Life(), gc.Equals, state.Dying)
}

func (s *uniterSuite) TestRefresh(c *gc.C) {
	unit, err := s.uniter.Unit("unit-wordpress-0")
	c.Assert(err, gc.IsNil)
	c.Assert(unit.Life(), gc.Equals, params.Alive)

	err = unit.EnsureDead()
	c.Assert(err, gc.IsNil)
	c.Assert(unit.Life(), gc.Equals, params.Alive)

	err = unit.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(unit.Life(), gc.Equals, params.Dead)
}

func (s *uniterSuite) TestWatch(c *gc.C) {
	unit, err := s.uniter.Unit("unit-wordpress-0")
	c.Assert(err, gc.IsNil)
	c.Assert(unit.Life(), gc.Equals, params.Alive)

	w, err := unit.Watch()
	c.Assert(err, gc.IsNil)
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewNotifyWatcherC(c, s.BackingState, w)

	// Initial event.
	wc.AssertOneChange()

	// Change something other than the lifecycle and make sure it's
	// not detected.
	err = unit.SetStatus(params.StatusStarted, "not really")
	c.Assert(err, gc.IsNil)
	wc.AssertNoChange()

	// Make the unit dead and check it's detected.
	err = unit.EnsureDead()
	c.Assert(err, gc.IsNil)
	wc.AssertOneChange()

	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *uniterSuite) TestResolve(c *gc.C) {
	unit, err := s.uniter.Unit("unit-wordpress-0")
	c.Assert(err, gc.IsNil)

	err = s.unit.SetResolved(state.ResolvedRetryHooks)
	c.Assert(err, gc.IsNil)

	mode, err := unit.Resolved()
	c.Assert(err, gc.IsNil)
	c.Assert(mode, gc.Equals, params.ResolvedRetryHooks)

	err = unit.ClearResolved()
	c.Assert(err, gc.IsNil)

	mode, err = unit.Resolved()
	c.Assert(err, gc.IsNil)
	c.Assert(mode, gc.Equals, params.ResolvedNone)
}

func (s *uniterSuite) TestIsPrincipal(c *gc.C) {
	unit, err := s.uniter.Unit("unit-wordpress-0")
	c.Assert(err, gc.IsNil)

	ok, err := unit.IsPrincipal()
	c.Assert(err, gc.IsNil)
	c.Assert(ok, jc.IsTrue)
}

func (s *uniterSuite) addRelation(c *gc.C, first, second string) *state.Relation {
	eps, err := s.State.InferEndpoints([]string{first, second})
	c.Assert(err, gc.IsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, gc.IsNil)
	return rel
}

func (s *uniterSuite) addRelatedService(c *gc.C, firstSvc, relatedSvc string, unit *state.Unit) (*state.Service, *state.Unit) {
	relatedService, err := s.State.AddService(relatedSvc, s.AddTestingCharm(c, relatedSvc))
	c.Assert(err, gc.IsNil)
	rel := s.addRelation(c, firstSvc, relatedSvc)
	relUnit, err := rel.Unit(unit)
	c.Assert(err, gc.IsNil)
	err = relUnit.EnterScope(nil)
	c.Assert(err, gc.IsNil)
	relatedUnit, err := relatedService.Unit(relatedSvc + "/0")
	c.Assert(err, gc.IsNil)
	return relatedService, relatedUnit
}

func (s *uniterSuite) TestSubordinateNames(c *gc.C) {
	unit, err := s.uniter.Unit("unit-wordpress-0")
	c.Assert(err, gc.IsNil)

	subordinates, err := unit.SubordinateNames()
	c.Assert(err, gc.IsNil)
	c.Assert(subordinates, gc.HasLen, 0)

	// Add a couple of subordinates and try again.
	s.addRelatedService(c, "wordpress", "logging", s.unit)
	s.addRelatedService(c, "wordpress", "monitoring", s.unit)

	subordinates, err = unit.SubordinateNames()
	c.Assert(err, gc.IsNil)
	c.Assert(subordinates, gc.DeepEquals, []string{"logging/0", "monitoring/0"})
}

func (s *uniterSuite) TestGetSetPublicAddress(c *gc.C) {
	unit, err := s.uniter.Unit("unit-wordpress-0")
	c.Assert(err, gc.IsNil)

	address, ok, err := unit.PublicAddress()
	c.Assert(err, gc.IsNil)
	c.Assert(ok, jc.IsFalse)
	c.Assert(address, gc.Equals, "")

	err = unit.SetPublicAddress("1.2.3.4")
	c.Assert(err, gc.IsNil)

	address, ok, err = unit.PublicAddress()
	c.Assert(err, gc.IsNil)
	c.Assert(ok, jc.IsTrue)
	c.Assert(address, gc.Equals, "1.2.3.4")
}

func (s *uniterSuite) TestGetSetPrivateAddress(c *gc.C) {
	unit, err := s.uniter.Unit("unit-wordpress-0")
	c.Assert(err, gc.IsNil)

	address, ok, err := unit.PrivateAddress()
	c.Assert(err, gc.IsNil)
	c.Assert(ok, jc.IsFalse)
	c.Assert(address, gc.Equals, "")

	err = unit.SetPrivateAddress("1.2.3.4")
	c.Assert(err, gc.IsNil)

	address, ok, err = unit.PrivateAddress()
	c.Assert(err, gc.IsNil)
	c.Assert(ok, jc.IsTrue)
	c.Assert(address, gc.Equals, "1.2.3.4")
}

func (s *uniterSuite) TestOpenClosePort(c *gc.C) {
	unit, err := s.uniter.Unit("unit-wordpress-0")
	c.Assert(err, gc.IsNil)

	ports := s.unit.OpenedPorts()
	c.Assert(ports, gc.HasLen, 0)

	err = unit.OpenPort("foo", 1234)
	c.Assert(err, gc.IsNil)
	err = unit.OpenPort("bar", 4321)
	c.Assert(err, gc.IsNil)

	err = s.unit.Refresh()
	c.Assert(err, gc.IsNil)
	ports = s.unit.OpenedPorts()
	// OpenedPorts returns a sorted slice.
	c.Assert(ports, gc.DeepEquals, []instance.Port{
		{Protocol: "bar", Number: 4321},
		{Protocol: "foo", Number: 1234},
	})

	err = unit.ClosePort("bar", 4321)
	c.Assert(err, gc.IsNil)

	err = s.unit.Refresh()
	c.Assert(err, gc.IsNil)
	ports = s.unit.OpenedPorts()
	// OpenedPorts returns a sorted slice.
	c.Assert(ports, gc.DeepEquals, []instance.Port{
		{Protocol: "foo", Number: 1234},
	})

	err = unit.ClosePort("foo", 1234)
	c.Assert(err, gc.IsNil)

	err = s.unit.Refresh()
	c.Assert(err, gc.IsNil)
	ports = s.unit.OpenedPorts()
	c.Assert(ports, gc.HasLen, 0)
}

func (s *uniterSuite) TestGetSetCharmURL(c *gc.C) {
	unit, err := s.uniter.Unit("unit-wordpress-0")
	c.Assert(err, gc.IsNil)

	// No charm URL set yet.
	curl, ok := s.unit.CharmURL()
	c.Assert(curl, gc.IsNil)
	c.Assert(ok, jc.IsFalse)

	// Now check the same through the API.
	curl, ok, err = unit.CharmURL()
	c.Assert(err, gc.IsNil)
	c.Assert(curl, gc.IsNil)
	c.Assert(ok, jc.IsFalse)

	err = unit.SetCharmURL(s.charm.URL())
	c.Assert(err, gc.IsNil)

	curl, ok, err = unit.CharmURL()
	c.Assert(err, gc.IsNil)
	c.Assert(curl, gc.NotNil)
	c.Assert(curl.String(), gc.Equals, s.charm.String())
	c.Assert(ok, jc.IsTrue)
}

func (s *uniterSuite) TestConfigSettings(c *gc.C) {
	unit, err := s.uniter.Unit("unit-wordpress-0")
	c.Assert(err, gc.IsNil)

	// Make sure ConfigSettings returns an error when
	// no charm URL is set, as its state counterpart does.
	settings, err := unit.ConfigSettings()
	c.Assert(err, gc.ErrorMatches, "unit charm not set")

	// Now set the charm and try again.
	err = unit.SetCharmURL(s.charm.URL())
	c.Assert(err, gc.IsNil)

	settings, err = unit.ConfigSettings()
	c.Assert(err, gc.IsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{
		"blog-title": "My Title",
	})

	// Update the config and check we get the changes on the next call.
	err = s.service.UpdateConfigSettings(charm.Settings{
		"blog-title": "superhero paparazzi",
	})
	c.Assert(err, gc.IsNil)

	settings, err = unit.ConfigSettings()
	c.Assert(err, gc.IsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{
		"blog-title": "superhero paparazzi",
	})
}

func (s *uniterSuite) TestWatchConfigSettings(c *gc.C) {
	unit, err := s.uniter.Unit("unit-wordpress-0")
	c.Assert(err, gc.IsNil)
	c.Assert(unit.Life(), gc.Equals, params.Alive)

	// Make sure WatchConfigSettings returns an error when
	// no charm URL is set, as its state counterpart does.
	w, err := unit.WatchConfigSettings()
	c.Assert(err, gc.ErrorMatches, "unit charm not set")

	// Now set the charm and try again.
	err = unit.SetCharmURL(s.charm.URL())
	c.Assert(err, gc.IsNil)

	w, err = unit.WatchConfigSettings()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewNotifyWatcherC(c, s.BackingState, w)

	// Initial event.
	wc.AssertOneChange()

	// Update config a couple of times, check a single event.
	err = s.service.UpdateConfigSettings(charm.Settings{
		"blog-title": "superhero paparazzi",
	})
	c.Assert(err, gc.IsNil)
	err = s.service.UpdateConfigSettings(charm.Settings{
		"blog-title": "sauceror central",
	})
	c.Assert(err, gc.IsNil)
	wc.AssertOneChange()

	// Non-change is not reported.
	err = s.service.UpdateConfigSettings(charm.Settings{
		"blog-title": "sauceror central",
	})
	c.Assert(err, gc.IsNil)
	wc.AssertNoChange()

	// NOTE: This test is not as exhaustive as the one in state,
	// because the watcher is already tested there. Here we just
	// ensure we get the events when we expect them and don't get
	// them when they're not expected.

	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}
