package presence_test

import (
	"fmt"
	"io"
	. "launchpad.net/gocheck"
	"launchpad.net/gozk/zookeeper"
	"launchpad.net/juju/go/state/presence"
	"net"
	"testing"
	"time"
)

func Test(t *testing.T) { TestingT(t) }

type PresenceSuite struct {
	zkServer   *zookeeper.Server
	zkTestRoot string
	zkTestPort int
	zkAddr     string
	zkConn     *zookeeper.Conn
}

func (s *PresenceSuite) SetUpSuite(c *C) {
	var err error
	s.zkTestRoot = c.MkDir() + "/zookeeper"
	s.zkTestPort = 21812
	s.zkAddr = fmt.Sprint("localhost:", s.zkTestPort)

	s.zkServer, err = zookeeper.CreateServer(s.zkTestPort, s.zkTestRoot, "")
	if err != nil {
		c.Fatal("Cannot set up ZooKeeper server environment: ", err)
	}
	err = s.zkServer.Start()
	if err != nil {
		c.Fatal("Cannot start ZooKeeper server: ", err)
	}
}

func (s *PresenceSuite) TearDownSuite(c *C) {
	if s.zkServer != nil {
		s.zkServer.Destroy()
	}
}
func (s *PresenceSuite) connect(c *C, addr string) *zookeeper.Conn {
	zk, session, err := zookeeper.Dial(addr, 15e9)
	c.Assert(err, IsNil)
	c.Assert((<-session).Ok(), Equals, true)
	return zk
}

func (s *PresenceSuite) SetUpTest(c *C) {
	s.zkConn = s.connect(c, s.zkAddr)
}

func (s *PresenceSuite) TearDownTest(c *C) {
	// No need to recurse in this suite; just try to delete what we can see.
	children, _, err := s.zkConn.Children("/")
	if err == nil {
		for _, child := range children {
			s.zkConn.Delete("/"+child, -1)
		}
	}
	s.zkConn.Close()
}

var (
	_          = Suite(&PresenceSuite{})
	timeout    = time.Duration(5e7) // 50ms
	longEnough = timeout * 2
	path       = "/presence"
)

func (s *PresenceSuite) TestAliveWatchPinger(c *C) {
	// Check not considered Alive before it exists.
	alive, err := presence.Alive(s.zkConn, path)
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, false)

	// Watch for life.
	w, err := presence.Watch(s.zkConn, path)
	c.Assert(err, IsNil)
	c.Assert(w.Error, IsNil)
	c.Assert(w.Alive, Equals, false)

	// Start a Pinger.
	p, err := presence.StartPing(s.zkConn, path, timeout)
	c.Assert(err, IsNil)

	// Check the watch fires.
	t := time.After(longEnough)
	select {
	case <-t:
		c.Log("liveness not detected")
		c.FailNow()
	case alive = <-w.C:
		c.Assert(alive, Equals, true)
		c.Assert(w.Error, IsNil)
	}

	// Check that Alive agrees.
	alive, err = presence.Alive(s.zkConn, path)
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, true)

	// Check that a living Pinger doesn't fire the watch again.
	t = time.After(longEnough)
	select {
	case <-t:
	case <-w.C:
		c.Log("unexpected Alive change")
		c.FailNow()
	}

	// Kill the Pinger, check the watch does fire.
	p.Close()
	t = time.After(longEnough)
	select {
	case <-t:
		c.Log("deadness not detected")
		c.FailNow()
	case alive, ok := <-w.C:
		c.Assert(alive, Equals, false)
		c.Assert(ok, Equals, true)
		c.Assert(w.Error, IsNil)
	}

	// Check Alive agrees it's not alive.
	alive, err = presence.Alive(s.zkConn, path)
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, false)

	// Start a new Pinger and check the watch fires again.
	p, err = presence.StartPing(s.zkConn, path, timeout)
	c.Assert(err, IsNil)
	t = time.After(longEnough)
	select {
	case <-t:
		c.Log("liveness not detected")
		c.FailNow()
	case alive = <-w.C:
		c.Assert(alive, Equals, true)
		c.Assert(w.Error, IsNil)
	}

	// Stop the watcher.
	w.Close()
	_, ok := <-w.C
	c.Assert(ok, Equals, false)
	c.Assert(w.Error, ErrorMatches, "stopped on request")

	// Close the Pinger and check the path is deleted as expected.
	p.Close()
	stat, err := s.zkConn.Exists(path)
	c.Assert(err, IsNil)
	c.Assert(stat, IsNil)

	// Verify that the Watcher didn't notice the Pinger closing.
	c.Assert(w.Alive, Equals, true)
}

