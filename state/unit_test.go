package state_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/state"
	"time"
)

type UnitSuite struct {
	ConnSuite
	charm *state.Charm
	unit  *state.Unit
}

var _ = Suite(&UnitSuite{})

func (s *UnitSuite) SetUpTest(c *C) {
	s.ConnSuite.SetUpTest(c)
	s.charm = s.AddTestingCharm(c, "dummy")
	svc, err := s.State.AddService("wordpress", s.charm)
	c.Assert(err, IsNil)
	s.unit, err = svc.AddUnit()
	c.Assert(err, IsNil)
}

func (s *UnitSuite) TestGetSetPublicAddress(c *C) {
	address, err := s.unit.PublicAddress()
	c.Assert(err, ErrorMatches, `public address of unit "wordpress/0" not found`)
	err = s.unit.SetPublicAddress("example.foobar.com")
	c.Assert(err, IsNil)
	address, err = s.unit.PublicAddress()
	c.Assert(err, IsNil)
	c.Assert(address, Equals, "example.foobar.com")
}

func (s *UnitSuite) TestGetSetPrivateAddress(c *C) {
	address, err := s.unit.PrivateAddress()
	c.Assert(err, ErrorMatches, `private address of unit "wordpress/0" not found`)
	err = s.unit.SetPrivateAddress("example.local")
	c.Assert(err, IsNil)
	address, err = s.unit.PrivateAddress()
	c.Assert(err, IsNil)
	c.Assert(address, Equals, "example.local")
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
}

func (s *UnitSuite) TestGetSetClearResolved(c *C) {
	setting, err := s.unit.Resolved()
	c.Assert(err, IsNil)
	c.Assert(setting, Equals, state.ResolvedNone)

	err = s.unit.SetResolved(state.ResolvedNoHooks)
	c.Assert(err, IsNil)
	err = s.unit.SetResolved(state.ResolvedNoHooks)
	c.Assert(err, ErrorMatches, `cannot set resolved mode for unit "wordpress/0": flag already set`)
	retry, err := s.unit.Resolved()
	c.Assert(err, IsNil)
	c.Assert(retry, Equals, state.ResolvedNoHooks)

	err = s.unit.ClearResolved()
	c.Assert(err, IsNil)
	setting, err = s.unit.Resolved()
	c.Assert(err, IsNil)
	c.Assert(setting, Equals, state.ResolvedNone)
	err = s.unit.ClearResolved()
	c.Assert(err, IsNil)

	err = s.unit.SetResolved(state.ResolvedMode(999))
	c.Assert(err, ErrorMatches, `cannot set resolved mode for unit "wordpress/0": invalid error resolution mode: 999`)
}

func (s *UnitSuite) TestGetOpenPorts(c *C) {
	// Verify no open ports before activity.
	open, err := s.unit.OpenPorts()
	c.Assert(err, IsNil)
	c.Assert(open, HasLen, 0)

	// Now open and close port.
	err = s.unit.OpenPort("tcp", 80)
	c.Assert(err, IsNil)
	open, err = s.unit.OpenPorts()
	c.Assert(err, IsNil)
	c.Assert(open, DeepEquals, []state.Port{
		{"tcp", 80},
	})

	err = s.unit.OpenPort("udp", 53)
	c.Assert(err, IsNil)
	open, err = s.unit.OpenPorts()
	c.Assert(err, IsNil)
	c.Assert(open, DeepEquals, []state.Port{
		{"tcp", 80},
		{"udp", 53},
	})

	err = s.unit.OpenPort("tcp", 53)
	c.Assert(err, IsNil)
	open, err = s.unit.OpenPorts()
	c.Assert(err, IsNil)
	c.Assert(open, DeepEquals, []state.Port{
		{"tcp", 80},
		{"udp", 53},
		{"tcp", 53},
	})

	err = s.unit.OpenPort("tcp", 443)
	c.Assert(err, IsNil)
	open, err = s.unit.OpenPorts()
	c.Assert(err, IsNil)
	c.Assert(open, DeepEquals, []state.Port{
		{"tcp", 80},
		{"udp", 53},
		{"tcp", 53},
		{"tcp", 443},
	})

	err = s.unit.ClosePort("tcp", 80)
	c.Assert(err, IsNil)
	open, err = s.unit.OpenPorts()
	c.Assert(err, IsNil)
	c.Assert(open, DeepEquals, []state.Port{
		{"udp", 53},
		{"tcp", 53},
		{"tcp", 443},
	})
}

func (s *UnitSuite) TestPathKey(c *C) {
	c.Assert(s.unit.PathKey(), Equals, "unit-wordpress-0")
}

func (s *UnitSuite) TestUnitSetAgentAlive(c *C) {
	alive, err := s.unit.AgentAlive()
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, false)

	pinger, err := s.unit.SetAgentAlive()
	c.Assert(err, IsNil)
	c.Assert(pinger, Not(IsNil))
	defer pinger.Kill()

	alive, err = s.unit.AgentAlive()
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, true)
}

