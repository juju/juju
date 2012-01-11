// launchpad.net/juju/go/state
//
// Copyright (c) 2011-2012 Canonical Ltd.

package state_test

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/gozk/zookeeper"
	"launchpad.net/juju/go/state"
	"testing"
)

// zkCreate is a simple helper to create a node with a value based
// on the path. It uses standard parameters for ZooKeeper and the
// test fails when the node can't be created.
func zkCreate(c *C, zk *zookeeper.Conn, path, value string) {
	if _, err := zk.Create(path, value, 0, zookeeper.WorldACL(zookeeper.PERM_ALL)); err != nil {
		c.Fatal("Cannot set path '"+path+"' in ZooKeeper: ", err)
	}
}

// TestPackage integrates the tests into gotest.
func TestPackage(t *testing.T) {
	TestingT(t)
}

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

func (s *StateSuite) SetUpSuite(c *C) {
	var err error
	s.zkTestRoot = c.MkDir() + "/zookeeper"
	s.zkTestPort = 21812
	s.zkAddr = fmt.Sprint("localhost:", s.zkTestPort)
	s.deadEventChans = make(chan bool)

	s.zkServer, err = zookeeper.CreateServer(s.zkTestPort, s.zkTestRoot, "")
	if err != nil {
		c.Fatal("Cannot set up ZooKeeper server environment: ", err)
	}

	err = s.zkServer.Start()
	if err != nil {
		c.Fatal("Cannot start ZooKeeper server: ", err)
	}
}

func (s *StateSuite) TearDownSuite(c *C) {
	if s.zkServer != nil {
		s.zkServer.Destroy()
	}
}

// init establishes a connection to ZooKeeper and returns it together with the
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

// done removes potentially created ZooKeeper nodes
// recursively and then closes the ZooKeeper connection. 
func (s *StateSuite) done(zk *zookeeper.Conn, c *C) {
	// Delete possible nodes, ignore errors.
	zkRemoveTree(zk, "/topology")
	zkRemoveTree(zk, "/services")
	zkRemoveTree(zk, "/units")
	zk.Close()
}

func (s StateSuite) TestReadService(c *C) {
	zk, _ := s.init(c)
	defer s.done(zk, c)

	// Create test scenario.
	zkCreate(c, zk, "/services", "")
	zkCreate(c, zk, "/services/service-0000000000", "charm: local:series/dummy-1")
	zkCreate(c, zk, "/topology", `
services:
    service-0000000000:
        name: wordpress
`)

	st, err := state.Open(zk)
	c.Assert(err, IsNil)

	// Check that retrieving a service works correctly.
	service, err := st.Service("wordpress")
	c.Assert(err, IsNil)
	c.Assert(service.Name(), Equals, "wordpress")
	charmId, err := service.CharmId()
	c.Assert(err, IsNil)
	c.Assert(charmId, Equals, "local:series/dummy-1")

	// Check that retrieving a non-existent service fails nicely.
	service, err = st.Service("pressword")
	c.Assert(err, ErrorMatches, `service with name "pressword" cannot be found`)
}

func (s StateSuite) TestReadUnit(c *C) {
	zk, _ := s.init(c)
	defer s.done(zk, c)

	// Create test scenario.
	zkCreate(c, zk, "/services", "")
	zkCreate(c, zk, "/services/service-0000000000", "charm: local:series/dummy-1")
	zkCreate(c, zk, "/topology", `
services:
    service-0000000000:
        name: wordpress
        units:
            unit-0000000000:
                sequence: 0
            unit-0000000001:
                sequence: 1
unit-sequence:
    wordpress: 2
`)
	zkCreate(c, zk, "/units", "")
	zkCreate(c, zk, "/units/unit-0000000000", "charm: local:series/dummy-1")
	zkCreate(c, zk, "/units/unit-0000000001", "charm: local:series/dummy-1")

	st, err := state.Open(zk)
	c.Assert(err, IsNil)
	service, err := st.Service("wordpress")
	c.Assert(err, IsNil)

	// Check that retrieving a unit works correctly.
	unit, err := service.Unit("wordpress/0")
	c.Assert(err, IsNil)
	c.Assert(unit.Name(), Equals, "wordpress/0")

	// Check that retrieving a non-existent or an invalidly
	// named unit fail nicely.
	unit, err = service.Unit("wordpress")
	c.Assert(err, ErrorMatches, `"wordpress" is no valid unit name`)
	unit, err = service.Unit("wordpress/0/0")
	c.Assert(err, ErrorMatches, `"wordpress/0/0" is no valid unit name`)
	unit, err = service.Unit("pressword/0")
	c.Assert(err, ErrorMatches, `can't find unit "pressword/0" on service "wordpress"`)

	// Check that retrieving a unit works.
	unit, err = st.Unit("wordpress/1")
	c.Assert(err, IsNil)
	c.Assert(unit.Name(), Equals, "wordpress/1")

	// Check that retrieving unit names works.
	unitNames, err := service.UnitNames()
	c.Assert(err, IsNil)
	c.Assert(unitNames, Equals, []string{"wordpress/0", "wordpress/1"})

	// Check that retrieving all units works.
	units, err := service.AllUnits()
	c.Assert(err, IsNil)
	c.Assert(len(units), Equals, 2)
	c.Assert(units[0].Name(), Equals, "wordpress/0")
	c.Assert(units[1].Name(), Equals, "wordpress/1")
}

func (s StateSuite) TestWriteUnit(c *C) {
	zk, _ := s.init(c)
	defer s.done(zk, c)

	// Create test scenario.
	zkCreate(c, zk, "/services", "")
	zkCreate(c, zk, "/services/service-0000000000", "charm: local:series/dummy-1")
	zkCreate(c, zk, "/topology", `
services:
    service-0000000000:
        name: wordpress
        units:
            unit-0000000000:
                sequence: 0
                machine: machine-00000000
            unit-0000000001:
                sequence: 1
unit-sequence:
    wordpress: 2
machines:
    machine-00000000:
`)
	zkCreate(c, zk, "/units", "")
	zkCreate(c, zk, "/units/unit-0000000000", "charm: local:series/dummy-1")
	zkCreate(c, zk, "/units/unit-0000000001", "charm: local:series/dummy-1")

	st, err := state.Open(zk)
	c.Assert(err, IsNil)
	service, err := st.Service("wordpress")
	c.Assert(err, IsNil)

	// Check that adding a unit works.
	unit, err := service.AddUnit()
	c.Assert(err, IsNil)
	c.Assert(unit.Name(), Equals, "wordpress/2")

	// Check that removing a unit works.
	unit, err = service.Unit("wordpress/0")
	c.Assert(err, IsNil)
	err = service.RemoveUnit(unit)
	c.Assert(err, IsNil)
	unitNames, err := service.UnitNames()
	c.Assert(err, IsNil)
	c.Assert(unitNames, Equals, []string{"wordpress/1", "wordpress/2"})

	// Check that removing a non-existent unit fails nicely.
	err = service.RemoveUnit(unit)
	c.Assert(err, ErrorMatches, "environment state has changed")
}

// zkRemoveTree recursively removes a tree.
func zkRemoveTree(zk *zookeeper.Conn, path string) error {
	// First recursively delete the children.
	children, _, err := zk.Children(path)
	if err != nil {
		return err
	}
	for _, child := range children {
		if err = zkRemoveTree(zk, fmt.Sprintf("%s/%s", path, child)); err != nil {
			return err
		}
	}
	// Now delete the path itself.
	return zk.Delete(path, -1)
}
