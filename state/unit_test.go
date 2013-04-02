package state_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"sort"
	"strconv"
	"time"
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
}

func (s *UnitSuite) TestUnitNotFound(c *C) {
	_, err := s.State.Unit("subway/0")
	c.Assert(err, ErrorMatches, `unit "subway/0" not found`)
	c.Assert(state.IsNotFound(err), Equals, true)
}

func (s *UnitSuite) TestService(c *C) {
	svc, err := s.unit.Service()
	c.Assert(err, IsNil)
	c.Assert(svc.Name(), Equals, s.unit.ServiceName())
}

func (s *UnitSuite) TestServiceConfig(c *C) {
	scfg, err := s.service.Config()
	c.Assert(err, IsNil)
	scfg.Update(map[string]interface{}{
		"foo":        "bar",
		"blog-title": "no title",
	})
	_, err = scfg.Write()
	c.Assert(err, IsNil)

	unit, err := s.service.AddUnit()
	c.Assert(err, IsNil)

	_, err = unit.ServiceConfig()
	c.Assert(err, ErrorMatches, "unit charm not set")

	err = unit.SetCharmURL(s.charm.URL())
	c.Assert(err, IsNil)

	cfg, err := unit.ServiceConfig()
	c.Assert(err, IsNil)
	c.Assert(cfg, DeepEquals, scfg.Map())
}

func (s *UnitSuite) TestGetSetPublicAddress(c *C) {
	address, ok := s.unit.PublicAddress()
	c.Assert(ok, Equals, false)
	err := s.unit.SetPublicAddress("example.foobar.com")
	c.Assert(err, IsNil)
	address, ok = s.unit.PublicAddress()
	c.Assert(ok, Equals, true)
	c.Assert(address, Equals, "example.foobar.com")
}

func (s *UnitSuite) TestGetSetPrivateAddress(c *C) {
	address, ok := s.unit.PrivateAddress()
	c.Assert(ok, Equals, false)
	err := s.unit.SetPrivateAddress("example.local")
	c.Assert(err, IsNil)
	address, ok = s.unit.PrivateAddress()
	c.Assert(ok, Equals, true)
	c.Assert(address, Equals, "example.local")
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
	c.Assert(state.IsNotFound(err), Equals, true)
}

func (s *UnitSuite) TestGetSetStatus(c *C) {
	fail := func() { s.unit.SetStatus(params.UnitError, "") }
	c.Assert(fail, PanicMatches, "must set info for unit error status")

	status, info, err := s.unit.Status()
	c.Assert(err, IsNil)
	c.Assert(status, Equals, params.UnitPending)
	c.Assert(info, Equals, "")

	err = s.unit.SetStatus(params.UnitStarted, "")
	c.Assert(err, IsNil)
	status, info, err = s.unit.Status()
	c.Assert(err, IsNil)
	c.Assert(status, Equals, params.UnitStarted)
	c.Assert(info, Equals, "")

	err = s.unit.SetStatus(params.UnitError, "test-hook failed")
	c.Assert(err, IsNil)
	status, info, err = s.unit.Status()
	c.Assert(err, IsNil)
	c.Assert(status, Equals, params.UnitError)
	c.Assert(info, Equals, "test-hook failed")

	err = s.unit.SetStatus(params.UnitPending, "deploying...")
	c.Assert(err, IsNil)
	status, info, err = s.unit.Status()
	c.Assert(err, IsNil)
	c.Assert(status, Equals, params.UnitPending)
	c.Assert(info, Equals, "deploying...")
}

