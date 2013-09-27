// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"strconv"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/testing"
	coretesting "launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
)

type UnitSuite struct {
	ConnSuite
	charm   *state.Charm
	service *state.Service
	unit    *state.Unit
}

var _ = gc.Suite(&UnitSuite{})

func (s *UnitSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.charm = s.AddTestingCharm(c, "wordpress")
	var err error
	s.service, err = s.State.AddService("wordpress", s.charm)
	c.Assert(err, gc.IsNil)
	s.unit, err = s.service.AddUnit()
	c.Assert(err, gc.IsNil)
	c.Assert(s.unit.Series(), gc.Equals, "quantal")
}

func (s *UnitSuite) TestUnitNotFound(c *gc.C) {
	_, err := s.State.Unit("subway/0")
	c.Assert(err, gc.ErrorMatches, `unit "subway/0" not found`)
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)
}

func (s *UnitSuite) TestService(c *gc.C) {
	svc, err := s.unit.Service()
	c.Assert(err, gc.IsNil)
	c.Assert(svc.Name(), gc.Equals, s.unit.ServiceName())
}

func (s *UnitSuite) TestConfigSettingsNeedCharmURLSet(c *gc.C) {
	_, err := s.unit.ConfigSettings()
	c.Assert(err, gc.ErrorMatches, "unit charm not set")
}

func (s *UnitSuite) TestConfigSettingsIncludeDefaults(c *gc.C) {
	err := s.unit.SetCharmURL(s.charm.URL())
	c.Assert(err, gc.IsNil)
	settings, err := s.unit.ConfigSettings()
	c.Assert(err, gc.IsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{"blog-title": "My Title"})
}

func (s *UnitSuite) TestConfigSettingsReflectService(c *gc.C) {
	err := s.service.UpdateConfigSettings(charm.Settings{"blog-title": "no title"})
	c.Assert(err, gc.IsNil)
	err = s.unit.SetCharmURL(s.charm.URL())
	c.Assert(err, gc.IsNil)
	settings, err := s.unit.ConfigSettings()
	c.Assert(err, gc.IsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{"blog-title": "no title"})

	err = s.service.UpdateConfigSettings(charm.Settings{"blog-title": "ironic title"})
	c.Assert(err, gc.IsNil)
	settings, err = s.unit.ConfigSettings()
	c.Assert(err, gc.IsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{"blog-title": "ironic title"})
}

func (s *UnitSuite) TestConfigSettingsReflectCharm(c *gc.C) {
	err := s.unit.SetCharmURL(s.charm.URL())
	c.Assert(err, gc.IsNil)
	newCharm := s.AddConfigCharm(c, "wordpress", "options: {}", 123)
	err = s.service.SetCharm(newCharm, false)
	c.Assert(err, gc.IsNil)

	// Settings still reflect charm set on unit.
	settings, err := s.unit.ConfigSettings()
	c.Assert(err, gc.IsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{"blog-title": "My Title"})

	// When the unit has the new charm set, it'll see the new config.
	err = s.unit.SetCharmURL(newCharm.URL())
	c.Assert(err, gc.IsNil)
	settings, err = s.unit.ConfigSettings()
	c.Assert(err, gc.IsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{})
}

func (s *UnitSuite) TestWatchConfigSettingsNeedsCharmURL(c *gc.C) {
	_, err := s.unit.WatchConfigSettings()
	c.Assert(err, gc.ErrorMatches, "unit charm not set")
}

func (s *UnitSuite) TestWatchConfigSettings(c *gc.C) {
	err := s.unit.SetCharmURL(s.charm.URL())
	c.Assert(err, gc.IsNil)
	w, err := s.unit.WatchConfigSettings()
	c.Assert(err, gc.IsNil)
	defer testing.AssertStop(c, w)

	// Initial event.
	wc := testing.NewNotifyWatcherC(c, s.State, w)
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

	// Change service's charm; nothing detected.
	newCharm := s.AddConfigCharm(c, "wordpress", floatConfig, 123)
	err = s.service.SetCharm(newCharm, false)
	c.Assert(err, gc.IsNil)
	wc.AssertNoChange()

	// Change service config for new charm; nothing detected.
	err = s.service.UpdateConfigSettings(charm.Settings{
		"key": 42.0,
	})
	c.Assert(err, gc.IsNil)
	wc.AssertNoChange()

	// NOTE: if we were to change the unit to use the new charm, we'd see
	// another event, because the originally-watched document will become
	// unreferenced and be removed. But I'm not testing that behaviour
	// because it's not very helpful and subject to change.
}

func (s *UnitSuite) TestGetSetPublicAddress(c *gc.C) {
	_, ok := s.unit.PublicAddress()
	c.Assert(ok, gc.Equals, false)

	err := s.unit.SetPublicAddress("example.foobar.com")
	c.Assert(err, gc.IsNil)
	address, ok := s.unit.PublicAddress()
	c.Assert(ok, gc.Equals, true)
	c.Assert(address, gc.Equals, "example.foobar.com")

	defer state.SetBeforeHooks(c, s.State, func() {
		c.Assert(s.unit.Destroy(), gc.IsNil)
	}).Check()
	err = s.unit.SetPublicAddress("example.foobar.com")
	c.Assert(err, gc.ErrorMatches, `cannot set public address of unit "wordpress/0": unit not found`)
}

func (s *UnitSuite) TestGetPublicAddressFromMachine(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	err = s.unit.AssignToMachine(machine)
	c.Assert(err, gc.IsNil)

	address, ok := s.unit.PublicAddress()
	c.Check(address, gc.Equals, "")
	c.Assert(ok, gc.Equals, false)

	addresses := []instance.Address{
		instance.NewAddress("127.0.0.1"),
		instance.NewAddress("8.8.8.8"),
	}
	err = machine.SetAddresses(addresses)
	c.Assert(err, gc.IsNil)

	address, ok = s.unit.PublicAddress()
	c.Check(address, gc.Equals, "8.8.8.8")
	c.Assert(ok, gc.Equals, true)
}

