package state_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/state"
	"time"
)

func (s *StateSuite) TestServiceWatchConfig(c *C) {
	dummy, _ := addDummyCharm(c, s.st)
	wordpress, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)
	c.Assert(wordpress.Name(), Equals, "wordpress")

	config, err := wordpress.Config()
	c.Assert(err, IsNil)
	c.Assert(config.Keys(), HasLen, 0)
	configWatcher := wordpress.WatchConfig()

	// Two change events.
	config.Set("foo", "bar")
	config.Set("baz", "yadda")
	_, err = config.Write()
	c.Assert(err, IsNil)
	time.Sleep(100 * time.Millisecond)
	config.Delete("foo")
	_, err = config.Write()
	c.Assert(err, IsNil)

	var expectedChanges = []map[string]interface{}{
		{},
		{"foo": "bar", "baz": "yadda"},
		{"baz": "yadda"},
	}
	for _, want := range expectedChanges {
		select {
		case got, ok := <-configWatcher.Changes():
			c.Assert(ok, Equals, true)
			c.Assert(got.Map(), DeepEquals, want)
		case <-time.After(200 * time.Millisecond):
			c.Fatalf("didn't get change: %#v", want)
		}
	}

	select {
	case got, _ := <-configWatcher.Changes():
		c.Fatalf("got unexpected change: %#v", got)
	case <-time.After(100 * time.Millisecond):
	}

	err = configWatcher.Stop()
	c.Assert(err, IsNil)
}

func (s *StateSuite) TestServiceWatchConfigIllegalData(c *C) {
	dummy, _ := addDummyCharm(c, s.st)
	wordpress, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)
	c.Assert(wordpress.Name(), Equals, "wordpress")
	configWatcher := wordpress.WatchConfig()

	// Receive empty change after service adding.
	select {
	case got, ok := <-configWatcher.Changes():
		c.Assert(ok, Equals, true)
		c.Assert(got.Map(), DeepEquals, map[string]interface{}{})
	case <-time.After(100 * time.Millisecond):
		c.Fatalf("unexpected timeout")
	}

	// Set config to illegal data.
	_, err = s.zkConn.Set("/services/service-0000000000/config", "---", -1)
	c.Assert(err, IsNil)

	select {
	case _, ok := <-configWatcher.Changes():
		c.Assert(ok, Equals, false)
	case <-time.After(100 * time.Millisecond):
	}

	err = configWatcher.Stop()
	c.Assert(err, ErrorMatches, "YAML error: .*")
}

func (s *StateSuite) TestUnitWatchNeedsUpgrade(c *C) {
	dummy, _ := addDummyCharm(c, s.st)
	wordpress, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)
	c.Assert(wordpress.Name(), Equals, "wordpress")
	unit, err := wordpress.AddUnit()
	c.Assert(err, IsNil)
	needsUpgradeWatcher := unit.WatchNeedsUpgrade()

	go func() {
		time.Sleep(50 * time.Millisecond)
		err = unit.SetNeedsUpgrade()
		c.Assert(err, IsNil)
		time.Sleep(50 * time.Millisecond)
		err = unit.ClearNeedsUpgrade()
		c.Assert(err, IsNil)
		time.Sleep(50 * time.Millisecond)
		err = unit.SetNeedsUpgrade()
		c.Assert(err, IsNil)
	}()

	var expectedChanges = []bool{
		true,
		false,
		true,
	}
	for _, want := range expectedChanges {
		select {
		case got, ok := <-needsUpgradeWatcher.Changes():
			c.Assert(ok, Equals, true)
			c.Assert(got, Equals, want)
		case <-time.After(200 * time.Millisecond):
			c.Fatalf("didn't get change: %#v", want)
		}
	}

	select {
	case got, _ := <-needsUpgradeWatcher.Changes():
		c.Fatalf("got unexpected change: %#v", got)
	case <-time.After(100 * time.Millisecond):
	}

	err = needsUpgradeWatcher.Stop()
	c.Assert(err, IsNil)
}

func (s *StateSuite) TestUnitWatchResolved(c *C) {
	dummy, _ := addDummyCharm(c, s.st)
	wordpress, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)
	c.Assert(wordpress.Name(), Equals, "wordpress")
	unit, err := wordpress.AddUnit()
	c.Assert(err, IsNil)
	resolvedWatcher := unit.WatchResolved()

	go func() {
		time.Sleep(50 * time.Millisecond)
		err = unit.SetResolved(state.ResolvedRetryHooks)
		c.Assert(err, IsNil)
		time.Sleep(50 * time.Millisecond)
		err = unit.ClearResolved()
		c.Assert(err, IsNil)
		time.Sleep(50 * time.Millisecond)
		err = unit.SetResolved(state.ResolvedNoHooks)
		c.Assert(err, IsNil)
	}()

	var expectedChanges = []state.ResolvedMode{
		state.ResolvedRetryHooks,
		state.ResolvedNone,
		state.ResolvedNoHooks,
	}
	for _, want := range expectedChanges {
		select {
		case got, ok := <-resolvedWatcher.Changes():
			c.Assert(ok, Equals, true)
			c.Assert(got, Equals, want)
		case <-time.After(200 * time.Millisecond):
			c.Fatalf("didn't get change: %#v", want)
		}
	}

	select {
	case got, _ := <-resolvedWatcher.Changes():
		c.Fatalf("got unexpected change: %#v", got)
	case <-time.After(100 * time.Millisecond):
	}

	err = resolvedWatcher.Stop()
	c.Assert(err, IsNil)
}
