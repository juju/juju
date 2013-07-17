// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"strconv"
	"time"

	. "launchpad.net/gocheck"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/testing"
	"launchpad.net/juju-core/testing/checkers"
)

type UnitSuite struct {
	ConnSuite
	charm   *state.Charm
	service *state.Service
	unit    *state.Unit
}

var _ = Suite(&UnitSuite{})

func (s *UnitSuite) SetUpTest(c *C) {
	s.ConnSuite.SetUpTest(c)
	s.charm = s.AddTestingCharm(c, "wordpress")
	var err error
	s.service, err = s.State.AddService("wordpress", s.charm)
	c.Assert(err, IsNil)
	s.unit, err = s.service.AddUnit()
	c.Assert(err, IsNil)
	c.Assert(s.unit.Series(), Equals, "series")
}

func (s *UnitSuite) TestUnitNotFound(c *C) {
	_, err := s.State.Unit("subway/0")
	c.Assert(err, ErrorMatches, `unit "subway/0" not found`)
	c.Assert(err, checkers.Satisfies, errors.IsNotFoundError)
}

func (s *UnitSuite) TestUnitNameFromTag(c *C) {
	// Try both valid and invalid tag formats.
	c.Assert(state.UnitNameFromTag("unit-wordpress-0"), Equals, "wordpress/0")
	c.Assert(state.UnitNameFromTag("foo"), Equals, "")
}

func (s *UnitSuite) TestService(c *C) {
	svc, err := s.unit.Service()
	c.Assert(err, IsNil)
	c.Assert(svc.Name(), Equals, s.unit.ServiceName())
}

func (s *UnitSuite) TestConfigSettingsNeedCharmURLSet(c *C) {
	_, err := s.unit.ConfigSettings()
	c.Assert(err, ErrorMatches, "unit charm not set")
}

func (s *UnitSuite) TestConfigSettingsIncludeDefaults(c *C) {
	err := s.unit.SetCharmURL(s.charm.URL())
	c.Assert(err, IsNil)
	settings, err := s.unit.ConfigSettings()
	c.Assert(err, IsNil)
	c.Assert(settings, DeepEquals, charm.Settings{"blog-title": "My Title"})
}

func (s *UnitSuite) TestConfigSettingsReflectService(c *C) {
	err := s.service.UpdateConfigSettings(charm.Settings{"blog-title": "no title"})
	c.Assert(err, IsNil)
	err = s.unit.SetCharmURL(s.charm.URL())
	c.Assert(err, IsNil)
	settings, err := s.unit.ConfigSettings()
	c.Assert(err, IsNil)
	c.Assert(settings, DeepEquals, charm.Settings{"blog-title": "no title"})

	err = s.service.UpdateConfigSettings(charm.Settings{"blog-title": "ironic title"})
	c.Assert(err, IsNil)
	settings, err = s.unit.ConfigSettings()
	c.Assert(err, IsNil)
	c.Assert(settings, DeepEquals, charm.Settings{"blog-title": "ironic title"})
}

func (s *UnitSuite) TestConfigSettingsReflectCharm(c *C) {
	err := s.unit.SetCharmURL(s.charm.URL())
	c.Assert(err, IsNil)
	newCharm := s.AddConfigCharm(c, "wordpress", "options: {}", 123)
	err = s.service.SetCharm(newCharm, false)
	c.Assert(err, IsNil)

	// Settings still reflect charm set on unit.
	settings, err := s.unit.ConfigSettings()
	c.Assert(err, IsNil)
	c.Assert(settings, DeepEquals, charm.Settings{"blog-title": "My Title"})

	// When the unit has the new charm set, it'll see the new config.
	err = s.unit.SetCharmURL(newCharm.URL())
	c.Assert(err, IsNil)
	settings, err = s.unit.ConfigSettings()
	c.Assert(err, IsNil)
	c.Assert(settings, DeepEquals, charm.Settings{})
}

func (s *UnitSuite) TestWatchConfigSettingsNeedsCharmURL(c *C) {
	_, err := s.unit.WatchConfigSettings()
	c.Assert(err, ErrorMatches, "unit charm not set")
}

func (s *UnitSuite) TestWatchConfigSettings(c *C) {
	err := s.unit.SetCharmURL(s.charm.URL())
	c.Assert(err, IsNil)
	w, err := s.unit.WatchConfigSettings()
	c.Assert(err, IsNil)
	defer testing.AssertStop(c, w)

	// Initial event.
	wc := testing.NewNotifyWatcherC(c, s.State, w)
	wc.AssertOneChange()

	// Update config a couple of times, check a single event.
	err = s.service.UpdateConfigSettings(charm.Settings{
		"blog-title": "superhero paparazzi",
	})
	c.Assert(err, IsNil)
	err = s.service.UpdateConfigSettings(charm.Settings{
		"blog-title": "sauceror central",
	})
	c.Assert(err, IsNil)
	wc.AssertOneChange()

	// Non-change is not reported.
	err = s.service.UpdateConfigSettings(charm.Settings{
		"blog-title": "sauceror central",
	})
	c.Assert(err, IsNil)
	wc.AssertNoChange()

	// Change service's charm; nothing detected.
	newCharm := s.AddConfigCharm(c, "wordpress", floatConfig, 123)
	err = s.service.SetCharm(newCharm, false)
	c.Assert(err, IsNil)
	wc.AssertNoChange()

	// Change service config for new charm; nothing detected.
	err = s.service.UpdateConfigSettings(charm.Settings{
		"key": 42.0,
	})
	c.Assert(err, IsNil)
	wc.AssertNoChange()

	// NOTE: if we were to change the unit to use the new charm, we'd see
	// another event, because the originally-watched document will become
	// unreferenced and be removed. But I'm not testing that behaviour
	// because it's not very helpful and subject to change.
}