func (s *UnitSuite) TestGetSetPrivateAddress(c *gc.C) {
	_, ok := s.unit.PrivateAddress()
	c.Assert(ok, gc.Equals, false)

	err := s.unit.SetPrivateAddress("example.local")
	c.Assert(err, gc.IsNil)
	address, ok := s.unit.PrivateAddress()
	c.Assert(ok, gc.Equals, true)
	c.Assert(address, gc.Equals, "example.local")

	defer state.SetBeforeHooks(c, s.State, func() {
		c.Assert(s.unit.Destroy(), gc.IsNil)
	}).Check()
	err = s.unit.SetPrivateAddress("example.local")
	c.Assert(err, gc.ErrorMatches, `cannot set private address of unit "wordpress/0": unit not found`)
}

func (s *UnitSuite) TestGetPrivateAddressFromMachine(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	err = s.unit.AssignToMachine(machine)
	c.Assert(err, gc.IsNil)

	address, ok := s.unit.PrivateAddress()
	c.Check(address, gc.Equals, "")
	c.Assert(ok, gc.Equals, false)

	addresses := []instance.Address{
		instance.NewAddress("127.0.0.1"),
		instance.NewAddress("8.8.8.8"),
	}
	err = machine.SetAddresses(addresses)
	c.Assert(err, gc.IsNil)

	address, ok = s.unit.PrivateAddress()
	c.Check(address, gc.Equals, "127.0.0.1")
	c.Assert(ok, gc.Equals, true)
}

func (s *UnitSuite) TestRefresh(c *gc.C) {
	unit1, err := s.State.Unit(s.unit.Name())
	c.Assert(err, gc.IsNil)

	err = s.unit.SetPrivateAddress("example.local")
	c.Assert(err, gc.IsNil)
	err = s.unit.SetPublicAddress("example.foobar.com")
	c.Assert(err, gc.IsNil)

	address, ok := unit1.PrivateAddress()
	c.Assert(ok, gc.Equals, false)
	address, ok = unit1.PublicAddress()
	c.Assert(ok, gc.Equals, false)

	err = unit1.Refresh()
	c.Assert(err, gc.IsNil)
	address, ok = unit1.PrivateAddress()
	c.Assert(ok, gc.Equals, true)
	c.Assert(address, gc.Equals, "example.local")
	address, ok = unit1.PublicAddress()
	c.Assert(ok, gc.Equals, true)
	c.Assert(address, gc.Equals, "example.foobar.com")

	err = unit1.EnsureDead()
	c.Assert(err, gc.IsNil)
	err = unit1.Remove()
	c.Assert(err, gc.IsNil)
	err = unit1.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)
}

func (s *UnitSuite) TestGetSetStatusWhileAlive(c *gc.C) {
	err := s.unit.SetStatus(params.StatusError, "", nil)
	c.Assert(err, gc.ErrorMatches, `cannot set status "error" without info`)
	err = s.unit.SetStatus(params.StatusPending, "", nil)
	c.Assert(err, gc.ErrorMatches, `cannot set status "pending"`)
	err = s.unit.SetStatus(params.StatusDown, "", nil)
	c.Assert(err, gc.ErrorMatches, `cannot set status "down"`)
	err = s.unit.SetStatus(params.Status("vliegkat"), "orville", nil)
	c.Assert(err, gc.ErrorMatches, `cannot set invalid status "vliegkat"`)

	status, info, err := s.unit.Status()
	c.Assert(err, gc.IsNil)
	c.Assert(status, gc.Equals, params.StatusPending)
	c.Assert(info, gc.Equals, "")

	err = s.unit.SetStatus(params.StatusStarted, "", nil)
	c.Assert(err, gc.IsNil)
	status, info, err = s.unit.Status()
	c.Assert(err, gc.IsNil)
	c.Assert(status, gc.Equals, params.StatusStarted)
	c.Assert(info, gc.Equals, "")

	err = s.unit.SetStatus(params.StatusError, "test-hook failed", nil)
	c.Assert(err, gc.IsNil)
	status, info, err = s.unit.Status()
	c.Assert(err, gc.IsNil)
	c.Assert(status, gc.Equals, params.StatusError)
	c.Assert(info, gc.Equals, "test-hook failed")
}

func (s *UnitSuite) TestGetSetStatusWhileNotAlive(c *gc.C) {
	err := s.unit.Destroy()
	c.Assert(err, gc.IsNil)
	err = s.unit.SetStatus(params.StatusStarted, "not really", nil)
	c.Assert(err, gc.ErrorMatches, `cannot set status of unit "wordpress/0": not found or dead`)
	_, _, err = s.unit.Status()
	c.Assert(err, gc.ErrorMatches, "status not found")

	err = s.unit.EnsureDead()
	c.Assert(err, gc.IsNil)
	err = s.unit.SetStatus(params.StatusStarted, "not really", nil)
	c.Assert(err, gc.ErrorMatches, `cannot set status of unit "wordpress/0": not found or dead`)
	_, _, err = s.unit.Status()
	c.Assert(err, gc.ErrorMatches, "status not found")
}

func (s *UnitSuite) TestGetSetStatusDataStandard(c *gc.C) {
	err := s.unit.SetStatus(params.StatusStarted, "", nil)
	c.Assert(err, gc.IsNil)
	_, _, err = s.unit.Status()
	c.Assert(err, gc.IsNil)

	// Regular status setting with data.
	err = s.unit.SetStatus(params.StatusError, "test-hook failed", params.StatusData{
		"1st-key": "one",
		"2nd-key": 2,
		"3rd-key": true,
	})
	c.Assert(err, gc.IsNil)

	status, info, err := s.unit.Status()
	c.Assert(err, gc.IsNil)
	c.Assert(status, gc.Equals, params.StatusError)
	c.Assert(info, gc.Equals, "test-hook failed")

	data, err := state.UnitStatusData(s.unit)
	c.Assert(err, gc.IsNil)
	c.Assert(data, gc.DeepEquals, params.StatusData{
		"1st-key": "one",
		"2nd-key": 2,
		"3rd-key": true,
	})
}

