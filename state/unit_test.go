package state_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/state"
	"sort"
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
	svc, err := s.State.Service(unit1.ServiceName())
	c.Assert(err, IsNil)
	err = svc.RemoveUnit(unit1)
	c.Assert(err, IsNil)
	err = unit1.Refresh()
	c.Assert(state.IsNotFound(err), Equals, true)
}

func (s *UnitSuite) TestGetSetStatus(c *C) {
	fail := func() { s.unit.SetStatus(state.UnitPending, "") }
	c.Assert(fail, PanicMatches, "unit status must not be set to pending")

	status, info, err := s.unit.Status()
	c.Assert(err, IsNil)
	c.Assert(status, Equals, state.UnitPending)
	c.Assert(info, Equals, "")

	err = s.unit.SetStatus(state.UnitStarted, "")
	c.Assert(err, IsNil)

	status, info, err = s.unit.Status()
	c.Assert(err, IsNil)
	c.Assert(status, Equals, state.UnitDown)
	c.Assert(info, Equals, "")

	p, err := s.unit.SetAgentAlive()
	c.Assert(err, IsNil)
	defer func() {
		c.Assert(p.Kill(), IsNil)
	}()

	s.State.Sync()
	status, info, err = s.unit.Status()
	c.Assert(err, IsNil)
	c.Assert(status, Equals, state.UnitStarted)
	c.Assert(info, Equals, "")

	err = s.unit.SetStatus(state.UnitError, "test-hook failed")
	c.Assert(err, IsNil)
	status, info, err = s.unit.Status()
	c.Assert(err, IsNil)
	c.Assert(status, Equals, state.UnitError)
	c.Assert(info, Equals, "test-hook failed")
}

func (s *UnitSuite) TestUnitCharm(c *C) {
	_, err := s.unit.Charm()
	c.Assert(err, ErrorMatches, `charm URL of unit "wordpress/0" not found`)

	err = s.unit.SetCharm(s.charm)
	c.Assert(err, IsNil)
	ch, err := s.unit.Charm()
	c.Assert(err, IsNil)
	c.Assert(ch.URL(), DeepEquals, s.charm.URL())

	err = s.unit.EnsureDying()
	c.Assert(err, IsNil)
	err = s.unit.SetCharm(s.charm)
	c.Assert(err, IsNil)
	ch, err = s.unit.Charm()
	c.Assert(err, IsNil)
	c.Assert(ch.URL(), DeepEquals, s.charm.URL())

	err = s.unit.EnsureDead()
	c.Assert(err, IsNil)
	err = s.unit.SetCharm(s.charm)
	c.Assert(err, ErrorMatches, `cannot set charm for unit "wordpress/0": not found or not alive`)
}

func (s *UnitSuite) TestEntityName(c *C) {
	c.Assert(s.unit.EntityName(), Equals, "unit-wordpress-0")
}

func (s *UnitSuite) TestUnitEntityName(c *C) {
	c.Assert(state.UnitEntityName("wordpress/2"), Equals, "unit-wordpress-2")
}

func (s *UnitSuite) TestSetMongoPassword(c *C) {
	testSetMongoPassword(c, func(st *state.State) (entity, error) {
		return st.Unit(s.unit.Name())
	})
}

func (s *UnitSuite) TestSetPassword(c *C) {
	testSetPassword(c, func() (entity, error) {
		return s.State.Unit(s.unit.Name())
	})
}

func (s *UnitSuite) TestSetMongoPasswordOnUnitAfterConnectingAsMachineEntity(c *C) {
	// Make a second unit to use later.
	subCharm := s.AddTestingCharm(c, "logging")
	logService, err := s.State.AddService("logging", subCharm)
	c.Assert(err, IsNil)
	subUnit, err := logService.AddUnitSubordinateTo(s.unit)
	c.Assert(err, IsNil)

	info := state.TestingStateInfo()
	st, err := state.Open(info)
	c.Assert(err, IsNil)
	defer st.Close()
	// Turn on fully-authenticated mode.
	err = st.SetAdminMongoPassword("admin-secret")
	c.Assert(err, IsNil)

	// Add a new machine, assign the units to it
	// and set its password.
	m, err := st.AddMachine(state.JobHostUnits)
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
	info.EntityName = m.EntityName()
	info.Password = "foo1"
	err = tryOpenState(info)
	c.Assert(err, Equals, state.ErrUnauthorized)

	// Connect as the machine entity.
	info.EntityName = m.EntityName()
	info.Password = "foo"
	st1, err := state.Open(info)
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
	info.EntityName = unit.EntityName()
	info.Password = "bar"
	st2, err := state.Open(info)
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
	c.Assert(pinger, Not(IsNil))
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
	c.Assert(open, DeepEquals, []state.Port{
		{"tcp", 80},
	})

	err = s.unit.OpenPort("udp", 53)
	c.Assert(err, IsNil)
	open = s.unit.OpenedPorts()
	c.Assert(open, DeepEquals, []state.Port{
		{"tcp", 80},
		{"udp", 53},
	})

	err = s.unit.OpenPort("tcp", 53)
	c.Assert(err, IsNil)
	open = s.unit.OpenedPorts()
	c.Assert(open, DeepEquals, []state.Port{
		{"tcp", 53},
		{"tcp", 80},
		{"udp", 53},
	})

	err = s.unit.OpenPort("tcp", 443)
	c.Assert(err, IsNil)
	open = s.unit.OpenedPorts()
	c.Assert(open, DeepEquals, []state.Port{
		{"tcp", 53},
		{"tcp", 80},
		{"tcp", 443},
		{"udp", 53},
	})

	err = s.unit.ClosePort("tcp", 80)
	c.Assert(err, IsNil)
	open = s.unit.OpenedPorts()
	c.Assert(open, DeepEquals, []state.Port{
		{"tcp", 53},
		{"tcp", 443},
		{"udp", 53},
	})

	err = s.unit.ClosePort("tcp", 80)
	c.Assert(err, IsNil)
	open = s.unit.OpenedPorts()
	c.Assert(open, DeepEquals, []state.Port{
		{"tcp", 53},
		{"tcp", 443},
		{"udp", 53},
	})
}

