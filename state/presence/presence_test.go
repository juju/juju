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
	testing.ZkConnSuite
}

var _ = Suite(&PresenceSuite{})

var (
	path   = "/presence"
	period = 50 * time.Millisecond

	// When hoping to detect a node status change, given a period of 50ms and
	// therefore a timeout of 50ms, the worst-case timeline is:
	//   0ms: Pinger fires for the last time
	//  99ms: watcher checks; sees node is "alive"
	// 199ms: watcher finally times out
	// 200ms: long enough
	// + a little bit for scheduler glitches.
	longEnough = period * 5
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

func (s *PresenceSuite) TestNewPinger(c *C) {
	// Check not considered Alive before it exists.
	alive, err := presence.Alive(s.ZkConn, path)
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, false)

	// Watch for life, and check the watch doesn't fire early.
	alive, watch, err := presence.AliveW(s.ZkConn, path)
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, false)
	assertNoChange(c, watch)

	// Start a Pinger, and check the watch fires.
	p, err := presence.StartPinger(s.ZkConn, path, period)
	c.Assert(err, IsNil)
	assertChange(c, watch, true)

	// Check that Alive agrees.
	alive, err = presence.Alive(s.ZkConn, path)
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, true)

	// Watch for life again, and check it doesn't change.
	alive, watch, err = presence.AliveW(s.ZkConn, path)
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, true)
	assertNoChange(c, watch)

	// Check Dying.
	select {
	case <-p.Dying():
		c.Fatalf("supposedly-alive pinger reported death")
	default:
	}

	// Clean up.
	err = p.Kill()
	c.Assert(err, IsNil)

	// Check Dying.
	select {
	case <-p.Dying():
	default:
		c.Fatalf("supposedly-dead pinger reported life")
	}
}

func (s *PresenceSuite) TestKillPinger(c *C) {
	// Start a Pinger and a watch, and check sanity.
	p, err := presence.StartPinger(s.ZkConn, path, period)
	c.Assert(err, IsNil)
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
}

func (s *PresenceSuite) TestStopPinger(c *C) {
	// Start a Pinger and a watch, and check sanity.
	p, err := presence.StartPinger(s.ZkConn, path, period)
	c.Assert(err, IsNil)
	alive, watch, err := presence.AliveW(s.ZkConn, path)
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, true)
	assertNoChange(c, watch)

	// Stop the Pinger; check the watch fires and Alive agrees.
	err = p.Stop()
	c.Assert(err, IsNil)
	assertChange(c, watch, false)
	alive, err = presence.Alive(s.ZkConn, path)
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, false)

	// Check that the pinger's node is still present.
	stat, err := s.ZkConn.Exists(path)
	c.Assert(err, IsNil)
	c.Assert(stat, NotNil)
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
	assertChange(c, watch, true)

	// Clean up.
	err = p.Kill()
	c.Assert(err, IsNil)
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

	// Start watching on an alternate connection.
	altConn := testing.ZkConnect()
	alive, watch, err := presence.AliveW(altConn, path)
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, true)

	// Kill the watch's connection and check it's alerted.
	altConn.Close()
	assertClose(c, watch)

	// Clean up.
	err = p.Kill()
	c.Assert(err, IsNil)
}

func (s *PresenceSuite) TestDisconnectPinger(c *C) {
	// Start a Pinger on an alternate connection.
	altConn := testing.ZkConnect()
	p, err := presence.StartPinger(altConn, path, period)
	c.Assert(err, IsNil)

	// Watch on the "main" connection.
	alive, watch, err := presence.AliveW(s.ZkConn, path)
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, true)

	// Kill the pinger connection and check the watch notices.
	altConn.Close()
	assertChange(c, watch, false)

	// Check the pinger already knows it broke.
	<-p.Dying()

	// Stop the pinger anyway; check we get an error.
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