func (s *UnitSuite) TestGetSetStatusDataMongo(c *gc.C) {
	err := s.unit.SetStatus(params.StatusStarted, "", nil)
	c.Assert(err, gc.IsNil)
	_, _, err = s.unit.Status()
	c.Assert(err, gc.IsNil)

	// Status setting with MongoDB special values.
	err = s.unit.SetStatus(params.StatusError, "mongo", params.StatusData{
		`{name: "Joe"}`: "$where",
		"eval":          `eval(function(foo) { return foo; }, "bar")`,
		"mapReduce":     "mapReduce",
		"group":         "group",
	})
	c.Assert(err, gc.IsNil)

	status, info, err := s.unit.Status()
	c.Assert(err, gc.IsNil)
	c.Assert(status, gc.Equals, params.StatusError)
	c.Assert(info, gc.Equals, "mongo")

	data, err := state.UnitStatusData(s.unit)
	c.Assert(err, gc.IsNil)
	c.Assert(data, gc.DeepEquals, params.StatusData{
		`{name: "Joe"}`: "$where",
		"eval":          `eval(function(foo) { return foo; }, "bar")`,
		"mapReduce":     "mapReduce",
		"group":         "group",
	})
}

func (s *UnitSuite) TestGetSetStatusDataChange(c *gc.C) {
	err := s.unit.SetStatus(params.StatusStarted, "", nil)
	c.Assert(err, gc.IsNil)
	_, _, err = s.unit.Status()
	c.Assert(err, gc.IsNil)

	// Status setting and changing data afterwards.
	data := params.StatusData{
		"1st-key": "one",
		"2nd-key": 2,
		"3rd-key": true,
	}
	err = s.unit.SetStatus(params.StatusError, "test-hook failed", data)
	c.Assert(err, gc.IsNil)
	data["4th-key"] = 4.0

	status, info, err := s.unit.Status()
	c.Assert(err, gc.IsNil)
	c.Assert(status, gc.Equals, params.StatusError)
	c.Assert(info, gc.Equals, "test-hook failed")

	data, err = state.UnitStatusData(s.unit)
	c.Assert(err, gc.IsNil)
	c.Assert(data, gc.DeepEquals, params.StatusData{
		"1st-key": "one",
		"2nd-key": 2,
		"3rd-key": true,
	})

	// Set status data to nil, so an empty map will be returned.
	err = s.unit.SetStatus(params.StatusStarted, "", nil)
	c.Assert(err, gc.IsNil)

	status, info, err = s.unit.Status()
	c.Assert(err, gc.IsNil)
	c.Assert(status, gc.Equals, params.StatusStarted)
	c.Assert(info, gc.Equals, "")

	data, err = state.UnitStatusData(s.unit)
	c.Assert(err, gc.IsNil)
	c.Assert(data, gc.HasLen, 0)
}

func (s *UnitSuite) TestUnitCharm(c *gc.C) {
	preventUnitDestroyRemove(c, s.unit)
	curl, ok := s.unit.CharmURL()
	c.Assert(ok, gc.Equals, false)
	c.Assert(curl, gc.IsNil)

	err := s.unit.SetCharmURL(nil)
	c.Assert(err, gc.ErrorMatches, "cannot set nil charm url")

	err = s.unit.SetCharmURL(charm.MustParseURL("cs:missing/one-1"))
	c.Assert(err, gc.ErrorMatches, `unknown charm url "cs:missing/one-1"`)

	err = s.unit.SetCharmURL(s.charm.URL())
	c.Assert(err, gc.IsNil)
	curl, ok = s.unit.CharmURL()
	c.Assert(ok, gc.Equals, true)
	c.Assert(curl, gc.DeepEquals, s.charm.URL())

	err = s.unit.Destroy()
	c.Assert(err, gc.IsNil)
	err = s.unit.SetCharmURL(s.charm.URL())
	c.Assert(err, gc.IsNil)
	curl, ok = s.unit.CharmURL()
	c.Assert(ok, gc.Equals, true)
	c.Assert(curl, gc.DeepEquals, s.charm.URL())

	err = s.unit.EnsureDead()
	c.Assert(err, gc.IsNil)
	err = s.unit.SetCharmURL(s.charm.URL())
	c.Assert(err, gc.ErrorMatches, `unit "wordpress/0" is dead`)
}

func (s *UnitSuite) TestDestroyPrincipalUnits(c *gc.C) {
	preventUnitDestroyRemove(c, s.unit)
	for i := 0; i < 4; i++ {
		unit, err := s.service.AddUnit()
		c.Assert(err, gc.IsNil)
		preventUnitDestroyRemove(c, unit)
	}

	// Destroy 2 of them; check they become Dying.
	err := s.State.DestroyUnits("wordpress/0", "wordpress/1")
	c.Assert(err, gc.IsNil)
	s.assertUnitLife(c, "wordpress/0", state.Dying)
	s.assertUnitLife(c, "wordpress/1", state.Dying)

	// Try to destroy an Alive one and a Dying one; check
	// it destroys the Alive one and ignores the Dying one.
	err = s.State.DestroyUnits("wordpress/2", "wordpress/0")
	c.Assert(err, gc.IsNil)
	s.assertUnitLife(c, "wordpress/2", state.Dying)

	// Try to destroy an Alive one along with a nonexistent one; check that
	// the valid instruction is followed but the invalid one is warned about.
	err = s.State.DestroyUnits("boojum/123", "wordpress/3")
	c.Assert(err, gc.ErrorMatches, `some units were not destroyed: unit "boojum/123" does not exist`)
	s.assertUnitLife(c, "wordpress/3", state.Dying)

	// Make one Dead, and destroy an Alive one alongside it; check no errors.
	wp0, err := s.State.Unit("wordpress/0")
	c.Assert(err, gc.IsNil)
	err = wp0.EnsureDead()
	c.Assert(err, gc.IsNil)
	err = s.State.DestroyUnits("wordpress/0", "wordpress/4")
	c.Assert(err, gc.IsNil)
	s.assertUnitLife(c, "wordpress/0", state.Dead)
	s.assertUnitLife(c, "wordpress/4", state.Dying)
}