func (s *UnitSuite) TestOpenClosePortWhenDying(c *C) {
	testWhenDying(c, s.unit, "", notAliveErr, func() error {
		return s.unit.OpenPort("tcp", 20)
	}, func() error {
		return s.unit.ClosePort("tcp", 20)
	})
}

func (s *UnitSuite) TestSetClearResolvedWhenNotAlive(c *C) {
	err := s.unit.EnsureDying()
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
	c.Assert(err, ErrorMatches, notAliveErr)
	err = s.unit.ClearResolved()
	c.Assert(err, IsNil)
}

func (s *UnitSuite) TestSubordinateChangeInPrincipal(c *C) {
	subCharm := s.AddTestingCharm(c, "logging")
	logService, err := s.State.AddService("logging", subCharm)
	c.Assert(err, IsNil)
	_, err = logService.AddUnitSubordinateTo(s.unit)
	c.Assert(err, IsNil)
	su1, err := logService.AddUnitSubordinateTo(s.unit)
	c.Assert(err, IsNil)

	doc := make(map[string][]string)
	s.ConnSuite.units.FindId(s.unit.Name()).One(&doc)
	subordinates, ok := doc["subordinates"]
	if !ok {
		c.Errorf(`unit document does not have a "subordinates" field`)
	}
	c.Assert(subordinates, DeepEquals, []string{"logging/0", "logging/1"})

	err = su1.EnsureDead()
	c.Assert(err, IsNil)
	err = logService.RemoveUnit(su1)
	c.Assert(err, IsNil)
	doc = make(map[string][]string)
	s.ConnSuite.units.FindId(s.unit.Name()).One(&doc)
	subordinates, ok = doc["subordinates"]
	if !ok {
		c.Errorf(`unit document does not have a "subordinates" field`)
	}
	c.Assert(subordinates, DeepEquals, []string{"logging/0"})
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
	logging, err := s.State.AddService("logging", s.AddTestingCharm(c, "logging"))
	c.Assert(err, IsNil)
	eps, err := s.State.InferEndpoints([]string{"logging", "wordpress"})
	c.Assert(err, IsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, IsNil)
	ru, err := rel.Unit(u)
	c.Assert(err, IsNil)
	err = u.SetPrivateAddress("blah")
	c.Assert(err, IsNil)
	err = ru.EnterScope()
	c.Assert(err, IsNil)

	// Check the unit cannot become Dead, but can become Dying...
	err = u.EnsureDead()
	c.Assert(err, Equals, state.ErrUnitHasSubordinates)
	err = u.EnsureDying()
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
	err = logging.RemoveUnit(sub)
	c.Assert(err, IsNil)
	err = u.EnsureDead()
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
	logging, err := s.State.AddService("logging", s.AddTestingCharm(c, "logging"))
	c.Assert(err, IsNil)
	logging0, err := logging.AddUnitSubordinateTo(s.unit)
	c.Assert(err, IsNil)
	logging1, err := logging.AddUnitSubordinateTo(s.unit)
	c.Assert(err, IsNil)
	assertChange("logging/0", "logging/1")
	assertNoChange()

	// Set one to Dying, check change.
	err = logging0.EnsureDying()
	c.Assert(err, IsNil)
	assertChange("logging/0")
	assertNoChange()

	// Set both to Dead, and remove one; check change.
	err = logging0.EnsureDead()
	c.Assert(err, IsNil)
	err = logging1.EnsureDead()
	c.Assert(err, IsNil)
	err = logging.RemoveUnit(logging1)
	c.Assert(err, IsNil)
	assertChange("logging/0", "logging/1")
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
	assertChange("logging/0")

	// Remove the leftover, check no change.
	err = logging.RemoveUnit(logging0)
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
			return u.EnsureDying()
		},
		unitInfo{
			Life: state.Dying,
		},
	},
}

func (s *UnitSuite) TestWatchUnit(c *C) {
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
