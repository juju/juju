package presence_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/gozk/zookeeper"
	"launchpad.net/juju-core/state/presence"
	"launchpad.net/juju-core/testing"
	stdtesting "testing"
	"time"
)

func TestPackage(t *stdtesting.T) {
	testing.ZkTestPackage(t)
}

type PresenceSuite struct {
	testing.LoggingSuite
	testing.ZkConnSuite
}

var _ = Suite(&PresenceSuite{})

func (s *PresenceSuite) TearDownTest(c *C) {
	s.ZkConnSuite.TearDownTest(c)
	s.LoggingSuite.TearDownTest(c)
}

var (
	path       = "/presence"
	period     = 500 * time.Millisecond
	longEnough = period * 6
)

func assertChange(c *C, watch <-chan bool, expectAlive bool) {
	select {
	case <-time.After(longEnough):
		c.Fatal("Liveness change not detected")
	case alive, ok := <-watch:
		c.Assert(ok, Equals, true)
		c.Assert(alive, Equals, expectAlive)
	}
}

func assertClose(c *C, watch <-chan bool) {
	select {
	case <-time.After(longEnough):
		c.Fatal("Connection loss not detected")
	case _, ok := <-watch:
		c.Assert(ok, Equals, false)
	}
}

func assertNoChange(c *C, watch <-chan bool) {
	select {
	case <-time.After(longEnough):
		return
	case <-watch:
		c.Fatal("Unexpected liveness change")
	}
}

func kill(c *C, p *presence.Pinger) {
	c.Assert(p.Kill(), IsNil)
}

func (s *PresenceSuite) TestStartPinger(c *C) {
	// Check not considered Alive before it exists.
	alive, err := presence.Alive(s.ZkConn, path)
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, false)

	// Watch for life, and check the watch doesn't fire early.
	alive, watch, err := presence.AliveW(s.ZkConn, path)
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, false)
	assertNoChange(c, watch)

	// Creating a Pinger does not automatically Start it.
	p := presence.NewPinger(s.ZkConn, path, period)
	c.Assert(err, IsNil)
	assertNoChange(c, watch)

	// Pinger Start should be detected on the change channel.
	err = p.Start()
	c.Assert(err, IsNil)
	assertChange(c, watch, true)

	// Check that Alive agrees.
	alive, err = presence.Alive(s.ZkConn, path)
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, true)

	// Watch for life again, and check that agrees.
	alive, watch, err = presence.AliveW(s.ZkConn, path)
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, true)
	assertNoChange(c, watch)

	// Starting the pinger when it is already running is not allowed.
	bad := func() { p.Start() }
	c.Assert(bad, PanicMatches, "pinger is already started")

	// Clean up.
	err = p.Kill()
	c.Assert(err, IsNil)
}

func (s *PresenceSuite) TestKillPinger(c *C) {
	// Start a Pinger and a watch, and check sanity.
	p, err := presence.StartPinger(s.ZkConn, path, period)
	c.Assert(err, IsNil)
	defer kill(c, p)
	alive, watch, err := presence.AliveW(s.ZkConn, path)
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, true)
	assertNoChange(c, watch)

	// Kill the Pinger; check the watch fires and Alive agrees.
	err = p.Kill()
	c.Assert(err, IsNil)
	assertChange(c, watch, false)
	alive, err = presence.Alive(s.ZkConn, path)
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, false)

	// Check that the pinger's node was deleted.
	stat, err := s.ZkConn.Exists(path)
	c.Assert(err, IsNil)
	c.Assert(stat, IsNil)

	// Check the pinger can no longer be used:
	bad := func() { p.Start() }
	c.Assert(bad, PanicMatches, "pinger has been killed")
	bad = func() { p.Stop() }
	c.Assert(bad, PanicMatches, "pinger has been killed")
}