func (s *UnitSuite) TestGetSetPublicAddress(c *C) {
	_, ok := s.unit.PublicAddress()
	c.Assert(ok, Equals, false)

	err := s.unit.SetPublicAddress("example.foobar.com")
	c.Assert(err, IsNil)
	address, ok := s.unit.PublicAddress()
	c.Assert(ok, Equals, true)
	c.Assert(address, Equals, "example.foobar.com")

	defer state.SetBeforeHooks(c, s.State, func() {
		c.Assert(s.unit.Destroy(), IsNil)
	}).Check()
	err = s.unit.SetPublicAddress("example.foobar.com")
	c.Assert(err, ErrorMatches, `cannot set public address of unit "wordpress/0": unit not found`)
}

func (s *UnitSuite) TestGetSetPrivateAddress(c *C) {
	_, ok := s.unit.PrivateAddress()
	c.Assert(ok, Equals, false)

	err := s.unit.SetPrivateAddress("example.local")
	c.Assert(err, IsNil)
	address, ok := s.unit.PrivateAddress()
	c.Assert(ok, Equals, true)
	c.Assert(address, Equals, "example.local")

	defer state.SetBeforeHooks(c, s.State, func() {
		c.Assert(s.unit.Destroy(), IsNil)
	}).Check()
	err = s.unit.SetPrivateAddress("example.local")
	c.Assert(err, ErrorMatches, `cannot set private address of unit "wordpress/0": unit not found`)
}

func (s *UnitSuite) TestRefresh(c *C) {
	unit1, err := s.State.Unit(s.unit.Name())
	c.Assert(err, IsNil)

	err = s.unit.SetPrivateAddress("example.local")
	c.Assert(err, IsNil)
	err = s.unit.SetPublicAddress("example.foobar.com")
	c.Assert(err, IsNil)

	address, ok := unit1.PrivateAddress()
	c.Assert(ok, Equals, false)
	address, ok = unit1.PublicAddress()
	c.Assert(ok, Equals, false)

	err = unit1.Refresh()
	c.Assert(err, IsNil)
	address, ok = unit1.PrivateAddress()
	c.Assert(ok, Equals, true)
	c.Assert(address, Equals, "example.local")
	address, ok = unit1.PublicAddress()
	c.Assert(ok, Equals, true)
	c.Assert(address, Equals, "example.foobar.com")

	err = unit1.EnsureDead()
	c.Assert(err, IsNil)
	err = unit1.Remove()
	c.Assert(err, IsNil)
	err = unit1.Refresh()
	c.Assert(err, checkers.Satisfies, errors.IsNotFoundError)
}

func (s *UnitSuite) TestGetSetStatusWhileAlive(c *C) {
	fail := func() { s.unit.SetStatus(params.StatusError, "") }
	c.Assert(fail, PanicMatches, "unit error status with no info")

	status, info, err := s.unit.Status()
	c.Assert(err, IsNil)
	c.Assert(status, Equals, params.StatusPending)
	c.Assert(info, Equals, "")

	err = s.unit.SetStatus(params.StatusStarted, "")
	c.Assert(err, IsNil)
	status, info, err = s.unit.Status()
	c.Assert(err, IsNil)
	c.Assert(status, Equals, params.StatusStarted)
	c.Assert(info, Equals, "")

	err = s.unit.SetStatus(params.StatusError, "test-hook failed")
	c.Assert(err, IsNil)
	status, info, err = s.unit.Status()
	c.Assert(err, IsNil)
	c.Assert(status, Equals, params.StatusError)
	c.Assert(info, Equals, "test-hook failed")

	err = s.unit.SetStatus(params.StatusPending, "deploying...")
	c.Assert(err, IsNil)
	status, info, err = s.unit.Status()
	c.Assert(err, IsNil)
	c.Assert(status, Equals, params.StatusPending)
	c.Assert(info, Equals, "deploying...")
}

func (s *UnitSuite) TestGetSetStatusWhileNotAlive(c *C) {
	err := s.unit.Destroy()
	c.Assert(err, IsNil)
	err = s.unit.SetStatus(params.StatusStarted, "not really")
	c.Assert(err, ErrorMatches, `cannot set status of unit "wordpress/0": not found or dead`)
	_, _, err = s.unit.Status()
	c.Assert(err, ErrorMatches, "status not found")

	err = s.unit.EnsureDead()
	c.Assert(err, IsNil)
	err = s.unit.SetStatus(params.StatusStarted, "not really")
	c.Assert(err, ErrorMatches, `cannot set status of unit "wordpress/0": not found or dead`)
	_, _, err = s.unit.Status()
	c.Assert(err, ErrorMatches, "status not found")
}