func (s *UnitSuite) TestDestroySetStatusRetry(c *gc.C) {
	defer state.SetRetryHooks(c, s.State, func() {
		err := s.unit.SetStatus(params.StatusStarted, "", nil)
		c.Assert(err, gc.IsNil)
	}, func() {
		assertUnitLife(c, s.unit, state.Dying)
	}).Check()
	err := s.unit.Destroy()
	c.Assert(err, gc.IsNil)
}

func (s *UnitSuite) TestDestroySetCharmRetry(c *gc.C) {
	defer state.SetRetryHooks(c, s.State, func() {
		err := s.unit.SetCharmURL(s.charm.URL())
		c.Assert(err, gc.IsNil)
	}, func() {
		assertUnitRemoved(c, s.unit)
	}).Check()
	err := s.unit.Destroy()
	c.Assert(err, gc.IsNil)
}

func (s *UnitSuite) TestDestroyChangeCharmRetry(c *gc.C) {
	err := s.unit.SetCharmURL(s.charm.URL())
	c.Assert(err, gc.IsNil)
	newCharm := s.AddConfigCharm(c, "mysql", "options: {}", 99)
	err = s.service.SetCharm(newCharm, false)
	c.Assert(err, gc.IsNil)

	defer state.SetRetryHooks(c, s.State, func() {
		err := s.unit.SetCharmURL(newCharm.URL())
		c.Assert(err, gc.IsNil)
	}, func() {
		assertUnitRemoved(c, s.unit)
	}).Check()
	err = s.unit.Destroy()
	c.Assert(err, gc.IsNil)
}

func (s *UnitSuite) TestDestroyAssignRetry(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)

	defer state.SetRetryHooks(c, s.State, func() {
		err := s.unit.AssignToMachine(machine)
		c.Assert(err, gc.IsNil)
	}, func() {
		assertUnitRemoved(c, s.unit)
		// Also check the unit ref was properly removed from the machine doc --
		// if it weren't, we wouldn't be able to make the machine Dead.
		err := machine.EnsureDead()
		c.Assert(err, gc.IsNil)
	}).Check()
	err = s.unit.Destroy()
	c.Assert(err, gc.IsNil)
}

func (s *UnitSuite) TestDestroyUnassignRetry(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	err = s.unit.AssignToMachine(machine)
	c.Assert(err, gc.IsNil)

	defer state.SetRetryHooks(c, s.State, func() {
		err := s.unit.UnassignFromMachine()
		c.Assert(err, gc.IsNil)
	}, func() {
		assertUnitRemoved(c, s.unit)
	}).Check()
	err = s.unit.Destroy()
	c.Assert(err, gc.IsNil)
}

func (s *UnitSuite) TestShortCircuitDestroyUnit(c *gc.C) {
	// A unit that has not set any status is removed directly.
	err := s.unit.Destroy()
	c.Assert(err, gc.IsNil)
	c.Assert(s.unit.Life(), gc.Equals, state.Dying)
	assertUnitRemoved(c, s.unit)
}

func (s *UnitSuite) TestCannotShortCircuitDestroyWithSubordinates(c *gc.C) {
	// A unit with subordinates is just set to Dying.
	_, err := s.State.AddService("logging", s.AddTestingCharm(c, "logging"))
	c.Assert(err, gc.IsNil)
	eps, err := s.State.InferEndpoints([]string{"logging", "wordpress"})
	c.Assert(err, gc.IsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, gc.IsNil)
	ru, err := rel.Unit(s.unit)
	c.Assert(err, gc.IsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, gc.IsNil)
	err = s.unit.Destroy()
	c.Assert(err, gc.IsNil)
	c.Assert(s.unit.Life(), gc.Equals, state.Dying)
	assertUnitLife(c, s.unit, state.Dying)
}

func (s *UnitSuite) TestCannotShortCircuitDestroyWithStatus(c *gc.C) {
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
		c.Assert(err, gc.IsNil)
		err = unit.SetStatus(test.status, test.info, nil)
		c.Assert(err, gc.IsNil)
		err = unit.Destroy()
		c.Assert(err, gc.IsNil)
		c.Assert(unit.Life(), gc.Equals, state.Dying)
		assertUnitLife(c, unit, state.Dying)
	}
}

func (s *UnitSuite) TestShortCircuitDestroyWithProvisionedMachine(c *gc.C) {
	// A unit assigned to a provisioned machine is still removed directly so
	// long as it has not set status.
	err := s.unit.AssignToNewMachine()
	c.Assert(err, gc.IsNil)
	mid, err := s.unit.AssignedMachineId()
	c.Assert(err, gc.IsNil)
	machine, err := s.State.Machine(mid)
	c.Assert(err, gc.IsNil)
	err = machine.SetProvisioned("i-malive", "fake_nonce", nil)
	c.Assert(err, gc.IsNil)
	err = s.unit.Destroy()
	c.Assert(err, gc.IsNil)
	c.Assert(s.unit.Life(), gc.Equals, state.Dying)
	assertUnitRemoved(c, s.unit)
}

func (s *UnitSuite) TestDestroySubordinateUnits(c *gc.C) {
	lgsch := s.AddTestingCharm(c, "logging")
	_, err := s.State.AddService("logging", lgsch)
	c.Assert(err, gc.IsNil)
	eps, err := s.State.InferEndpoints([]string{"logging", "wordpress"})
	c.Assert(err, gc.IsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, gc.IsNil)
	ru, err := rel.Unit(s.unit)
	c.Assert(err, gc.IsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, gc.IsNil)

	// Try to destroy the subordinate alone; check it fails.
	err = s.State.DestroyUnits("logging/0")
	c.Assert(err, gc.ErrorMatches, `no units were destroyed: unit "logging/0" is a subordinate`)
	s.assertUnitLife(c, "logging/0", state.Alive)

	// Try to destroy the principal and the subordinate together; check it warns
	// about the subordinate, but destroys the one it can. (The principal unit
	// agent will be resposible for destroying the subordinate.)
	err = s.State.DestroyUnits("wordpress/0", "logging/0")
	c.Assert(err, gc.ErrorMatches, `some units were not destroyed: unit "logging/0" is a subordinate`)
	s.assertUnitLife(c, "wordpress/0", state.Dying)
	s.assertUnitLife(c, "logging/0", state.Alive)
}

