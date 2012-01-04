// launchpad.net/juju/go/state
//
// Copyright (c) 2011 Canonical Ltd.

package state

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/gozk/zookeeper"
	"testing"
)

// zkCreate is a simple helper to create test scenarios in ZooKeeper.
func zkCreate(zk *zookeeper.Conn, p, v string, c *C) {
	if _, err := zk.Create(p, v, 0, zookeeper.WorldACL(zookeeper.PERM_ALL)); err != nil {
		c.Fatal("Cannot set path '"+p+"' in ZooKeeper: ", err)
	}
}

// TestPackage integrates the tests into gotest.
func TestPackage(t *testing.T) {
	TestingT(t)
}

// StateSuite for State and the related types.
type StateSuite struct {
	zkServer       *zookeeper.Server
	zkTestRoot     string
	zkTestPort     int
	zkAddr         string
	handles        []*zookeeper.Conn
	events         []*zookeeper.Event
	liveEventChans int
	deadEventChans chan bool
}

var _ = Suite(&StateSuite{})

// SetUpSuite starts and inits ZooKeeper.
func (s *StateSuite) SetUpSuite(c *C) {
	var err error
	s.zkTestRoot = c.MkDir() + "/zookeeper"
	s.zkTestPort = 21812
	s.zkAddr = fmt.Sprint("localhost:", s.zkTestPort)
	s.deadEventChans = make(chan bool)

	// Create server.
	s.zkServer, err = zookeeper.CreateServer(s.zkTestPort, s.zkTestRoot, "")
	if err != nil {
		c.Fatal("Cannot set up ZooKeeper server environment: ", err)
	}

	// Start server.
	err = s.zkServer.Start()
	if err != nil {
		c.Fatal("Cannot start ZooKeeper server: ", err)
	}
}

// TearDownSuite stops ZooKeeper.
func (s *StateSuite) TearDownSuite(c *C) {
	if s.zkServer != nil {
		s.zkServer.Destroy()
	}
}

// init establishes a connection to ZooKeeper returns it together with the
// event channel.
func (s *StateSuite) init(c *C) (*zookeeper.Conn, <-chan zookeeper.Event) {
	// Connect the server.
	conn, eventChan, err := zookeeper.Dial(s.zkAddr, 5e9)
	c.Assert(err, IsNil)

	// Wait for connect signal.
	event := <-eventChan
	c.Assert(event.Type, Equals, zookeeper.EVENT_SESSION)
	c.Assert(event.State, Equals, zookeeper.STATE_CONNECTED)

	return conn, eventChan
}

// done closes a ZooKeeper connection.
func (s *StateSuite) done(zk *zookeeper.Conn, c *C) {
	// Delete possible nodes, ignore errors.
	zk.Delete("/services/service-0", -1)
	zk.Delete("/services", -1)
	zk.Delete("/topology", -1)
	zk.Delete("/units", -1)
	zk.Close()
}

// TestReadService tests reading operations on services.
func (s StateSuite) TestReadService(c *C) {
	zk, _ := s.init(c)
	defer s.done(zk, c)

	// Create test scenario.
	zkCreate(zk, "/services", "", c)
	zkCreate(zk, "/services/service-0", "charm: local:series/dummy-1", c)
	zkCreate(zk, "/topology", `
services:
    service-0:
        name: wordpress
`, c)

	// Open state.
	state, err := Open(zk)
	c.Assert(err, IsNil)

	// Retrieve legal service.
	service, err := state.Service("wordpress")
	c.Assert(err, IsNil)
	c.Assert(service.Id(), Equals, "service-0")
	c.Assert(service.Name(), Equals, "wordpress")

	// Retrieve charm id of legal service.
	charmId, err := service.CharmId()
	c.Assert(err, IsNil)
	c.Assert(charmId, Equals, "local:series/dummy-1")

	// Retrieve illegal service.
	service, err = state.Service("pressword")
	c.Assert(err, Not(IsNil))
}

// TestReadUnit tests reading on units.
func (s StateSuite) TestReadUnit(c *C) {
	zk, _ := s.init(c)
	defer s.done(zk, c)

	// Create test scenario.
	zkCreate(zk, "/services", "", c)
	zkCreate(zk, "/services/service-0", "charm: local:series/dummy-1", c)
	zkCreate(zk, "/topology", `
services:
    service-0:
        name: wordpress
        units:
            unit-0:
                sequence: 0
`, c)
	zkCreate(zk, "/units", "", c)
	zkCreate(zk, "/units/unit-0", "charm: local:series/dummy-1", c)

	// Open state.
	state, err := Open(zk)
	c.Assert(err, IsNil)

	// Retrieve legal service.
	service, err := state.Service("wordpress")
	c.Assert(err, IsNil)

	// Retrieve legal unit.
	unit, err := service.Unit("wordpress/0")
	c.Assert(err, IsNil)
	c.Assert(unit.Id(), Equals, "unit-0")
	c.Assert(unit.Name(), Equals, "wordpress/0")

	// Retrieve illegal unit names and illegal units.
	unit, err = service.Unit("wordpress")
	c.Assert(err, Not(IsNil))
	unit, err = service.Unit("wordpress/0/0")
	c.Assert(err, Not(IsNil))
	unit, err = service.Unit("pressword/0")
	c.Assert(err, Not(IsNil))

	// Retrieve legal unit directly from state.
	unit, err = state.Unit("wordpress/0")
	c.Assert(err, IsNil)
	c.Assert(unit.Id(), Equals, "unit-0")
	c.Assert(unit.Name(), Equals, "wordpress/0")
}