func (s *UnitSuite) TestUnitCharm(c *C) {
	preventUnitDestroyRemove(c, s.unit)
	curl, ok := s.unit.CharmURL()
	c.Assert(ok, Equals, false)
	c.Assert(curl, IsNil)

	err := s.unit.SetCharmURL(nil)
	c.Assert(err, ErrorMatches, "cannot set nil charm url")

	err = s.unit.SetCharmURL(charm.MustParseURL("cs:missing/one-1"))
	c.Assert(err, ErrorMatches, `unknown charm url "cs:missing/one-1"`)

	err = s.unit.SetCharmURL(s.charm.URL())
	c.Assert(err, IsNil)
	curl, ok = s.unit.CharmURL()
	c.Assert(ok, Equals, true)
	c.Assert(curl, DeepEquals, s.charm.URL())

	err = s.unit.Destroy()
	c.Assert(err, IsNil)
	err = s.unit.SetCharmURL(s.charm.URL())
	c.Assert(err, IsNil)
	curl, ok = s.unit.CharmURL()
	c.Assert(ok, Equals, true)
	c.Assert(curl, DeepEquals, s.charm.URL())

	err = s.unit.EnsureDead()
	c.Assert(err, IsNil)
	err = s.unit.SetCharmURL(s.charm.URL())
	c.Assert(err, ErrorMatches, `unit "wordpress/0" is dead`)
}

func (s *UnitSuite) TestDestroyPrincipalUnits(c *C) {
	preventUnitDestroyRemove(c, s.unit)
	for i := 0; i < 4; i++ {
		unit, err := s.service.AddUnit()
		c.Assert(err, IsNil)
		preventUnitDestroyRemove(c, unit)
	}

	// Destroy 2 of them; check they become Dying.
	err := s.State.DestroyUnits("wordpress/0", "wordpress/1")
	c.Assert(err, IsNil)
	s.assertUnitLife(c, "wordpress/0", state.Dying)
	s.assertUnitLife(c, "wordpress/1", state.Dying)

	// Try to destroy an Alive one and a Dying one; check
	// it destroys the Alive one and ignores the Dying one.
	err = s.State.DestroyUnits("wordpress/2", "wordpress/0")
	c.Assert(err, IsNil)
	s.assertUnitLife(c, "wordpress/2", state.Dying)

	// Try to destroy an Alive one along with a nonexistent one; check that
	// the valid instruction is followed but the invalid one is warned about.
	err = s.State.DestroyUnits("boojum/123", "wordpress/3")
	c.Assert(err, ErrorMatches, `some units were not destroyed: unit "boojum/123" does not exist`)
	s.assertUnitLife(c, "wordpress/3", state.Dying)

	// Make one Dead, and destroy an Alive one alongside it; check no errors.
	wp0, err := s.State.Unit("wordpress/0")
	c.Assert(err, IsNil)
	err = wp0.EnsureDead()
	c.Assert(err, IsNil)
	err = s.State.DestroyUnits("wordpress/0", "wordpress/4")
	c.Assert(err, IsNil)
	s.assertUnitLife(c, "wordpress/0", state.Dead)
	s.assertUnitLife(c, "wordpress/4", state.Dying)
}

func (s *UnitSuite) TestDestroySetStatusRetry(c *C) {
	defer state.SetRetryHooks(c, s.State, func() {
		err := s.unit.SetStatus(params.StatusStarted, "")
		c.Assert(err, IsNil)
	}, func() {
		assertUnitLife(c, s.unit, state.Dying)
	}).Check()
	err := s.unit.Destroy()
	c.Assert(err, IsNil)
}

func (s *UnitSuite) TestDestroySetCharmRetry(c *C) {
	defer state.SetRetryHooks(c, s.State, func() {
		err := s.unit.SetCharmURL(s.charm.URL())
		c.Assert(err, IsNil)
	}, func() {
		assertUnitRemoved(c, s.unit)
	}).Check()
	err := s.unit.Destroy()
	c.Assert(err, IsNil)
}

func (s *UnitSuite) TestDestroyChangeCharmRetry(c *C) {
	err := s.unit.SetCharmURL(s.charm.URL())
	c.Assert(err, IsNil)
	newCharm := s.AddConfigCharm(c, "mysql", "options: {}", 99)
	err = s.service.SetCharm(newCharm, false)
	c.Assert(err, IsNil)

	defer state.SetRetryHooks(c, s.State, func() {
		err := s.unit.SetCharmURL(newCharm.URL())
		c.Assert(err, IsNil)
	}, func() {
		assertUnitRemoved(c, s.unit)
	}).Check()
	err = s.unit.Destroy()
	c.Assert(err, IsNil)
}

func (s *UnitSuite) TestDestroyAssignRetry(c *C) {
	machine, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)

	defer state.SetRetryHooks(c, s.State, func() {
		err := s.unit.AssignToMachine(machine)
		c.Assert(err, IsNil)
	}, func() {
		assertUnitRemoved(c, s.unit)
		// Also check the unit ref was properly removed from the machine doc --
		// if it weren't, we wouldn't be able to make the machine Dead.
		err := machine.EnsureDead()
		c.Assert(err, IsNil)
	}).Check()
	err = s.unit.Destroy()
	c.Assert(err, IsNil)
}