func (s *UnitSuite) assertUnitLife(c *gc.C, name string, life state.Life) {
	unit, err := s.State.Unit(name)
	c.Assert(err, gc.IsNil)
	assertUnitLife(c, unit, life)
}

func assertUnitLife(c *gc.C, unit *state.Unit, life state.Life) {
	c.Assert(unit.Refresh(), gc.IsNil)
	c.Assert(unit.Life(), gc.Equals, life)
}

func assertUnitRemoved(c *gc.C, unit *state.Unit) {
	err := unit.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)
	err = unit.Destroy()
	c.Assert(err, gc.IsNil)
	err = unit.EnsureDead()
	c.Assert(err, gc.IsNil)
	err = unit.Remove()
	c.Assert(err, gc.IsNil)
}

func (s *UnitSuite) TestTag(c *gc.C) {
	c.Assert(s.unit.Tag(), gc.Equals, "unit-wordpress-0")
}

func (s *UnitSuite) TestSetMongoPassword(c *gc.C) {
	testSetMongoPassword(c, func(st *state.State) (entity, error) {
		return st.Unit(s.unit.Name())
	})
}

func (s *UnitSuite) TestSetPassword(c *gc.C) {
	preventUnitDestroyRemove(c, s.unit)
	testSetPassword(c, func() (state.Authenticator, error) {
		return s.State.Unit(s.unit.Name())
	})
}

func (s *UnitSuite) TestSetMongoPasswordOnUnitAfterConnectingAsMachineEntity(c *gc.C) {
	// Make a second unit to use later. (Subordinate units can only be created
	// as a side-effect of a principal entering relation scope.)
	subCharm := s.AddTestingCharm(c, "logging")
	_, err := s.State.AddService("logging", subCharm)
	c.Assert(err, gc.IsNil)
	eps, err := s.State.InferEndpoints([]string{"wordpress", "logging"})
	c.Assert(err, gc.IsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, gc.IsNil)
	ru, err := rel.Unit(s.unit)
	c.Assert(err, gc.IsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, gc.IsNil)
	subUnit, err := s.State.Unit("logging/0")
	c.Assert(err, gc.IsNil)

	info := state.TestingStateInfo()
	st, err := state.Open(info, state.TestingDialOpts())
	c.Assert(err, gc.IsNil)
	defer st.Close()
	// Turn on fully-authenticated mode.
	err = st.SetAdminMongoPassword("admin-secret")
	c.Assert(err, gc.IsNil)

	// Add a new machine, assign the units to it
	// and set its password.
	m, err := st.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	unit, err := st.Unit(s.unit.Name())
	c.Assert(err, gc.IsNil)
	subUnit, err = st.Unit(subUnit.Name())
	c.Assert(err, gc.IsNil)
	err = unit.AssignToMachine(m)
	c.Assert(err, gc.IsNil)
	err = m.SetMongoPassword("foo")
	c.Assert(err, gc.IsNil)

	// Sanity check that we cannot connect with the wrong
	// password
	info.Tag = m.Tag()
	info.Password = "foo1"
	err = tryOpenState(info)
	c.Assert(err, jc.Satisfies, errors.IsUnauthorizedError)

	// Connect as the machine entity.
	info.Tag = m.Tag()
	info.Password = "foo"
	st1, err := state.Open(info, state.TestingDialOpts())
	c.Assert(err, gc.IsNil)
	defer st1.Close()

	// Change the password for a unit derived from
	// the machine entity's state.
	unit, err = st1.Unit(s.unit.Name())
	c.Assert(err, gc.IsNil)
	err = unit.SetMongoPassword("bar")
	c.Assert(err, gc.IsNil)

	// Now connect as the unit entity and, as that
	// that entity, change the password for a new unit.
	info.Tag = unit.Tag()
	info.Password = "bar"
	st2, err := state.Open(info, state.TestingDialOpts())
	c.Assert(err, gc.IsNil)
	defer st2.Close()

	// Check that we can set its password.
	unit, err = st2.Unit(subUnit.Name())
	c.Assert(err, gc.IsNil)
	err = unit.SetMongoPassword("bar2")
	c.Assert(err, gc.IsNil)

	// Clear the admin password, so tests can reset the db.
	err = st.SetAdminMongoPassword("")
	c.Assert(err, gc.IsNil)
}

func (s *UnitSuite) TestUnitSetAgentAlive(c *gc.C) {
	alive, err := s.unit.AgentAlive()
	c.Assert(err, gc.IsNil)
	c.Assert(alive, gc.Equals, false)

	pinger, err := s.unit.SetAgentAlive()
	c.Assert(err, gc.IsNil)
	c.Assert(pinger, gc.NotNil)
	defer pinger.Stop()

	s.State.StartSync()
	alive, err = s.unit.AgentAlive()
	c.Assert(err, gc.IsNil)
	c.Assert(alive, gc.Equals, true)
}

func (s *UnitSuite) TestUnitWaitAgentAlive(c *gc.C) {
	alive, err := s.unit.AgentAlive()
	c.Assert(err, gc.IsNil)
	c.Assert(alive, gc.Equals, false)

	err = s.unit.WaitAgentAlive(coretesting.ShortWait)
	c.Assert(err, gc.ErrorMatches, `waiting for agent of unit "wordpress/0": still not alive after timeout`)

	pinger, err := s.unit.SetAgentAlive()
	c.Assert(err, gc.IsNil)

	s.State.StartSync()
	err = s.unit.WaitAgentAlive(coretesting.LongWait)
	c.Assert(err, gc.IsNil)

	alive, err = s.unit.AgentAlive()
	c.Assert(err, gc.IsNil)
	c.Assert(alive, gc.Equals, true)

	err = pinger.Kill()
	c.Assert(err, gc.IsNil)

	s.State.StartSync()

	alive, err = s.unit.AgentAlive()
	c.Assert(err, gc.IsNil)
	c.Assert(alive, gc.Equals, false)
}