func (s *UnitSuite) TestUnitCharm(c *C) {
	preventUnitDestroyRemove(c, s.State, s.unit)
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
	preventUnitDestroyRemove(c, s.State, s.unit)
	for i := 0; i < 4; i++ {
		unit, err := s.service.AddUnit()
		c.Assert(err, IsNil)
		preventUnitDestroyRemove(c, s.State, unit)
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

func (s *UnitSuite) TestShortCircuitDestroyUnassignedUnit(c *C) {
	// A unit without subordinates or assigned machine is removed directly.
	err := s.unit.Destroy()
	c.Assert(err, IsNil)
	c.Assert(s.unit.Life(), Equals, state.Dying)
	s.assertUnitRemoved(c, s.unit)
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
	s.assertUnitLife(c, s.unit.Name(), state.Dying)
}

func (s *UnitSuite) TestShortCircuitDestroyWithUnprovisionedMachine(c *C) {
	// A unit with an assigned but unprovisioned machine is removed directly.
	err := s.unit.AssignToNewMachine()
	c.Assert(err, IsNil)
	err = s.unit.Destroy()
	c.Assert(err, IsNil)
	c.Assert(s.unit.Life(), Equals, state.Dying)
	s.assertUnitRemoved(c, s.unit)
}

func (s *UnitSuite) TestCannotShortCircuitDestroyWithProvisionedMachine(c *C) {
	// A unit assigned to a provisioned machine is set to Dying.
	err := s.unit.AssignToNewMachine()
	c.Assert(err, IsNil)
	mid, err := s.unit.AssignedMachineId()
	c.Assert(err, IsNil)
	machine, err := s.State.Machine(mid)
	c.Assert(err, IsNil)
	err = machine.SetInstanceId("i-malive")
	c.Assert(err, IsNil)
	err = s.unit.Destroy()
	c.Assert(err, IsNil)
	c.Assert(s.unit.Life(), Equals, state.Dying)
	s.assertUnitLife(c, s.unit.Name(), state.Dying)
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
	c.Assert(unit.Refresh(), IsNil)
	c.Assert(unit.Life(), Equals, life)
}

func (s *UnitSuite) assertUnitRemoved(c *C, unit *state.Unit) {
	err := unit.Refresh()
	c.Assert(state.IsNotFound(err), Equals, true)
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
	preventUnitDestroyRemove(c, s.State, s.unit)
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
	c.Assert(state.IsUnauthorizedError(err), Equals, true)

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

func (s *UnitSuite) TestGetSetClearResolved(c *C) {
	mode := s.unit.Resolved()
	c.Assert(mode, Equals, params.ResolvedNone)

	err := s.unit.SetResolved(params.ResolvedNoHooks)
	c.Assert(err, IsNil)
	err = s.unit.SetResolved(params.ResolvedNoHooks)
	c.Assert(err, ErrorMatches, `cannot set resolved mode for unit "wordpress/0": already resolved`)

	mode = s.unit.Resolved()
	c.Assert(mode, Equals, params.ResolvedNoHooks)
	err = s.unit.Refresh()
	c.Assert(err, IsNil)
	mode = s.unit.Resolved()
	c.Assert(mode, Equals, params.ResolvedNoHooks)

	err = s.unit.ClearResolved()
	c.Assert(err, IsNil)
	mode = s.unit.Resolved()
	c.Assert(mode, Equals, params.ResolvedNone)
	err = s.unit.Refresh()
	c.Assert(err, IsNil)
	mode = s.unit.Resolved()
	c.Assert(mode, Equals, params.ResolvedNone)
	err = s.unit.ClearResolved()
	c.Assert(err, IsNil)

	err = s.unit.SetResolved(params.ResolvedNone)
	c.Assert(err, ErrorMatches, `cannot set resolved mode for unit "wordpress/0": invalid error resolution mode: ""`)
	err = s.unit.SetResolved(params.ResolvedMode("foo"))
	c.Assert(err, ErrorMatches, `cannot set resolved mode for unit "wordpress/0": invalid error resolution mode: "foo"`)
}

func (s *UnitSuite) TestOpenedPorts(c *C) {
	// Verify no open ports before activity.
	c.Assert(s.unit.OpenedPorts(), HasLen, 0)

	// Now open and close port.
	err := s.unit.OpenPort("tcp", 80)
	c.Assert(err, IsNil)
	open := s.unit.OpenedPorts()
	c.Assert(open, DeepEquals, []params.Port{
		{"tcp", 80},
	})

	err = s.unit.OpenPort("udp", 53)
	c.Assert(err, IsNil)
	open = s.unit.OpenedPorts()
	c.Assert(open, DeepEquals, []params.Port{
		{"tcp", 80},
		{"udp", 53},
	})

	err = s.unit.OpenPort("tcp", 53)
	c.Assert(err, IsNil)
	open = s.unit.OpenedPorts()
	c.Assert(open, DeepEquals, []params.Port{
		{"tcp", 53},
		{"tcp", 80},
		{"udp", 53},
	})

	err = s.unit.OpenPort("tcp", 443)
	c.Assert(err, IsNil)
	open = s.unit.OpenedPorts()
	c.Assert(open, DeepEquals, []params.Port{
		{"tcp", 53},
		{"tcp", 80},
		{"tcp", 443},
		{"udp", 53},
	})

	err = s.unit.ClosePort("tcp", 80)
	c.Assert(err, IsNil)
	open = s.unit.OpenedPorts()
	c.Assert(open, DeepEquals, []params.Port{
		{"tcp", 53},
		{"tcp", 443},
		{"udp", 53},
	})

	err = s.unit.ClosePort("tcp", 80)
	c.Assert(err, IsNil)
	open = s.unit.OpenedPorts()
	c.Assert(open, DeepEquals, []params.Port{
		{"tcp", 53},
		{"tcp", 443},
		{"udp", 53},
	})
}

func (s *UnitSuite) TestOpenClosePortWhenDying(c *C) {
	preventUnitDestroyRemove(c, s.State, s.unit)
	testWhenDying(c, s.unit, noErr, deadErr, func() error {
		return s.unit.OpenPort("tcp", 20)
	}, func() error {
		return s.unit.ClosePort("tcp", 20)
	})
}

func (s *UnitSuite) TestSetClearResolvedWhenNotAlive(c *C) {
	preventUnitDestroyRemove(c, s.State, s.unit)
	err := s.unit.Destroy()
	c.Assert(err, IsNil)
	err = s.unit.SetResolved(params.ResolvedNoHooks)
	c.Assert(err, IsNil)
	err = s.unit.Refresh()
	c.Assert(err, IsNil)
	c.Assert(s.unit.Resolved(), Equals, params.ResolvedNoHooks)
	err = s.unit.ClearResolved()
	c.Assert(err, IsNil)

	err = s.unit.EnsureDead()
	c.Assert(err, IsNil)
	err = s.unit.SetResolved(params.ResolvedRetryHooks)
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
	c.Assert(state.IsNotFound(err), Equals, true)
	units, err := s.service.AllUnits()
	c.Assert(err, IsNil)
	c.Assert(units, HasLen, 0)
	err = s.unit.Remove()
	c.Assert(err, IsNil)
}

func (s *UnitSuite) TestWatchSubordinates(c *C) {
	w := s.unit.WatchSubordinateUnits()
	defer stop(c, w)
	assertChange := func(units ...string) {
		s.State.Sync()
		select {
		case ch, ok := <-w.Changes():
			c.Assert(ok, Equals, true)
			if len(units) > 0 {
				sort.Strings(ch)
				sort.Strings(units)
				c.Assert(ch, DeepEquals, units)
			} else {
				c.Assert(ch, HasLen, 0)
			}
		case <-time.After(500 * time.Millisecond):
			c.Fatalf("timed out waiting for %#v", units)
		}
	}
	assertChange()
	assertNoChange := func() {
		s.State.StartSync()
		select {
		case ch, ok := <-w.Changes():
			c.Fatalf("unexpected change: %#v, %v", ch, ok)
		case <-time.After(100 * time.Millisecond):
		}
	}
	assertNoChange()

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
	assertChange(subUnits[0].Name(), subUnits[1].Name())
	assertNoChange()

	// Set one to Dying, check change.
	err := subUnits[0].Destroy()
	c.Assert(err, IsNil)
	assertChange(subUnits[0].Name())
	assertNoChange()

	// Set both to Dead, and remove one; check change.
	err = subUnits[0].EnsureDead()
	c.Assert(err, IsNil)
	err = subUnits[1].EnsureDead()
	c.Assert(err, IsNil)
	err = subUnits[1].Remove()
	c.Assert(err, IsNil)
	assertChange(subUnits[0].Name(), subUnits[1].Name())
	assertNoChange()

	// Stop watcher, check closed.
	err = w.Stop()
	c.Assert(err, IsNil)
	select {
	case _, ok := <-w.Changes():
		c.Assert(ok, Equals, false)
	default:
	}

	// Start a new watch, check Dead unit is reported.
	w = s.unit.WatchSubordinateUnits()
	defer stop(c, w)
	assertChange(subUnits[0].Name())

	// Remove the leftover, check no change.
	err = subUnits[0].Remove()
	c.Assert(err, IsNil)
	assertNoChange()
}

type unitInfo struct {
	PublicAddress string
	Life          state.Life
}

var watchUnitTests = []struct {
	test func(m *state.Unit) error
	want unitInfo
}{
	{
		func(u *state.Unit) error {
			return u.SetPublicAddress("example.foobar.com")
		},
		unitInfo{
			PublicAddress: "example.foobar.com",
		},
	},
	{
		func(u *state.Unit) error {
			return u.SetPublicAddress("ubuntu.com")
		},
		unitInfo{
			PublicAddress: "ubuntu.com",
		},
	},
	{
		func(u *state.Unit) error {
			return u.Destroy()
		},
		unitInfo{
			Life: state.Dying,
		},
	},
}

func (s *UnitSuite) TestWatchUnit(c *C) {
	preventUnitDestroyRemove(c, s.State, s.unit)
	altunit, err := s.State.Unit(s.unit.Name())
	c.Assert(err, IsNil)
	err = altunit.SetPublicAddress("newer-address")
	c.Assert(err, IsNil)
	_, ok := s.unit.PublicAddress()
	c.Assert(ok, Equals, false)

	w := s.unit.Watch()
	defer func() {
		c.Assert(w.Stop(), IsNil)
	}()
	s.State.Sync()
	select {
	case _, ok := <-w.Changes():
		c.Assert(ok, Equals, true)
		err := s.unit.Refresh()
		c.Assert(err, IsNil)
		addr, ok := s.unit.PublicAddress()
		c.Assert(ok, Equals, true)
		c.Assert(addr, Equals, "newer-address")
	case <-time.After(500 * time.Millisecond):
		c.Fatalf("did not get change: %v", s.unit)
	}

	for i, test := range watchUnitTests {
		c.Logf("test %d", i)
		err := test.test(altunit)
		c.Assert(err, IsNil)
		s.State.StartSync()
		select {
		case _, ok := <-w.Changes():
			c.Assert(ok, Equals, true)
			err := s.unit.Refresh()
			c.Assert(err, IsNil)
			var info unitInfo
			info.Life = s.unit.Life()
			c.Assert(err, IsNil)
			if test.want.PublicAddress != "" {
				info.PublicAddress, ok = s.unit.PublicAddress()
				c.Assert(ok, Equals, true)
			}
			c.Assert(info, DeepEquals, test.want)
		case <-time.After(500 * time.Millisecond):
			c.Fatalf("did not get change: %v", test.want)
		}
	}
	select {
	case got, ok := <-w.Changes():
		c.Fatalf("got unexpected change: %#v, %v", got, ok)
	case <-time.After(100 * time.Millisecond):
	}
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