func (s *UnitSuite) TestDestroyUnassignRetry(c *C) {
	machine, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	err = s.unit.AssignToMachine(machine)
	c.Assert(err, IsNil)

	defer state.SetRetryHooks(c, s.State, func() {
		err := s.unit.UnassignFromMachine()
		c.Assert(err, IsNil)
	}, func() {
		assertUnitRemoved(c, s.unit)
	}).Check()
	err = s.unit.Destroy()
	c.Assert(err, IsNil)
}

func (s *UnitSuite) TestShortCircuitDestroyUnit(c *C) {
	// A unit that has not set any status is removed directly.
	err := s.unit.Destroy()
	c.Assert(err, IsNil)
	c.Assert(s.unit.Life(), Equals, state.Dying)
	assertUnitRemoved(c, s.unit)
}

func (s *UnitSuite) TestCannotShortCircuitDestroyWithSubordinates(c *C) {
	// A unit with subordinates is just set to Dying.
	_, err := s.State.AddService("logging", s.AddTestingCharm(c, "logging"))
	c.Assert(err, IsNil)
	eps, err := s.State.InferEndpoints([]string{"logging", "wordpress"})
	c.Assert(err, IsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, IsNil)
	ru, err := rel.Unit(s.unit)
	c.Assert(err, IsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, IsNil)
	err = s.unit.Destroy()
	c.Assert(err, IsNil)
	c.Assert(s.unit.Life(), Equals, state.Dying)
	assertUnitLife(c, s.unit, state.Dying)
}

func (s *UnitSuite) TestCannotShortCircuitDestroyWithStatus(c *C) {
	for i, test := range []struct {
		status params.Status
		info   string
	}{{
		params.StatusInstalled, "",
	}, {
		params.StatusStarted, "",
	}, {
		params.StatusError, "blah",
	}, {
		params.StatusStopped, "",
	}} {
		c.Logf("test %d: %s", i, test.status)
		unit, err := s.service.AddUnit()
		c.Assert(err, IsNil)
		err = unit.SetStatus(test.status, test.info)
		c.Assert(err, IsNil)
		err = unit.Destroy()
		c.Assert(err, IsNil)
		c.Assert(unit.Life(), Equals, state.Dying)
		assertUnitLife(c, unit, state.Dying)
	}
}

func (s *UnitSuite) TestShortCircuitDestroyWithProvisionedMachine(c *C) {
	// A unit assigned to a provisioned machine is still removed directly so
	// long as it has not set status.
	err := s.unit.AssignToNewMachine()
	c.Assert(err, IsNil)
	mid, err := s.unit.AssignedMachineId()
	c.Assert(err, IsNil)
	machine, err := s.State.Machine(mid)
	c.Assert(err, IsNil)
	err = machine.SetProvisioned("i-malive", "fake_nonce", nil)
	c.Assert(err, IsNil)
	err = s.unit.Destroy()
	c.Assert(err, IsNil)
	c.Assert(s.unit.Life(), Equals, state.Dying)
	assertUnitRemoved(c, s.unit)
}

func (s *UnitSuite) TestDestroySubordinateUnits(c *C) {
	lgsch := s.AddTestingCharm(c, "logging")
	_, err := s.State.AddService("logging", lgsch)
	c.Assert(err, IsNil)
	eps, err := s.State.InferEndpoints([]string{"logging", "wordpress"})
	c.Assert(err, IsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, IsNil)
	ru, err := rel.Unit(s.unit)
	c.Assert(err, IsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, IsNil)

	// Try to destroy the subordinate alone; check it fails.
	err = s.State.DestroyUnits("logging/0")
	c.Assert(err, ErrorMatches, `no units were destroyed: unit "logging/0" is a subordinate`)
	s.assertUnitLife(c, "logging/0", state.Alive)

	// Try to destroy the principal and the subordinate together; check it warns
	// about the subordinate, but destroys the one it can. (The principal unit
	// agent will be resposible for destroying the subordinate.)
	err = s.State.DestroyUnits("wordpress/0", "logging/0")
	c.Assert(err, ErrorMatches, `some units were not destroyed: unit "logging/0" is a subordinate`)
	s.assertUnitLife(c, "wordpress/0", state.Dying)
	s.assertUnitLife(c, "logging/0", state.Alive)
}

func (s *UnitSuite) assertUnitLife(c *C, name string, life state.Life) {
	unit, err := s.State.Unit(name)
	c.Assert(err, IsNil)
	assertUnitLife(c, unit, life)
}

func assertUnitLife(c *C, unit *state.Unit, life state.Life) {
	c.Assert(unit.Refresh(), IsNil)
	c.Assert(unit.Life(), Equals, life)
}

func assertUnitRemoved(c *C, unit *state.Unit) {
	err := unit.Refresh()
	c.Assert(err, checkers.Satisfies, errors.IsNotFoundError)
	err = unit.Destroy()
	c.Assert(err, IsNil)
	err = unit.EnsureDead()
	c.Assert(err, IsNil)
	err = unit.Remove()
	c.Assert(err, IsNil)
}

