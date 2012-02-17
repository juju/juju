package presence_test

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/gozk/zookeeper"
	"launchpad.net/juju/go/state/presence"
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

func (s *PresenceSuite) connect(c *C) *zookeeper.Conn {
	zk, session, err := zookeeper.Dial(s.zkAddr, 15e9)
	c.Assert(err, IsNil)
	c.Assert((<-session).Ok(), Equals, true)
	return zk
}

func (s *PresenceSuite) SetUpTest(c *C) {
	s.zkConn = s.connect(c)
}

func (s *PresenceSuite) TearDownTest(c *C) {
	// No trees are created in this suite, just top-level nodes; kill what we
	// can see and ignore errors.
	children, _, err := s.zkConn.Children("/")
	if err == nil {
		for _, child := range children {
			s.zkConn.Delete("/"+child, -1)
		}
	}
	s.zkConn.Close()
}

var _ = Suite(&PresenceSuite{})

func (s *PresenceSuite) TestChangeNode(c *C) {
	// Set up infrastructure.
	path := "/change"
	stat, err := s.zkConn.Exists(path)
	c.Assert(err, IsNil)
	c.Assert(stat, IsNil)

	// Check a node gets created.
	node, err := presence.NewChangeNode(s.zkConn, path, "")
	c.Assert(err, IsNil)
	_, _, watch, err := s.zkConn.GetW(path)
	c.Assert(err, IsNil)

	// Check watch fires on Change.
	t0, err := node.Change()
	c.Assert(err, IsNil)
	<-watch

	// Check another Change fires another watch, and returns a later time.
	time.Sleep(1e7)
	_, _, watch, err = s.zkConn.GetW(path)
	t1, err := node.Change()
	<-watch
	c.Assert(t1.After(t0), Equals, true)
}

func (s *PresenceSuite) TestOccupyVacate(c *C) {
	// Set up infrastructure
	path := "/presence"
	timeout := time.Duration(5e7) // 50ms
	longEnough := timeout * 2
	clock, err := presence.NewChangeNode(s.zkConn, "/clock", "")
	c.Assert(err, IsNil)

	// Check that a fresh node is unoccupied.
	client := presence.NewClient(s.zkConn, path)
	occupied, err := client.Occupied(clock)
	c.Assert(err, IsNil)
	c.Assert(occupied, Equals, false)

	// Check that OccupiedW agrees...
	occupied, watch, err := client.OccupiedW(clock)
	c.Assert(err, IsNil)
	c.Assert(occupied, Equals, false)

	// ...and that the watch fires when it gets occupied.
	node, err := presence.Occupy(s.zkConn, path, timeout)
	c.Assert(err, IsNil)
	event := <-watch
	c.Assert(event.Error, IsNil)
	c.Assert(event.Occupied, Equals, true)

	// Check that Occupied sees the node is now occupied...
	occupied, err = client.Occupied(clock)
	c.Assert(err, IsNil)
	c.Assert(occupied, Equals, true)

	// ...and that OccupiedW agrees...
	occupied, watch, err = client.OccupiedW(clock)
	c.Assert(err, IsNil)
	c.Assert(occupied, Equals, true)

	// ...and that the watch doesn't fire while the node remains occupied.
	stillThere := time.After(longEnough)
	select {
	case <-watch:
		c.Log("vacated early")
		c.Fail()
	case <-stillThere:
		// cool, didn't break
	}

	// Vacate the node; check that the watch fires...
	node.Vacate()
	tooLong := time.After(longEnough)
	select {
	case event = <-watch:
		c.Assert(event.Error, IsNil)
		c.Assert(event.Occupied, Equals, false)
	case <-tooLong:
		c.Log("vacation not detected")
		c.Fail()
	}

	// ...and that the node has been deleted...
	stat, err := s.zkConn.Exists(path)
	c.Assert(err, IsNil)
	c.Assert(stat, IsNil)

	// ...and that Occupied still agrees.
	occupied, err = client.Occupied(clock)
	c.Assert(err, IsNil)
	c.Assert(occupied, Equals, false)
}

func (s *PresenceSuite) TestNotAPresenceNode(c *C) {
	// Set up infrastructure.
	path := "/presence"
	clock, err := presence.NewChangeNode(s.zkConn, "/clock", "")
	c.Assert(err, IsNil)
	_, err = s.zkConn.Create(path, "roflcopter", 0, zookeeper.WorldACL(zookeeper.PERM_ALL))
	c.Assert(err, IsNil)

	// Check we can tell it's not a presence node.
	client := presence.NewClient(s.zkConn, path)
	_, err = client.Occupied(clock)
	c.Assert(err, NotNil)
	_, _, err = client.OccupiedW(clock)
	c.Assert(err, NotNil)
}

func (s *PresenceSuite) DONTTestOccupyDie(c *C) {
	// Set up infrastructure.
	path := "/presence"
	timeout := time.Duration(5e7) // 50ms
	longEnough := timeout * 2
	clock, err := presence.NewChangeNode(s.zkConn, "/clock", "")
	c.Assert(err, IsNil)

	// Occupy a presence node with a separate connection.
	otherConn := s.connect(c)
	_, err = presence.Occupy(otherConn, path, timeout)
	c.Assert(err, IsNil)

	// Watch it with the "main" connection.
	client := presence.NewClient(s.zkConn, path)
	occupied, watch, err := client.OccupiedW(clock)
	c.Assert(occupied, Equals, true)

	// Kill the old connection and see whether we detect the vacation.
	otherConn.Close()
	tooLong := time.After(longEnough)
	select {
	case event := <-watch:
		c.Assert(event.Error, IsNil)
		c.Assert(event.Occupied, Equals, false)
	case <-tooLong:
		c.Log("vacation not detected")
		c.Fail()
	}

	// Double check the node really is still there...
	stat, err := s.zkConn.Exists(path)
	c.Assert(stat, NotNil)

	// ...but we still don't think it's occupied.
	occupied, err = client.Occupied(clock)
	c.Assert(err, IsNil)
	c.Assert(occupied, Equals, false)
}
