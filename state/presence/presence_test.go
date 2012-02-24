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

// connect returns a zookeeper connection to addr, and its session event channel.
func connect(c *C, addr string) (*zookeeper.Conn, <-chan zookeeper.Event) {
	zk, session, err := zookeeper.Dial(addr, 15e9)
	c.Assert(err, IsNil)
	c.Assert((<-session).Ok(), Equals, true)
	return zk, session
}

// forward will listen on proxyAddr and connect its client to dstAddr, and return
// a function to kill the connection (or, if not connected, cancel listening).
// This is used for killing zookeeper connections without calling Close; doing
// so when C code is running will cause an unrecoverable panic in C.
func forward(c *C, proxyAddr string, dstAddr string) func() {
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
	return func() { stop <- true }
}

// waitFor blocks until conn knows that path exists. This is necessary because
// distinct zk connections don't always have precisely the same view of the data.
func waitFor(c *C, conn *zookeeper.Conn, path string) {
	stat, watch, err := conn.ExistsW(path)
	c.Assert(err, IsNil)
	if stat != nil {
		return
	}
	// Test code only, local server; if this event *isn't* the node coming into
	// existence, we'll find out soon enough, and we don't want to stay blocked.
	<-watch
}

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
	s.zkTestPort = 21810
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

func (s *PresenceSuite) SetUpTest(c *C) {
	s.zkConn, _ = connect(c, s.zkAddr)
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
	period     = time.Duration(2.5e7) // 25ms
	longEnough = period * 4           // longest possible time to detect a close
	path       = "/presence"
)

func assertChange(c *C, watch <-chan bool, expectAlive bool) {
	t := time.After(longEnough)
	select {
	case <-t:
		c.Log("Liveness change not detected")
		c.FailNow()
	case alive, ok := <-watch:
		c.Assert(ok, Equals, true)
		c.Assert(alive, Equals, expectAlive)
	}
}

func assertClose(c *C, watch <-chan bool) {
	t := time.After(longEnough)
	select {
	case <-t:
		c.Log("Connection loss not detected")
		c.FailNow()
	case _, ok := <-watch:
		c.Assert(ok, Equals, false)
	}
}

func assertNoChange(c *C, watch <-chan bool) {
	t := time.After(longEnough)
	select {
	case <-t:
		return
	case <-watch:
		c.Log("Unexpected liveness change")
		c.FailNow()
	}
}

func (s *PresenceSuite) TestNewPinger(c *C) {
	// Check not considered Alive before it exists.
	alive, err := presence.Alive(s.zkConn, path)
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, false)

	// Watch for life, and check the watch doesn't fire early.
	alive, watch, err := presence.AliveW(s.zkConn, path)
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, false)
	assertNoChange(c, watch)

	// Start a Pinger, and check the watch fires.
	p, err := presence.StartPinger(s.zkConn, path, period)
	c.Assert(err, IsNil)
	defer p.Close()
	assertChange(c, watch, true)

	// Check that Alive agrees.
	alive, err = presence.Alive(s.zkConn, path)
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, true)

	// Watch for life again, and check it doesn't change.
	alive, watch, err = presence.AliveW(s.zkConn, path)
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, true)
	assertNoChange(c, watch)
}

func (s *PresenceSuite) TestKillPinger(c *C) {
	// Start a Pinger and a watch, and check sanity.
	p, err := presence.StartPinger(s.zkConn, path, period)
	c.Assert(err, IsNil)
	alive, watch, err := presence.AliveW(s.zkConn, path)
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, true)
	assertNoChange(c, watch)

	// Kill the Pinger; check the watch fires and Alive agrees.
	p.Kill()
	assertChange(c, watch, false)
	alive, err = presence.Alive(s.zkConn, path)
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, false)

	// Check that the pinger's node was deleted.
	stat, err := s.zkConn.Exists(path)
	c.Assert(err, IsNil)
	c.Assert(stat, IsNil)
}

func (s *PresenceSuite) TestClosePinger(c *C) {
	// Start a Pinger and a watch, and check sanity.
	p, err := presence.StartPinger(s.zkConn, path, period)
	c.Assert(err, IsNil)
	alive, watch, err := presence.AliveW(s.zkConn, path)
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, true)
	assertNoChange(c, watch)

	// Close the Pinger; check the watch fires and Alive agrees.
	p.Close()
	assertChange(c, watch, false)
	alive, err = presence.Alive(s.zkConn, path)
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, false)

	// Check that the pinger's node is still present.
	stat, err := s.zkConn.Exists(path)
	c.Assert(err, IsNil)
	c.Assert(stat, NotNil)
}

func (s *PresenceSuite) TestBadData(c *C) {
	// Create a node that contains inappropriate data.
	_, err := s.zkConn.Create(path, "roflcopter", 0, zookeeper.WorldACL(zookeeper.PERM_ALL))
	c.Assert(err, IsNil)

	// Check it is not interpreted as a presence node by Alive.
	_, err = presence.Alive(s.zkConn, path)
	c.Assert(err, ErrorMatches, ".* is not a valid presence node: .*")

	// Check it is not interpreted as a presence node by Watch.
	_, watch, err := presence.AliveW(s.zkConn, path)
	c.Assert(watch, IsNil)
	c.Assert(err, ErrorMatches, ".* is not a valid presence node: .*")
}

func (s *PresenceSuite) TestDisconnectDeadWatch(c *C) {
	// Create a target node.
	p, err := presence.StartPinger(s.zkConn, path, period)
	c.Assert(err, IsNil)
	p.Close()

	// Start an alternate connection and ensure the node is stale.
	altConn, _ := connect(c, s.zkAddr)
	waitFor(c, altConn, path)
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
	altConn, _ := connect(c, s.zkAddr)
	alive, watch, err := presence.AliveW(altConn, path)
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, false)

	// Kill the watch connection and check it's alerted.
	altConn.Close()
	assertClose(c, watch)
}

func (s *PresenceSuite) TestDisconnectAliveWatch(c *C) {
	// Start a Pinger on the main connection
	p, err := presence.StartPinger(s.zkConn, path, period)
	c.Assert(err, IsNil)
	defer p.Close()

	// Start watching on an alternate connection, forwarded over another
	// connection we can kill safely.
	kill := forward(c, "localhost:21811", s.zkAddr)
	altConn, session := connect(c, "localhost:21811")
	go func() {
		// Assume the first session event will be as a result of the kill below,
		// and explicitly close the connection to clear out the watches. This
		// should be equivalent to what we actually do in a real situation.
		<-session
		altConn.Close()
	}()
	waitFor(c, altConn, path)
	alive, watch, err := presence.AliveW(altConn, path)
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, true)

	// Kill the watch connection and check it's alerted.
	kill()
	assertClose(c, watch)
}

func (s *PresenceSuite) TestDisconnectPinger(c *C) {
	// Start a Pinger on an alternate connection, forwarded over another
	// connection we can kill safely.
	kill := forward(c, "localhost:21811", s.zkAddr)
	altConn, _ := connect(c, "localhost:21811")
	p, err := presence.StartPinger(altConn, path, period)
	c.Assert(err, IsNil)
	defer p.Close()

	// Watch on the "main" connection.
	waitFor(c, s.zkConn, path)
	alive, watch, err := presence.AliveW(s.zkConn, path)
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, true)

	// Kill the pinger connection and check the watch notices.
	kill()
	assertChange(c, watch, false)
}