func (s *UnitSuite) TestTag(c *C) {
	c.Assert(s.unit.Tag(), Equals, "unit-wordpress-0")
}

func (s *UnitSuite) TestUnitTag(c *C) {
	c.Assert(state.UnitTag("wordpress/2"), Equals, "unit-wordpress-2")
}

func (s *UnitSuite) TestSetMongoPassword(c *C) {
	testSetMongoPassword(c, func(st *state.State) (entity, error) {
		return st.Unit(s.unit.Name())
	})
}

func (s *UnitSuite) TestSetPassword(c *C) {
	preventUnitDestroyRemove(c, s.unit)
	testSetPassword(c, func() (state.Authenticator, error) {
		return s.State.Unit(s.unit.Name())
	})
}

func (s *UnitSuite) TestSetMongoPasswordOnUnitAfterConnectingAsMachineEntity(c *C) {
	// Make a second unit to use later. (Subordinate units can only be created
	// as a side-effect of a principal entering relation scope.)
	subCharm := s.AddTestingCharm(c, "logging")
	_, err := s.State.AddService("logging", subCharm)
	c.Assert(err, IsNil)
	eps, err := s.State.InferEndpoints([]string{"wordpress", "logging"})
	c.Assert(err, IsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, IsNil)
	ru, err := rel.Unit(s.unit)
	c.Assert(err, IsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, IsNil)
	subUnit, err := s.State.Unit("logging/0")
	c.Assert(err, IsNil)

	info := state.TestingStateInfo()
	st, err := state.Open(info, state.TestingDialOpts())
	c.Assert(err, IsNil)
	defer st.Close()
	// Turn on fully-authenticated mode.
	err = st.SetAdminMongoPassword("admin-secret")
	c.Assert(err, IsNil)

	// Add a new machine, assign the units to it
	// and set its password.
	m, err := st.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	unit, err := st.Unit(s.unit.Name())
	c.Assert(err, IsNil)
	subUnit, err = st.Unit(subUnit.Name())
	c.Assert(err, IsNil)
	err = unit.AssignToMachine(m)
	c.Assert(err, IsNil)
	err = m.SetMongoPassword("foo")
	c.Assert(err, IsNil)

	// Sanity check that we cannot connect with the wrong
	// password
	info.Tag = m.Tag()
	info.Password = "foo1"
	err = tryOpenState(info)
	c.Assert(err, checkers.Satisfies, errors.IsUnauthorizedError)

	// Connect as the machine entity.
	info.Tag = m.Tag()
	info.Password = "foo"
	st1, err := state.Open(info, state.TestingDialOpts())
	c.Assert(err, IsNil)
	defer st1.Close()

	// Change the password for a unit derived from
	// the machine entity's state.
	unit, err = st1.Unit(s.unit.Name())
	c.Assert(err, IsNil)
	err = unit.SetMongoPassword("bar")
	c.Assert(err, IsNil)

	// Now connect as the unit entity and, as that
	// that entity, change the password for a new unit.
	info.Tag = unit.Tag()
	info.Password = "bar"
	st2, err := state.Open(info, state.TestingDialOpts())
	c.Assert(err, IsNil)
	defer st2.Close()

	// Check that we can set its password.
	unit, err = st2.Unit(subUnit.Name())
	c.Assert(err, IsNil)
	err = unit.SetMongoPassword("bar2")
	c.Assert(err, IsNil)

	// Clear the admin password, so tests can reset the db.
	err = st.SetAdminMongoPassword("")
	c.Assert(err, IsNil)
}

func (s *UnitSuite) TestUnitSetAgentAlive(c *C) {
	alive, err := s.unit.AgentAlive()
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, false)

	pinger, err := s.unit.SetAgentAlive()
	c.Assert(err, IsNil)
	c.Assert(pinger, NotNil)
	defer pinger.Stop()

	s.State.Sync()
	alive, err = s.unit.AgentAlive()
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, true)
}

func (s *UnitSuite) TestUnitWaitAgentAlive(c *C) {
	timeout := 200 * time.Millisecond
	alive, err := s.unit.AgentAlive()
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, false)

	err = s.unit.WaitAgentAlive(timeout)
	c.Assert(err, ErrorMatches, `waiting for agent of unit "wordpress/0": still not alive after timeout`)

	pinger, err := s.unit.SetAgentAlive()
	c.Assert(err, IsNil)

	s.State.StartSync()
	err = s.unit.WaitAgentAlive(timeout)
	c.Assert(err, IsNil)

	alive, err = s.unit.AgentAlive()
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, true)

	err = pinger.Kill()
	c.Assert(err, IsNil)

	s.State.Sync()
	alive, err = s.unit.AgentAlive()
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, false)
}

