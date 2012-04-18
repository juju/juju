package state_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/state"
	"time"
)

func (s *StateSuite) TestServiceWatchConfig(c *C) {
	receiveChange := func(w *state.ConfigWatcher) (*state.ConfigNode, bool, bool) {
		select {
		case change, ok := <-w.Changes():
			return change, ok, false
		case <-time.After(100 * time.Millisecond):
			return nil, false, true
		}
		return nil, false, false
	}
	dummy, _ := addDummyCharm(c, s.st)
	wordpress, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)
	c.Assert(wordpress.Name(), Equals, "wordpress")

	config, err := wordpress.Config()
	c.Assert(err, IsNil)
	c.Assert(config.Keys(), HasLen, 0)
	watcher := wordpress.WatchConfig()

	// Recieve initial event after creation.
	changedConfig, ok, timeout := receiveChange(watcher)
	c.Assert(timeout, Equals, false)
	c.Assert(ok, Equals, true)
	c.Assert(changedConfig.Keys(), HasLen, 0)

	// Two more change events.
	config.Set("foo", "bar")
	config.Set("baz", "yadda")
	_, err = config.Write()
	c.Assert(err, IsNil)
	time.Sleep(100 * time.Millisecond)
	config.Delete("foo")
	_, err = config.Write()
	c.Assert(err, IsNil)

	// Receive the two changes.
	changedConfig, ok, timeout = receiveChange(watcher)
	c.Assert(timeout, Equals, false)
	c.Assert(ok, Equals, true)
	c.Assert(changedConfig.Map(), DeepEquals, map[string]interface{}{"foo": "bar", "baz": "yadda"})
	foo, found := changedConfig.Get("foo")
	c.Assert(found, Equals, true)
	c.Assert(foo, Equals, "bar")

	changedConfig, ok, timeout = receiveChange(watcher)
	c.Assert(timeout, Equals, false)
	c.Assert(ok, Equals, true)
	c.Assert(changedConfig.Map(), DeepEquals, map[string]interface{}{"baz": "yadda"})
	foo, found = changedConfig.Get("foo")
	c.Assert(found, Equals, false)
	baz, found := changedConfig.Get("baz")
	c.Assert(found, Equals, true)
	c.Assert(baz, Equals, "yadda")

	// No more changes.
	_, _, timeout = receiveChange(watcher)
	c.Assert(timeout, Equals, true)

	err = watcher.Stop()
	c.Assert(err, IsNil)
}

func (s *StateSuite) TestServiceWatchConfigIllegalData(c *C) {
	receiveChange := func(w *state.ConfigWatcher) (*state.ConfigNode, bool, bool) {
		select {
		case change, ok := <-w.Changes():
			return change, ok, false
		case <-time.After(100 * time.Millisecond):
			return nil, false, true
		}
		return nil, false, false
	}
	dummy, _ := addDummyCharm(c, s.st)
	wordpress, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)
	c.Assert(wordpress.Name(), Equals, "wordpress")
	watcher := wordpress.WatchConfig()

	// Create with illegal data.
	_, err = s.zkConn.Set("/services/service-0000000000/config", "---", -1)
	c.Assert(err, IsNil)

	// Changes() has to be closed.
	_, ok, timeout := receiveChange(watcher)
	c.Assert(ok, Equals, false)
	c.Assert(timeout, Equals, false)

	err = watcher.Stop()
	c.Assert(err, ErrorMatches, "YAML error: .*")
}

func (s *StateSuite) TestUnitWatchNeedsUpgrade(c *C) {
	receiveChange := func(w *state.NeedsUpgradeWatcher) (bool, bool, bool) {
		select {
		case change, ok := <-w.Changes():
			return change, ok, false
		case <-time.After(100 * time.Millisecond):
			return false, false, true
		}
		return false, false, false
	}
	dummy, _ := addDummyCharm(c, s.st)
	wordpress, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)
	c.Assert(wordpress.Name(), Equals, "wordpress")
	unit, err := wordpress.AddUnit()
	c.Assert(err, IsNil)
	watcher := unit.WatchNeedsUpgrade()

	go func() {
		time.Sleep(50 * time.Millisecond)
		err = unit.SetNeedsUpgrade()
		c.Assert(err, IsNil)
		time.Sleep(50 * time.Millisecond)
		err = unit.ClearNeedsUpgrade()
		c.Assert(err, IsNil)
	}()

	// Receive the changes.
	upgrade, ok, timeout := receiveChange(watcher)
	c.Assert(timeout, Equals, false)
	c.Assert(ok, Equals, true)
	c.Assert(upgrade, Equals, true)
	upgrade, ok, timeout = receiveChange(watcher)
	c.Assert(ok, Equals, true)
	c.Assert(timeout, Equals, false)
	c.Assert(upgrade, Equals, false)

	// No more changes.
	_, _, timeout = receiveChange(watcher)
	c.Assert(timeout, Equals, true)

	err = watcher.Stop()
	c.Assert(err, IsNil)
}