func (s *PresenceSuite) TestBadData(c *C) {
	// Create a node that contains inappropriate data.
	_, err := s.zkConn.Create(path, "roflcopter", 0, zookeeper.WorldACL(zookeeper.PERM_ALL))
	c.Assert(err, IsNil)

	// Check it is not interpreted as a presence node by Alive.
	_, err = presence.Alive(s.zkConn, path)
	c.Assert(err, ErrorMatches, ".* is not a valid presence node: .*")

	// Check it is not interpreted as a presence node by Watch.
	w, err := presence.Watch(s.zkConn, path)
	c.Assert(w, IsNil)
	c.Assert(err, ErrorMatches, ".* is not a valid presence node: .*")
}

func (s *PresenceSuite) TestOldNode(c *C) {
	// Create something that looks like a presence node, but is not being
	// refreshed by a Pinger.
	_, err := s.zkConn.Create(path, timeout.String(), 0, zookeeper.WorldACL(zookeeper.PERM_ALL))
	c.Assert(err, IsNil)
	time.Sleep(longEnough)

	// Check it is interpreted as dead by Alive.
	alive, err := presence.Alive(s.zkConn, path)
	c.Assert(alive, Equals, false)
	c.Assert(err, IsNil)

	// Check it is interpreted as dead by Watch.
	w, err := presence.Watch(s.zkConn, path)
	c.Assert(w.Alive, Equals, false)
	c.Assert(err, IsNil)

	// Start a Pinger.
	p, err := presence.StartPing(s.zkConn, path, timeout)
	c.Assert(err, IsNil)

	// Check that the Watcher sees the Pinger coming up.
	t := time.After(longEnough)
	select {
	case <-t:
		c.Log("aliveness not detected")
		c.FailNow()
	case alive = <-w.C:
		c.Assert(alive, Equals, true)
		c.Assert(w.Alive, Equals, true)
		c.Assert(w.Error, IsNil)
	}

	// Close watcher before pinger to avoid blocking on send to w.C.
	w.Close()
	p.Close()
}

// forward will listen on proxyAddr and connect its client to dstAddr, and return
// a channel which terminates the connection when it receives a value.
func forward(c *C, proxyAddr string, dstAddr string) chan bool {
	// This is necessary so I can close the alternate zk connection in
	// TestDetectTimeout *without* explicitly closing the zookeeper.Conn itself
	// (which causes an unrecoverable panic (in C) when Pinger tries to use the
	// closed connection).
	stop := make(chan bool)
	go func() {
		var client net.Conn
		accepted := make(chan bool)
		go func() {
			listener, err := net.Listen("tcp", proxyAddr)
			c.Assert(err, IsNil)
			defer listener.Close()
			client, err = listener.Accept()
			c.Assert(err, IsNil)
			accepted <- true
		}()
		select {
		case <-accepted:
			defer client.Close()
		case <-stop:
			return
		}
		server, err := net.Dial("tcp", dstAddr)
		c.Assert(err, IsNil)
		defer server.Close()
		go io.Copy(client, server)
		go io.Copy(server, client)
		<-stop
	}()
	return stop
}

func (s *PresenceSuite) TestDetectTimeout(c *C) {
	// Start a Pinger on an alternate connection.
	done := forward(c, "localhost:21813", s.zkAddr)
	altConn := s.connect(c, "localhost:21813")
	_, err := presence.StartPing(altConn, path, timeout)
	c.Assert(err, IsNil)

	// Watch the Pinger on the "main" connection.
	w, err := presence.Watch(s.zkConn, path)
	c.Assert(err, IsNil)
	c.Assert(w.Alive, Equals, true)

	// Kill the Pinger's connection, and check the Watcher detects the dead Pinger.
	done <- true
	t := time.After(longEnough)
	select {
	case <-t:
		c.Log("deadness not detected")
		c.FailNow()
	case alive := <-w.C:
		c.Assert(alive, Equals, false)
		c.Assert(w.Alive, Equals, false)
		c.Assert(w.Error, IsNil)
	}

	// Start a new Pinger, and check the Watcher detects it.
	p, err := presence.StartPing(s.zkConn, path, timeout)
	c.Assert(err, IsNil)
	t = time.After(longEnough)
	select {
	case <-t:
		c.Log("liveness not detected")
		c.FailNow()
	case alive := <-w.C:
		c.Assert(alive, Equals, true)
		c.Assert(w.Alive, Equals, true)
		c.Assert(w.Error, IsNil)
	}

	// Close watcher before pinger to avoid blocking on send to w.C.
	w.Close()
	p.Close()
}