func (s *UnitSuite) TestResolve(c *C) {
	err := s.unit.Resolve(false)
	c.Assert(err, ErrorMatches, `unit "wordpress/0" is not in an error state`)
	err = s.unit.Resolve(true)
	c.Assert(err, ErrorMatches, `unit "wordpress/0" is not in an error state`)

	err = s.unit.SetStatus(params.StatusError, "gaaah")
	c.Assert(err, IsNil)
	err = s.unit.Resolve(false)
	c.Assert(err, IsNil)
	err = s.unit.Resolve(true)
	c.Assert(err, ErrorMatches, `cannot set resolved mode for unit "wordpress/0": already resolved`)
	c.Assert(s.unit.Resolved(), Equals, state.ResolvedNoHooks)

	err = s.unit.ClearResolved()
	c.Assert(err, IsNil)
	err = s.unit.Resolve(true)
	c.Assert(err, IsNil)
	err = s.unit.Resolve(false)
	c.Assert(err, ErrorMatches, `cannot set resolved mode for unit "wordpress/0": already resolved`)
	c.Assert(s.unit.Resolved(), Equals, state.ResolvedRetryHooks)
}

func (s *UnitSuite) TestGetSetClearResolved(c *C) {
	mode := s.unit.Resolved()
	c.Assert(mode, Equals, state.ResolvedNone)

	err := s.unit.SetResolved(state.ResolvedNoHooks)
	c.Assert(err, IsNil)
	err = s.unit.SetResolved(state.ResolvedNoHooks)
	c.Assert(err, ErrorMatches, `cannot set resolved mode for unit "wordpress/0": already resolved`)

	mode = s.unit.Resolved()
	c.Assert(mode, Equals, state.ResolvedNoHooks)
	err = s.unit.Refresh()
	c.Assert(err, IsNil)
	mode = s.unit.Resolved()
	c.Assert(mode, Equals, state.ResolvedNoHooks)

	err = s.unit.ClearResolved()
	c.Assert(err, IsNil)
	mode = s.unit.Resolved()
	c.Assert(mode, Equals, state.ResolvedNone)
	err = s.unit.Refresh()
	c.Assert(err, IsNil)
	mode = s.unit.Resolved()
	c.Assert(mode, Equals, state.ResolvedNone)
	err = s.unit.ClearResolved()
	c.Assert(err, IsNil)

	err = s.unit.SetResolved(state.ResolvedNone)
	c.Assert(err, ErrorMatches, `cannot set resolved mode for unit "wordpress/0": invalid error resolution mode: ""`)
	err = s.unit.SetResolved(state.ResolvedMode("foo"))
	c.Assert(err, ErrorMatches, `cannot set resolved mode for unit "wordpress/0": invalid error resolution mode: "foo"`)
}

func (s *UnitSuite) TestOpenedPorts(c *C) {
	// Verify no open ports before activity.
	c.Assert(s.unit.OpenedPorts(), HasLen, 0)

	// Now open and close port.
	err := s.unit.OpenPort("tcp", 80)
	c.Assert(err, IsNil)
	open := s.unit.OpenedPorts()
	c.Assert(open, DeepEquals, []instance.Port{
		{"tcp", 80},
	})

	err = s.unit.OpenPort("udp", 53)
	c.Assert(err, IsNil)
	open = s.unit.OpenedPorts()
	c.Assert(open, DeepEquals, []instance.Port{
		{"tcp", 80},
		{"udp", 53},
	})

	err = s.unit.OpenPort("tcp", 53)
	c.Assert(err, IsNil)
	open = s.unit.OpenedPorts()
	c.Assert(open, DeepEquals, []instance.Port{
		{"tcp", 53},
		{"tcp", 80},
		{"udp", 53},
	})

	err = s.unit.OpenPort("tcp", 443)
	c.Assert(err, IsNil)
	open = s.unit.OpenedPorts()
	c.Assert(open, DeepEquals, []instance.Port{
		{"tcp", 53},
		{"tcp", 80},
		{"tcp", 443},
		{"udp", 53},
	})

	err = s.unit.ClosePort("tcp", 80)
	c.Assert(err, IsNil)
	open = s.unit.OpenedPorts()
	c.Assert(open, DeepEquals, []instance.Port{
		{"tcp", 53},
		{"tcp", 443},
		{"udp", 53},
	})

	err = s.unit.ClosePort("tcp", 80)
	c.Assert(err, IsNil)
	open = s.unit.OpenedPorts()
	c.Assert(open, DeepEquals, []instance.Port{
		{"tcp", 53},
		{"tcp", 443},
		{"udp", 53},
	})
}

func (s *UnitSuite) TestOpenClosePortWhenDying(c *C) {
	preventUnitDestroyRemove(c, s.unit)
	testWhenDying(c, s.unit, noErr, deadErr, func() error {
		return s.unit.OpenPort("tcp", 20)
	}, func() error {
		return s.unit.ClosePort("tcp", 20)
	})
}

func (s *UnitSuite) TestSetClearResolvedWhenNotAlive(c *C) {
	preventUnitDestroyRemove(c, s.unit)
	err := s.unit.Destroy()
	c.Assert(err, IsNil)
	err = s.unit.SetResolved(state.ResolvedNoHooks)
	c.Assert(err, IsNil)
	err = s.unit.Refresh()
	c.Assert(err, IsNil)
	c.Assert(s.unit.Resolved(), Equals, state.ResolvedNoHooks)
	err = s.unit.ClearResolved()
	c.Assert(err, IsNil)

	err = s.unit.EnsureDead()
	c.Assert(err, IsNil)
	err = s.unit.SetResolved(state.ResolvedRetryHooks)
	c.Assert(err, ErrorMatches, deadErr)
	err = s.unit.ClearResolved()
	c.Assert(err, IsNil)
}