func (s *UnitSuite) TestResolve(c *gc.C) {
	err := s.unit.Resolve(false)
	c.Assert(err, gc.ErrorMatches, `unit "wordpress/0" is not in an error state`)
	err = s.unit.Resolve(true)
	c.Assert(err, gc.ErrorMatches, `unit "wordpress/0" is not in an error state`)

	err = s.unit.SetStatus(params.StatusError, "gaaah", nil)
	c.Assert(err, gc.IsNil)
	err = s.unit.Resolve(false)
	c.Assert(err, gc.IsNil)
	err = s.unit.Resolve(true)
	c.Assert(err, gc.ErrorMatches, `cannot set resolved mode for unit "wordpress/0": already resolved`)
	c.Assert(s.unit.Resolved(), gc.Equals, state.ResolvedNoHooks)

	err = s.unit.ClearResolved()
	c.Assert(err, gc.IsNil)
	err = s.unit.Resolve(true)
	c.Assert(err, gc.IsNil)
	err = s.unit.Resolve(false)
	c.Assert(err, gc.ErrorMatches, `cannot set resolved mode for unit "wordpress/0": already resolved`)
	c.Assert(s.unit.Resolved(), gc.Equals, state.ResolvedRetryHooks)
}

func (s *UnitSuite) TestGetSetClearResolved(c *gc.C) {
	mode := s.unit.Resolved()
	c.Assert(mode, gc.Equals, state.ResolvedNone)

	err := s.unit.SetResolved(state.ResolvedNoHooks)
	c.Assert(err, gc.IsNil)
	err = s.unit.SetResolved(state.ResolvedNoHooks)
	c.Assert(err, gc.ErrorMatches, `cannot set resolved mode for unit "wordpress/0": already resolved`)

	mode = s.unit.Resolved()
	c.Assert(mode, gc.Equals, state.ResolvedNoHooks)
	err = s.unit.Refresh()
	c.Assert(err, gc.IsNil)
	mode = s.unit.Resolved()
	c.Assert(mode, gc.Equals, state.ResolvedNoHooks)

	err = s.unit.ClearResolved()
	c.Assert(err, gc.IsNil)
	mode = s.unit.Resolved()
	c.Assert(mode, gc.Equals, state.ResolvedNone)
	err = s.unit.Refresh()
	c.Assert(err, gc.IsNil)
	mode = s.unit.Resolved()
	c.Assert(mode, gc.Equals, state.ResolvedNone)
	err = s.unit.ClearResolved()
	c.Assert(err, gc.IsNil)

	err = s.unit.SetResolved(state.ResolvedNone)
	c.Assert(err, gc.ErrorMatches, `cannot set resolved mode for unit "wordpress/0": invalid error resolution mode: ""`)
	err = s.unit.SetResolved(state.ResolvedMode("foo"))
	c.Assert(err, gc.ErrorMatches, `cannot set resolved mode for unit "wordpress/0": invalid error resolution mode: "foo"`)
}

func (s *UnitSuite) TestOpenedPorts(c *gc.C) {
	// Verify no open ports before activity.
	c.Assert(s.unit.OpenedPorts(), gc.HasLen, 0)

	// Now open and close port.
	err := s.unit.OpenPort("tcp", 80)
	c.Assert(err, gc.IsNil)
	open := s.unit.OpenedPorts()
	c.Assert(open, gc.DeepEquals, []instance.Port{
		{"tcp", 80},
	})

	err = s.unit.OpenPort("udp", 53)
	c.Assert(err, gc.IsNil)
	open = s.unit.OpenedPorts()
	c.Assert(open, gc.DeepEquals, []instance.Port{
		{"tcp", 80},
		{"udp", 53},
	})

	err = s.unit.OpenPort("tcp", 53)
	c.Assert(err, gc.IsNil)
	open = s.unit.OpenedPorts()
	c.Assert(open, gc.DeepEquals, []instance.Port{
		{"tcp", 53},
		{"tcp", 80},
		{"udp", 53},
	})

	err = s.unit.OpenPort("tcp", 443)
	c.Assert(err, gc.IsNil)
	open = s.unit.OpenedPorts()
	c.Assert(open, gc.DeepEquals, []instance.Port{
		{"tcp", 53},
		{"tcp", 80},
		{"tcp", 443},
		{"udp", 53},
	})

	err = s.unit.ClosePort("tcp", 80)
	c.Assert(err, gc.IsNil)
	open = s.unit.OpenedPorts()
	c.Assert(open, gc.DeepEquals, []instance.Port{
		{"tcp", 53},
		{"tcp", 443},
		{"udp", 53},
	})

	err = s.unit.ClosePort("tcp", 80)
	c.Assert(err, gc.IsNil)
	open = s.unit.OpenedPorts()
	c.Assert(open, gc.DeepEquals, []instance.Port{
		{"tcp", 53},
		{"tcp", 443},
		{"udp", 53},
	})
}

func (s *UnitSuite) TestOpenClosePortWhenDying(c *gc.C) {
	preventUnitDestroyRemove(c, s.unit)
	testWhenDying(c, s.unit, noErr, deadErr, func() error {
		return s.unit.OpenPort("tcp", 20)
	}, func() error {
		return s.unit.ClosePort("tcp", 20)
	})
}

