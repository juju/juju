package state_test

import (
	. "launchpad.net/gocheck"
	"time"
)

func (s *StateSuite) TestServiceWatchConfig(c *C) {
	dummy := s.addDummyCharm(c)
	wordpress, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)
	c.Assert(wordpress.Name(), Equals, "wordpress")

	config, err := wordpress.Config()
	c.Assert(err, IsNil)
	c.Assert(config.Keys(), HasLen, 0)
	watcher := wordpress.WatchConfig()

	// Recieve initial event after creation.
	changedConfig := <-watcher.Changes()
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
	changedConfig = <-watcher.Changes()
	c.Assert(changedConfig.Map(), DeepEquals, map[string]interface{}{"foo": "bar", "baz": "yadda"})
	foo, found := changedConfig.Get("foo")
	c.Assert(found, Equals, true)
	c.Assert(foo, Equals, "bar")

	changedConfig = <-watcher.Changes()
	c.Assert(changedConfig.Map(), DeepEquals, map[string]interface{}{"baz": "yadda"})
	foo, found = changedConfig.Get("foo")
	c.Assert(found, Equals, false)
	baz, found := changedConfig.Get("baz")
	c.Assert(found, Equals, true)
	c.Assert(baz, Equals, "yadda")

	// No more changes.
	select {
	case <-watcher.Changes():
		c.Fatalf("no more config changes expected")
	case <-time.After(200 * time.Millisecond):
		// The timeout is expected.
	}

	err = watcher.Stop()
	c.Assert(err, IsNil)
}

func (s *StateSuite) TestServiceWatchConfigIllegalData(c *C) {
	dummy := s.addDummyCharm(c)
	wordpress, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)
	c.Assert(wordpress.Name(), Equals, "wordpress")
	watcher := wordpress.WatchConfig()

	// Create with illegal data.
	_, err = s.zkConn.Set("/services/service-0000000000/config", "---", -1)
	c.Assert(err, IsNil)

	// Changes() has to be closed
	select {
	case _, ok := <-watcher.Changes():
		c.Assert(ok, Equals, false)
	case <-time.After(200 * time.Millisecond):
		// Timeout should not be needed.
		c.Fatalf("config change channel should have been closed due to the illegal data")
	}

	err = watcher.Stop()
	c.Assert(err, ErrorMatches, "YAML error: .*")
}
