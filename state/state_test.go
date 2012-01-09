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
	zkRemoveTree(zk, "/topology")
	zkRemoveTree(zk, "/services")
	zkRemoveTree(zk, "/units")
	zk.Close()
}

// TestReadService tests reading operations on services.
func (s StateSuite) TestReadService(c *C) {
	zk, _ := s.init(c)
	defer s.done(zk, c)

	// Create test scenario.
	zkCreate(zk, "/services", "", c)
	zkCreate(zk, "/services/service-0000000000", "charm: local:series/dummy-1", c)
	zkCreate(zk, "/topology", `
services:
    service-0000000000:
        name: wordpress
`, c)

	// Open state.
	jujuState, err := state.Open(zk)
	c.Assert(err, IsNil)

	// Retrieve legal service.
	service, err := jujuState.Service("wordpress")
	c.Assert(err, IsNil)
	c.Assert(service.Id(), Equals, "service-0000000000")
	c.Assert(service.Name(), Equals, "wordpress")

	// Retrieve charm id of legal service.
	charmId, err := service.CharmId()
	c.Assert(err, IsNil)
	c.Assert(charmId, Equals, "local:series/dummy-1")

	// Retrieve illegal service.
	service, err = jujuState.Service("pressword")
	c.Assert(err, Not(IsNil))
}

// TestReadUnit tests reading on units.
func (s StateSuite) TestReadUnit(c *C) {
	zk, _ := s.init(c)
	defer s.done(zk, c)

	// Create test scenario.
	zkCreate(zk, "/services", "", c)
	zkCreate(zk, "/services/service-0000000000", "charm: local:series/dummy-1", c)
	zkCreate(zk, "/topology", `
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
`, c)
	zkCreate(zk, "/units", "", c)
	zkCreate(zk, "/units/unit-0000000000", "charm: local:series/dummy-1", c)
	zkCreate(zk, "/units/unit-0000000001", "charm: local:series/dummy-1", c)

	// Open state.
	jujuState, err := state.Open(zk)
	c.Assert(err, IsNil)

	// Retrieve legal service.
	service, err := jujuState.Service("wordpress")
	c.Assert(err, IsNil)

	// Retrieve legal unit.
	unit, err := service.Unit("wordpress/0")
	c.Assert(err, IsNil)
	c.Assert(unit.Id(), Equals, "unit-0000000000")
	c.Assert(unit.Name(), Equals, "wordpress/0")

	// Retrieve illegal unit names and illegal units.
	unit, err = service.Unit("wordpress")
	c.Assert(err, Not(IsNil))
	c.Assert(err.Error(), Equals, "state: 'wordpress' is no valid unit name")
	unit, err = service.Unit("wordpress/0/0")
	c.Assert(err, Not(IsNil))
	c.Assert(err.Error(), Equals, "state: 'wordpress/0/0' is no valid unit name")
	unit, err = service.Unit("pressword/0")
	c.Assert(err.Error(), Equals, "state: service name 'pressword' of unit does not match with service name 'wordpress'")
	c.Assert(err, Not(IsNil))

	// Retrieve legal unit directly from state.
	unit, err = jujuState.Unit("wordpress/1")
	c.Assert(err, IsNil)
	c.Assert(unit.Id(), Equals, "unit-0000000001")
	c.Assert(unit.Name(), Equals, "wordpress/1")

	// Retrieve all unit names.
	unitNames, err := service.UnitNames()
	c.Assert(err, IsNil)
	c.Assert(unitNames, Equals, []string{"wordpress/0", "wordpress/1"})

	// Retrieve all units.
	units, err := service.AllUnits()
	c.Assert(err, IsNil)
	c.Assert(len(units), Equals, 2)
}

// TestWriteUnit tests writing on units.
func (s StateSuite) TestWriteUnit(c *C) {
	zk, _ := s.init(c)
	defer s.done(zk, c)

	// Create test scenario.
	zkCreate(zk, "/services", "", c)
	zkCreate(zk, "/services/service-0000000000", "charm: local:series/dummy-1", c)
	zkCreate(zk, "/topology", `
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
`, c)
	zkCreate(zk, "/units", "", c)
	zkCreate(zk, "/units/unit-0000000000", "charm: local:series/dummy-1", c)
	zkCreate(zk, "/units/unit-0000000001", "charm: local:series/dummy-1", c)

	// Open state and get a service.
	jujuState, err := state.Open(zk)
	c.Assert(err, IsNil)
	service, err := jujuState.Service("wordpress")
	c.Assert(err, IsNil)

	// Add a unit.
	unit, err := service.AddUnit()
	c.Assert(err, IsNil)
	c.Assert(unit.Id(), Equals, "unit-0000000002")
	c.Assert(unit.Name(), Equals, "wordpress/2")

	// Remove a legal unit.
	unit, err = service.Unit("wordpress/0")
	c.Assert(err, IsNil)
	err = service.RemoveUnit(unit)
	c.Assert(err, IsNil)
	unitNames, err := service.UnitNames()
	c.Assert(err, IsNil)
	c.Assert(len(unitNames), Equals, 2)

	// Remove an illegal unit.
	err = service.RemoveUnit(unit)
	c.Assert(err, Not(IsNil))
	c.Assert(err.Error(), Equals, "state: service state has changed")
}

// zkRemoveTree recursively removes a tree.
func zkRemoveTree(zk *zookeeper.Conn, path string) error {
	// First recursively delete the cildren.
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