func (s *UnitSuite) TestSubordinateChangeInPrincipal(c *C) {
	subCharm := s.AddTestingCharm(c, "logging")
	for i := 0; i < 2; i++ {
		// Note: subordinate units can only be created as a side effect of a
		// principal entering scope; and a given principal can only have a
		// single subordinate unit of each service.
		name := "logging" + strconv.Itoa(i)
		_, err := s.State.AddService(name, subCharm)
		c.Assert(err, IsNil)
		eps, err := s.State.InferEndpoints([]string{name, "wordpress"})
		c.Assert(err, IsNil)
		rel, err := s.State.AddRelation(eps...)
		c.Assert(err, IsNil)
		ru, err := rel.Unit(s.unit)
		c.Assert(err, IsNil)
		err = ru.EnterScope(nil)
		c.Assert(err, IsNil)
	}

	err := s.unit.Refresh()
	c.Assert(err, IsNil)
	subordinates := s.unit.SubordinateNames()
	c.Assert(subordinates, DeepEquals, []string{"logging0/0", "logging1/0"})

	su1, err := s.State.Unit("logging1/0")
	c.Assert(err, IsNil)
	err = su1.EnsureDead()
	c.Assert(err, IsNil)
	err = su1.Remove()
	c.Assert(err, IsNil)
	err = s.unit.Refresh()
	c.Assert(err, IsNil)
	subordinates = s.unit.SubordinateNames()
	c.Assert(subordinates, DeepEquals, []string{"logging0/0"})
}

func (s *UnitSuite) TestDeathWithSubordinates(c *C) {
	// Check that units can become dead when they've never had subordinates.
	u, err := s.service.AddUnit()
	c.Assert(err, IsNil)
	err = u.EnsureDead()
	c.Assert(err, IsNil)

	// Create a new unit and add a subordinate.
	u, err = s.service.AddUnit()
	c.Assert(err, IsNil)
	_, err = s.State.AddService("logging", s.AddTestingCharm(c, "logging"))
	c.Assert(err, IsNil)
	eps, err := s.State.InferEndpoints([]string{"logging", "wordpress"})
	c.Assert(err, IsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, IsNil)
	ru, err := rel.Unit(u)
	c.Assert(err, IsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, IsNil)

	// Check the unit cannot become Dead, but can become Dying...
	err = u.EnsureDead()
	c.Assert(err, Equals, state.ErrUnitHasSubordinates)
	err = u.Destroy()
	c.Assert(err, IsNil)

	// ...and that it still can't become Dead now it's Dying.
	err = u.EnsureDead()
	c.Assert(err, Equals, state.ErrUnitHasSubordinates)

	// Make the subordinate Dead and check the principal still cannot be removed.
	sub, err := s.State.Unit("logging/0")
	c.Assert(err, IsNil)
	err = sub.EnsureDead()
	c.Assert(err, IsNil)
	err = u.EnsureDead()
	c.Assert(err, Equals, state.ErrUnitHasSubordinates)

	// remove the subordinate and check the principal can finally become Dead.
	err = sub.Remove()
	c.Assert(err, IsNil)
	err = u.EnsureDead()
	c.Assert(err, IsNil)
}

func (s *UnitSuite) TestRemove(c *C) {
	err := s.unit.Remove()
	c.Assert(err, ErrorMatches, `cannot remove unit "wordpress/0": unit is not dead`)
	err = s.unit.EnsureDead()
	c.Assert(err, IsNil)
	err = s.unit.Remove()
	c.Assert(err, IsNil)
	err = s.unit.Refresh()
	c.Assert(err, checkers.Satisfies, errors.IsNotFoundError)
	units, err := s.service.AllUnits()
	c.Assert(err, IsNil)
	c.Assert(units, HasLen, 0)
	err = s.unit.Remove()
	c.Assert(err, IsNil)
}

func (s *UnitSuite) TestRemovePathological(c *C) {
	// Add a relation between wordpress and mysql...
	wordpress := s.service
	wordpress0 := s.unit
	mysql, err := s.State.AddService("mysql", s.AddTestingCharm(c, "mysql"))
	c.Assert(err, IsNil)
	eps, err := s.State.InferEndpoints([]string{"wordpress", "mysql"})
	c.Assert(err, IsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, IsNil)

	// The relation holds a reference to wordpress, but that can't keep
	// wordpress from being removed -- because the relation will be removed
	// if we destroy wordpress.
	// However, if a unit of the *other* service joins the relation, that
	// will add an additional reference and prevent the relation -- and
	// thus wordpress itself -- from being removed when its last unit is.
	mysql0, err := mysql.AddUnit()
	c.Assert(err, IsNil)
	mysql0ru, err := rel.Unit(mysql0)
	c.Assert(err, IsNil)
	err = mysql0ru.EnterScope(nil)
	c.Assert(err, IsNil)

	// Destroy wordpress, and remove its last unit.
	err = wordpress.Destroy()
	c.Assert(err, IsNil)
	err = wordpress0.EnsureDead()
	c.Assert(err, IsNil)
	err = wordpress0.Remove()
	c.Assert(err, IsNil)

	// Check this didn't kill the service or relation yet...
	err = wordpress.Refresh()
	c.Assert(err, IsNil)
	err = rel.Refresh()
	c.Assert(err, IsNil)

	// ...but when the unit on the other side departs the relation, the
	// relation and the other service are cleaned up.
	err = mysql0ru.LeaveScope()
	c.Assert(err, IsNil)
	err = wordpress.Refresh()
	c.Assert(err, checkers.Satisfies, errors.IsNotFoundError)
	err = rel.Refresh()
	c.Assert(err, checkers.Satisfies, errors.IsNotFoundError)
}