func (s *UnitSuite) TestSetClearResolvedWhenNotAlive(c *gc.C) {
	preventUnitDestroyRemove(c, s.unit)
	err := s.unit.Destroy()
	c.Assert(err, gc.IsNil)
	err = s.unit.SetResolved(state.ResolvedNoHooks)
	c.Assert(err, gc.IsNil)
	err = s.unit.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(s.unit.Resolved(), gc.Equals, state.ResolvedNoHooks)
	err = s.unit.ClearResolved()
	c.Assert(err, gc.IsNil)

	err = s.unit.EnsureDead()
	c.Assert(err, gc.IsNil)
	err = s.unit.SetResolved(state.ResolvedRetryHooks)
	c.Assert(err, gc.ErrorMatches, deadErr)
	err = s.unit.ClearResolved()
	c.Assert(err, gc.IsNil)
}

func (s *UnitSuite) TestSubordinateChangeInPrincipal(c *gc.C) {
	subCharm := s.AddTestingCharm(c, "logging")
	for i := 0; i < 2; i++ {
		// Note: subordinate units can only be created as a side effect of a
		// principal entering scope; and a given principal can only have a
		// single subordinate unit of each service.
		name := "logging" + strconv.Itoa(i)
		_, err := s.State.AddService(name, subCharm)
		c.Assert(err, gc.IsNil)
		eps, err := s.State.InferEndpoints([]string{name, "wordpress"})
		c.Assert(err, gc.IsNil)
		rel, err := s.State.AddRelation(eps...)
		c.Assert(err, gc.IsNil)
		ru, err := rel.Unit(s.unit)
		c.Assert(err, gc.IsNil)
		err = ru.EnterScope(nil)
		c.Assert(err, gc.IsNil)
	}

	err := s.unit.Refresh()
	c.Assert(err, gc.IsNil)
	subordinates := s.unit.SubordinateNames()
	c.Assert(subordinates, gc.DeepEquals, []string{"logging0/0", "logging1/0"})

	su1, err := s.State.Unit("logging1/0")
	c.Assert(err, gc.IsNil)
	err = su1.EnsureDead()
	c.Assert(err, gc.IsNil)
	err = su1.Remove()
	c.Assert(err, gc.IsNil)
	err = s.unit.Refresh()
	c.Assert(err, gc.IsNil)
	subordinates = s.unit.SubordinateNames()
	c.Assert(subordinates, gc.DeepEquals, []string{"logging0/0"})
}

func (s *UnitSuite) TestDeathWithSubordinates(c *gc.C) {
	// Check that units can become dead when they've never had subordinates.
	u, err := s.service.AddUnit()
	c.Assert(err, gc.IsNil)
	err = u.EnsureDead()
	c.Assert(err, gc.IsNil)

	// Create a new unit and add a subordinate.
	u, err = s.service.AddUnit()
	c.Assert(err, gc.IsNil)
	_, err = s.State.AddService("logging", s.AddTestingCharm(c, "logging"))
	c.Assert(err, gc.IsNil)
	eps, err := s.State.InferEndpoints([]string{"logging", "wordpress"})
	c.Assert(err, gc.IsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, gc.IsNil)
	ru, err := rel.Unit(u)
	c.Assert(err, gc.IsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, gc.IsNil)

	// Check the unit cannot become Dead, but can become Dying...
	err = u.EnsureDead()
	c.Assert(err, gc.Equals, state.ErrUnitHasSubordinates)
	err = u.Destroy()
	c.Assert(err, gc.IsNil)

	// ...and that it still can't become Dead now it's Dying.
	err = u.EnsureDead()
	c.Assert(err, gc.Equals, state.ErrUnitHasSubordinates)

	// Make the subordinate Dead and check the principal still cannot be removed.
	sub, err := s.State.Unit("logging/0")
	c.Assert(err, gc.IsNil)
	err = sub.EnsureDead()
	c.Assert(err, gc.IsNil)
	err = u.EnsureDead()
	c.Assert(err, gc.Equals, state.ErrUnitHasSubordinates)

	// remove the subordinate and check the principal can finally become Dead.
	err = sub.Remove()
	c.Assert(err, gc.IsNil)
	err = u.EnsureDead()
	c.Assert(err, gc.IsNil)
}

func (s *UnitSuite) TestPrincipalName(c *gc.C) {
	subCharm := s.AddTestingCharm(c, "logging")
	_, err := s.State.AddService("logging", subCharm)
	c.Assert(err, gc.IsNil)
	eps, err := s.State.InferEndpoints([]string{"logging", "wordpress"})
	c.Assert(err, gc.IsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, gc.IsNil)
	ru, err := rel.Unit(s.unit)
	c.Assert(err, gc.IsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, gc.IsNil)

	err = s.unit.Refresh()
	c.Assert(err, gc.IsNil)
	subordinates := s.unit.SubordinateNames()
	c.Assert(subordinates, gc.DeepEquals, []string{"logging/0"})

	su, err := s.State.Unit("logging/0")
	c.Assert(err, gc.IsNil)
	principal, valid := su.PrincipalName()
	c.Assert(valid, gc.Equals, true)
	c.Assert(principal, gc.Equals, s.unit.Name())

	// Calling PrincipalName on a principal unit yields "", false.
	principal, valid = s.unit.PrincipalName()
	c.Assert(valid, gc.Equals, false)
	c.Assert(principal, gc.Equals, "")
}

func (s *UnitSuite) TestRemove(c *gc.C) {
	err := s.unit.Remove()
	c.Assert(err, gc.ErrorMatches, `cannot remove unit "wordpress/0": unit is not dead`)
	err = s.unit.EnsureDead()
	c.Assert(err, gc.IsNil)
	err = s.unit.Remove()
	c.Assert(err, gc.IsNil)
	err = s.unit.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)
	units, err := s.service.AllUnits()
	c.Assert(err, gc.IsNil)
	c.Assert(units, gc.HasLen, 0)
	err = s.unit.Remove()
	c.Assert(err, gc.IsNil)
}