func (s *UnitSuite) TestUnitWaitAgentAlive(c *C) {
	timeout := 5 * time.Second
	alive, err := s.unit.AgentAlive()
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, false)

	err = s.unit.WaitAgentAlive(timeout)
	c.Assert(err, ErrorMatches, `waiting for agent of unit "wordpress/0": presence: still not alive after timeout`)

	pinger, err := s.unit.SetAgentAlive()
	c.Assert(err, IsNil)
	c.Assert(pinger, Not(IsNil))

	err = s.unit.WaitAgentAlive(timeout)
	c.Assert(err, IsNil)

	alive, err = s.unit.AgentAlive()
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, true)

	pinger.Kill()

	alive, err = s.unit.AgentAlive()
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, false)
}

type unitWatchResolvedTest struct {
	test func(*state.Unit) error
	want state.ResolvedMode
}

var unitWatchResolvedTests = []unitWatchResolvedTest{
	{func(u *state.Unit) error { return nil }, state.ResolvedNone},
	{func(u *state.Unit) error { return u.SetResolved(state.ResolvedRetryHooks) }, state.ResolvedRetryHooks},
	{func(u *state.Unit) error { return u.ClearResolved() }, state.ResolvedNone},
	{func(u *state.Unit) error { return u.SetResolved(state.ResolvedNoHooks) }, state.ResolvedNoHooks},
}

func (s *UnitSuite) TestUnitWatchResolved(c *C) {
	resolvedWatcher := s.unit.WatchResolved()
	defer func() {
		c.Assert(resolvedWatcher.Stop(), IsNil)
	}()

	for i, test := range unitWatchResolvedTests {
		c.Logf("test %d", i)
		err := test.test(s.unit)
		c.Assert(err, IsNil)
		select {
		case got, ok := <-resolvedWatcher.Changes():
			c.Assert(ok, Equals, true)
			c.Assert(got, Equals, test.want)
		case <-time.After(200 * time.Millisecond):
			c.Fatalf("did not get change: %#v", test.want)
		}
	}

	select {
	case got := <-resolvedWatcher.Changes():
		c.Fatalf("got unexpected change: %#v", got)
	case <-time.After(100 * time.Millisecond):
	}
}

type unitWatchPortsTest struct {
	test func(*state.Unit) error
	want []state.Port
}

var unitWatchPortsTests = []unitWatchPortsTest{
	{func(u *state.Unit) error { return nil }, nil},
	{func(u *state.Unit) error { return u.OpenPort("tcp", 80) }, []state.Port{{"tcp", 80}}},
	{func(u *state.Unit) error { return u.OpenPort("udp", 53) }, []state.Port{{"tcp", 80}, {"udp", 53}}},
	{func(u *state.Unit) error { return u.ClosePort("tcp", 80) }, []state.Port{{"udp", 53}}},
}

func (s *UnitSuite) TestUnitWatchPorts(c *C) {
	portsWatcher := s.unit.WatchPorts()
	defer func() {
		c.Assert(portsWatcher.Stop(), IsNil)
	}()

	for i, test := range unitWatchPortsTests {
		c.Logf("test %d", i)
		err := test.test(s.unit)
		c.Assert(err, IsNil)
		select {
		case got, ok := <-portsWatcher.Changes():
			c.Assert(ok, Equals, true)
			c.Assert(got, DeepEquals, test.want)
		case <-time.After(200 * time.Millisecond):
			c.Fatalf("did not get change: %#v", test.want)
		}
	}

	select {
	case got := <-portsWatcher.Changes():
		c.Fatalf("got unexpected change: %#v", got)
	case <-time.After(100 * time.Millisecond):
	}
}

type unitInfo struct {
	publicAddress string
	tools *state.Tools
}

var watchUnitTests = []struct {
	test func(u *state.Unit) error
	want unitInfo
}{
	{
		func(u *state.Unit) error {
			return nil
		},
		unitInfo{
			tools: &state.Tools{},
		},
	},
	{
		func(u *state.Unit) error {
			return u.SetPublicAddress("localhost")
		},
		unitInfo{
			publicAddress:      "localhost",
		},
	},
	{
		func(u *state.Unit) error {
			return u.SetAgentTools(tools(3, "baz"))
		},
		unitInfo{
			publicAddress:      "localhost",
			tools: tools(3, "baz"),
		},
	},
	{
		func(u *state.Unit) error {
			return u.SetAgentTools(tools(4, "khroomph"))
		},
		unitInfo{
			publicAddress:      "localhost",
			tools: tools(4, "khroomph"),
		},
	},
}

func (s *UnitSuite) TestWatchUnit(c *C) {
	w := s.unit.Watch()
	defer func() {
		c.Assert(w.Stop(), IsNil)
	}()
	for i, test := range watchUnitTests {
		c.Logf("test %d", i)
		err := test.test(s.unit)
		c.Assert(err, IsNil)
		select {
		case u, ok := <-w.Changes():
			c.Assert(ok, Equals, true)
			c.Assert(u.Name(), Equals, s.unit.Name())
			var info unitInfo
			info.tools, err = u.AgentTools()
			c.Assert(err, IsNil)
			info.publicAddress, err = u.PublicAddress()
			c.Assert(err, IsNil)
			c.Assert(info, DeepEquals, test.want)
		case <-time.After(500 * time.Millisecond):
			c.Fatalf("did not get change: %v", test.want)
		}
	}
	select {
	case got := <-w.Changes():
		c.Fatalf("got unexpected change: %#v", got)
	case <-time.After(100 * time.Millisecond):
	}
}
