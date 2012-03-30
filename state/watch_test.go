package state_test

import (
	. "launchpad.net/gocheck"
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

	watcher := wordpress.WatchConfig()

	// Recieve initial event after creation.
	changedConfig := <-watcher.Changes()
	c.Assert(changedConfig.Keys(), HasLen, 0)

	go func() {
		time.Sleep(100 * time.Millisecond)
		config.Set("foo", "bar")
		config.Set("baz", "yadda")
		config.Write()

		time.Sleep(100 * time.Millisecond)
		config.Delete("foo")
		config.Write()
	}()

	// Receive the two changes.
	changedConfig = <-watcher.Changes()
	c.Assert(changedConfig.Keys(), HasLen, 2)
	foo, found := changedConfig.Get("foo")
	c.Assert(found, Equals, true)
	c.Assert(foo, Equals, "bar")

	changedConfig = <-watcher.Changes()
	c.Assert(changedConfig.Keys(), HasLen, 1)
	foo, found = changedConfig.Get("foo")
	c.Assert(found, Equals, false)
	baz, found := changedConfig.Get("baz")
	c.Assert(found, Equals, true)
	c.Assert(baz, Equals, "yadda")

	// No more changes.
	select {
	case <-watcher.Changes():
		c.Fail()
	case <-time.After(2 * time.Second):
		// The timeout is expected.
	}

	err = watcher.Stop()
	c.Assert(err, IsNil)
}