func (s *UnitSuite) TestWatchSubordinates(c *C) {
	w := s.unit.WatchSubordinateUnits()
	defer testing.AssertStop(c, w)
	wc := testing.NewLaxStringsWatcherC(c, s.State, w)
	wc.AssertOneChange()

	// Add a couple of subordinates, check change.
	subCharm := s.AddTestingCharm(c, "logging")
	var subUnits []*state.Unit
	for i := 0; i < 2; i++ {
		// Note: subordinate units can only be created as a side effect of a
		// principal entering scope; and a given principal can only have a
		// single subordinate unit of each service.
		name := "logging" + strconv.Itoa(i)
		subSvc, err := s.State.AddService(name, subCharm)
		c.Assert(err, IsNil)
		eps, err := s.State.InferEndpoints([]string{name, "wordpress"})
		c.Assert(err, IsNil)
		rel, err := s.State.AddRelation(eps...)
		c.Assert(err, IsNil)
		ru, err := rel.Unit(s.unit)
		c.Assert(err, IsNil)
		err = ru.EnterScope(nil)
		c.Assert(err, IsNil)
		units, err := subSvc.AllUnits()
		c.Assert(err, IsNil)
		c.Assert(units, HasLen, 1)
		subUnits = append(subUnits, units[0])
	}
	wc.AssertOneChange(subUnits[0].Name(), subUnits[1].Name())

	// Set one to Dying, check change.
	err := subUnits[0].Destroy()
	c.Assert(err, IsNil)
	wc.AssertOneChange(subUnits[0].Name())

	// Set both to Dead, and remove one; check change.
	err = subUnits[0].EnsureDead()
	c.Assert(err, IsNil)
	err = subUnits[1].EnsureDead()
	c.Assert(err, IsNil)
	err = subUnits[1].Remove()
	c.Assert(err, IsNil)
	wc.AssertOneChange(subUnits[0].Name(), subUnits[1].Name())

	// Stop watcher, check closed.
	testing.AssertStop(c, w)
	wc.AssertClosed()

	// Start a new watch, check Dead unit is reported.
	w = s.unit.WatchSubordinateUnits()
	defer testing.AssertStop(c, w)
	wc = testing.NewLaxStringsWatcherC(c, s.State, w)
	wc.AssertOneChange(subUnits[0].Name())

	// Remove the leftover, check no change.
	err = subUnits[0].Remove()
	c.Assert(err, IsNil)
	wc.AssertNoChange()
}

func (s *UnitSuite) TestWatchUnit(c *C) {
	preventUnitDestroyRemove(c, s.unit)
	w := s.unit.Watch()
	defer testing.AssertStop(c, w)

	// Initial event.
	wc := testing.NewNotifyWatcherC(c, s.State, w)
	wc.AssertOneChange()

	// Make one change (to a separate instance), check one event.
	unit, err := s.State.Unit(s.unit.Name())
	c.Assert(err, IsNil)
	err = unit.SetPublicAddress("example.foobar.com")
	c.Assert(err, IsNil)
	wc.AssertOneChange()

	// Make two changes, check one event.
	err = unit.SetPrivateAddress("example.foobar")
	c.Assert(err, IsNil)
	err = unit.Destroy()
	c.Assert(err, IsNil)
	wc.AssertOneChange()

	// Stop, check closed.
	testing.AssertStop(c, w)
	wc.AssertClosed()

	// Remove unit, start new watch, check single event.
	err = unit.EnsureDead()
	c.Assert(err, IsNil)
	err = unit.Remove()
	c.Assert(err, IsNil)
	w = s.unit.Watch()
	defer testing.AssertStop(c, w)
	testing.NewNotifyWatcherC(c, s.State, w).AssertOneChange()
}

func (s *UnitSuite) TestAnnotatorForUnit(c *C) {
	testAnnotator(c, func() (state.Annotator, error) {
		return s.State.Unit("wordpress/0")
	})
}

func (s *UnitSuite) TestAnnotationRemovalForUnit(c *C) {
	annotations := map[string]string{"mykey": "myvalue"}
	err := s.unit.SetAnnotations(annotations)
	c.Assert(err, IsNil)
	err = s.unit.EnsureDead()
	c.Assert(err, IsNil)
	err = s.unit.Remove()
	c.Assert(err, IsNil)
	ann, err := s.unit.Annotations()
	c.Assert(err, IsNil)
	c.Assert(ann, DeepEquals, make(map[string]string))
}