func (s *PresenceSuite) TestStopPinger(c *C) {
	// Start a Pinger and a watch, and check sanity.
	p, err := presence.StartPinger(s.ZkConn, path, period)
	c.Assert(err, IsNil)
	defer kill(c, p)
	alive, watch, err := presence.AliveW(s.ZkConn, path)
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, true)
	assertNoChange(c, watch)

	// Stop the Pinger; check the watch fires and Alive agrees.
	err = p.Stop()
	c.Assert(err, IsNil)
	assertChange(c, watch, false)
	alive, watch, err = presence.AliveW(s.ZkConn, path)
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, false)

	// Check that we can Stop again.
	err = p.Stop()
	c.Assert(err, IsNil)

	// Check that the pinger's node is still present, but no changes have been seen.
	stat, err := s.ZkConn.Exists(path)
	c.Assert(err, IsNil)
	c.Assert(stat, NotNil)
	assertNoChange(c, watch)

	// Start the Pinger again, and check the change is detected.
	err = p.Start()
	c.Assert(err, IsNil)
	assertChange(c, watch, true)
}

func (s *PresenceSuite) TestWatchDeadnessChange(c *C) {
	// Create a stale node.
	p, err := presence.StartPinger(s.ZkConn, path, period)
	c.Assert(err, IsNil)
	err = p.Stop()
	c.Assert(err, IsNil)
	time.Sleep(longEnough)

	// Start watching for liveness.
	alive, watch, err := presence.AliveW(s.ZkConn, path)
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, false)

	// Delete the node and check the watch doesn't fire.
	err = s.ZkConn.Delete(path, -1)
	c.Assert(err, IsNil)
	assertNoChange(c, watch)

	// Start a new Pinger and check the watch does fire.
	p, err = presence.StartPinger(s.ZkConn, path, period)
	c.Assert(err, IsNil)
	defer kill(c, p)
	assertChange(c, watch, true)
}

func (s *PresenceSuite) TestBadData(c *C) {
	// Create a node that contains inappropriate data.
	_, err := s.ZkConn.Create(path, "roflcopter", 0, zookeeper.WorldACL(zookeeper.PERM_ALL))
	c.Assert(err, IsNil)

	// Check it is not interpreted as a presence node by Alive.
	_, err = presence.Alive(s.ZkConn, path)
	c.Assert(err, ErrorMatches, `/presence presence node has bad data: "roflcopter"`)

	// Check it is not interpreted as a presence node by Watch.
	_, watch, err := presence.AliveW(s.ZkConn, path)
	c.Assert(watch, IsNil)
	c.Assert(err, ErrorMatches, `/presence presence node has bad data: "roflcopter"`)
}

func (s *PresenceSuite) TestDisconnectDeadWatch(c *C) {
	// Create a target node.
	p, err := presence.StartPinger(s.ZkConn, path, period)
	c.Assert(err, IsNil)
	err = p.Stop()
	c.Assert(err, IsNil)

	// Start an alternate connection and ensure the node is stale.
	altConn := testing.ZkConnect()
	time.Sleep(longEnough)

	// Start a watch using the alternate connection.
	alive, watch, err := presence.AliveW(altConn, path)
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, false)

	// Kill the watch connection and check it's alerted.
	altConn.Close()
	assertClose(c, watch)
}

func (s *PresenceSuite) TestDisconnectMissingWatch(c *C) {
	// Don't even create a target node.

	// Start watching on an alternate connection.
	altConn := testing.ZkConnect()
	alive, watch, err := presence.AliveW(altConn, path)
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, false)

	// Kill the watch's connection and check it's alerted.
	altConn.Close()
	assertClose(c, watch)
}

func (s *PresenceSuite) TestDisconnectAliveWatch(c *C) {
	// Start a Pinger on the main connection
	p, err := presence.StartPinger(s.ZkConn, path, period)
	c.Assert(err, IsNil)
	defer kill(c, p)

	// Start watching on an alternate connection.
	altConn := testing.ZkConnect()
	alive, watch, err := presence.AliveW(altConn, path)
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, true)

	// Kill the watch's connection and check it's alerted.
	altConn.Close()
	assertClose(c, watch)
}

func (s *PresenceSuite) TestDisconnectPinger(c *C) {
	// Start a Pinger on an alternate connection.
	altConn := testing.ZkConnect()
	p, err := presence.StartPinger(altConn, path, period)
	c.Assert(err, IsNil)
	defer p.Kill()

	// Watch on the "main" connection.
	alive, watch, err := presence.AliveW(s.ZkConn, path)
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, true)

	// Kill the pinger connection and check the watch notices.
	altConn.Close()
	assertChange(c, watch, false)

	// Stop the pinger anyway; check we still get an error.
	err = p.Stop()
	c.Assert(err, NotNil)
}