func (s *UnitSuite) TestRemovePathological(c *gc.C) {
	// Add a relation between wordpress and mysql...
	wordpress := s.service
	wordpress0 := s.unit
	mysql, err := s.State.AddService("mysql", s.AddTestingCharm(c, "mysql"))
	c.Assert(err, gc.IsNil)
	eps, err := s.State.InferEndpoints([]string{"wordpress", "mysql"})
	c.Assert(err, gc.IsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, gc.IsNil)

	// The relation holds a reference to wordpress, but that can't keep
	// wordpress from being removed -- because the relation will be removed
	// if we destroy wordpress.
	// However, if a unit of the *other* service joins the relation, that
	// will add an additional reference and prevent the relation -- and
	// thus wordpress itself -- from being removed when its last unit is.
	mysql0, err := mysql.AddUnit()
	c.Assert(err, gc.IsNil)
	mysql0ru, err := rel.Unit(mysql0)
	c.Assert(err, gc.IsNil)
	err = mysql0ru.EnterScope(nil)
	c.Assert(err, gc.IsNil)

	// Destroy wordpress, and remove its last unit.
	err = wordpress.Destroy()
	c.Assert(err, gc.IsNil)
	err = wordpress0.EnsureDead()
	c.Assert(err, gc.IsNil)
	err = wordpress0.Remove()
	c.Assert(err, gc.IsNil)

	// Check this didn't kill the service or relation yet...
	err = wordpress.Refresh()
	c.Assert(err, gc.IsNil)
	err = rel.Refresh()
	c.Assert(err, gc.IsNil)

	// ...but when the unit on the other side departs the relation, the
	// relation and the other service are cleaned up.
	err = mysql0ru.LeaveScope()
	c.Assert(err, gc.IsNil)
	err = wordpress.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)
	err = rel.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)
}

func (s *UnitSuite) TestWatchSubordinates(c *gc.C) {
	w := s.unit.WatchSubordinateUnits()
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, s.State, w)
	wc.AssertChange()
	wc.AssertNoChange()

	// Add a couple of subordinates, check change.
	subCharm := s.AddTestingCharm(c, "logging")
	var subUnits []*state.Unit
	for i := 0; i < 2; i++ {
		// Note: subordinate units can only be created as a side effect of a
		// principal entering scope; and a given principal can only have a
		// single subordinate unit of each service.
		name := "logging" + strconv.Itoa(i)
		subSvc, err := s.State.AddService(name, subCharm)
		c.Assert(err, gc.IsNil)
		eps, err := s.State.InferEndpoints([]string{name, "wordpress"})
		c.Assert(err, gc.IsNil)
		rel, err := s.State.AddRelation(eps...)
		c.Assert(err, gc.IsNil)
		ru, err := rel.Unit(s.unit)
		c.Assert(err, gc.IsNil)
		err = ru.EnterScope(nil)
		c.Assert(err, gc.IsNil)
		units, err := subSvc.AllUnits()
		c.Assert(err, gc.IsNil)
		c.Assert(units, gc.HasLen, 1)
		subUnits = append(subUnits, units[0])
	}
	wc.AssertChange(subUnits[0].Name(), subUnits[1].Name())
	wc.AssertNoChange()

	// Set one to Dying, check change.
	err := subUnits[0].Destroy()
	c.Assert(err, gc.IsNil)
	wc.AssertChange(subUnits[0].Name())
	wc.AssertNoChange()

	// Set both to Dead, and remove one; check change.
	err = subUnits[0].EnsureDead()
	c.Assert(err, gc.IsNil)
	err = subUnits[1].EnsureDead()
	c.Assert(err, gc.IsNil)
	err = subUnits[1].Remove()
	c.Assert(err, gc.IsNil)
	wc.AssertChange(subUnits[0].Name(), subUnits[1].Name())
	wc.AssertNoChange()

	// Stop watcher, check closed.
	testing.AssertStop(c, w)
	wc.AssertClosed()

	// Start a new watch, check Dead unit is reported.
	w = s.unit.WatchSubordinateUnits()
	defer testing.AssertStop(c, w)
	wc = testing.NewStringsWatcherC(c, s.State, w)
	wc.AssertChange(subUnits[0].Name())
	wc.AssertNoChange()

	// Remove the leftover, check no change.
	err = subUnits[0].Remove()
	c.Assert(err, gc.IsNil)
	wc.AssertNoChange()
}

func (s *UnitSuite) TestWatchUnit(c *gc.C) {
	preventUnitDestroyRemove(c, s.unit)
	w := s.unit.Watch()
	defer testing.AssertStop(c, w)

	// Initial event.
	wc := testing.NewNotifyWatcherC(c, s.State, w)
	wc.AssertOneChange()

	// Make one change (to a separate instance), check one event.
	unit, err := s.State.Unit(s.unit.Name())
	c.Assert(err, gc.IsNil)
	err = unit.SetPublicAddress("example.foobar.com")
	c.Assert(err, gc.IsNil)
	wc.AssertOneChange()

	// Make two changes, check one event.
	err = unit.SetPrivateAddress("example.foobar")
	c.Assert(err, gc.IsNil)
	err = unit.Destroy()
	c.Assert(err, gc.IsNil)
	wc.AssertOneChange()

	// Stop, check closed.
	testing.AssertStop(c, w)
	wc.AssertClosed()

	// Remove unit, start new watch, check single event.
	err = unit.EnsureDead()
	c.Assert(err, gc.IsNil)
	err = unit.Remove()
	c.Assert(err, gc.IsNil)
	w = s.unit.Watch()
	defer testing.AssertStop(c, w)
	testing.NewNotifyWatcherC(c, s.State, w).AssertOneChange()
}

func (s *UnitSuite) TestAnnotatorForUnit(c *gc.C) {
	testAnnotator(c, func() (state.Annotator, error) {
		return s.State.Unit("wordpress/0")
	})
}

func (s *UnitSuite) TestAnnotationRemovalForUnit(c *gc.C) {
	annotations := map[string]string{"mykey": "myvalue"}
	err := s.unit.SetAnnotations(annotations)
	c.Assert(err, gc.IsNil)
	err = s.unit.EnsureDead()
	c.Assert(err, gc.IsNil)
	err = s.unit.Remove()
	c.Assert(err, gc.IsNil)
	ann, err := s.unit.Annotations()
	c.Assert(err, gc.IsNil)
	c.Assert(ann, gc.DeepEquals, make(map[string]string))
}