func (s *PresenceSuite) TestWaitAlive(c *C) {
	err := presence.WaitAlive(s.ZkConn, path, longEnough)
	c.Assert(err, ErrorMatches, "presence: still not alive after timeout")

	dying := make(chan struct{})
	dead := make(chan struct{})

	// Start a pinger with a short delay so that WaitAlive() has to wait.
	go func() {
		time.Sleep(period * 2)
		p, err := presence.StartPinger(s.ZkConn, path, period)
		c.Assert(err, IsNil)
		<-dying
		err = p.Kill()
		c.Assert(err, IsNil)
		close(dead)
	}()

	// Wait for, and check, liveness.
	err = presence.WaitAlive(s.ZkConn, path, longEnough)
	c.Assert(err, IsNil)
	close(dying)
	<-dead
}

func (s *PresenceSuite) TestDisconnectWaitAlive(c *C) {
	// Start a new connection with a short lifespan.
	altConn := testing.ZkConnect()
	go func() {
		time.Sleep(period * 2)
		altConn.Close()
	}()

	// Check that WaitAlive returns an appropriate error.
	err := presence.WaitAlive(altConn, path, longEnough)
	c.Assert(err, ErrorMatches, "presence: channel closed while waiting")
}

func (s *PresenceSuite) TestChildrenWatcher(c *C) {
	w := presence.NewChildrenWatcher(s.ZkConn, "/nodes")

	// Check initial event.
	assertChange := func(added, removed []string) {
		select {
		case ch, ok := <-w.Changes():
			c.Assert(ok, Equals, true)
			c.Assert(ch.Added, DeepEquals, added)
			c.Assert(ch.Removed, DeepEquals, removed)
		case <-time.After(longEnough):
			c.Fatalf("expected change, got none")
		}
	}
	assertChange(nil, nil)
	assertNoChange := func() {
		select {
		case ch := <-w.Changes():
			c.Fatalf("got unexpected change: %#v", ch)
		case <-time.After(longEnough):
		}
	}
	assertNoChange()

	// Create the node we're watching, check no change.
	_, err := s.ZkConn.Create("/nodes", "", 0, zookeeper.WorldACL(zookeeper.PERM_ALL))
	c.Assert(err, IsNil)
	assertNoChange()

	// Add a pinger, check it's noticed.
	p1, err := presence.StartPinger(s.ZkConn, "/nodes/p1", period)
	c.Assert(err, IsNil)
	defer kill(c, p1)
	assertChange([]string{"p1"}, nil)
	assertNoChange()

	// Add another pinger, check it's noticed.
	p2, err := presence.StartPinger(s.ZkConn, "/nodes/p2", period)
	c.Assert(err, IsNil)
	defer kill(c, p2)
	assertChange([]string{"p2"}, nil)

	// Stop watcher, check closed.
	err = w.Stop()
	c.Assert(err, IsNil)
	assertClosed := func() {
		select {
		case _, ok := <-w.Changes():
			c.Assert(ok, Equals, false)
		case <-time.After(longEnough):
			c.Fatalf("changes channel not closed")
		}
	}
	assertClosed()

	// Stop a pinger, wait long enough for it to be considered dead.
	err = p1.Stop()
	c.Assert(err, IsNil)
	<-time.After(longEnough)

	// Start a new watcher, check initial event.
	w = presence.NewChildrenWatcher(s.ZkConn, "/nodes")
	assertChange([]string{"p2"}, nil)
	assertNoChange()

	// Kill the remaining pinger, check it's noticed.
	err = p2.Kill()
	assertChange(nil, []string{"p2"})
	assertNoChange()

	// A few times, initially starting with p1 abandoned and subsequently
	// with it deleted:
	for i := 0; i < 3; i++ {
		// Start a new pinger on p1 and check its presence is noted...
		p1, err = presence.StartPinger(s.ZkConn, "/nodes/p1", period)
		c.Assert(err, IsNil)
		defer kill(c, p1)
		assertChange([]string{"p1"}, nil)
		assertNoChange()

		// ...and so is its absence.
		err = p1.Kill()
		assertChange(nil, []string{"p1"})
		assertNoChange()
	}

	// Stop the watcher, check closed again.
	err = w.Stop()
	c.Assert(err, IsNil)
	assertClosed()
}
