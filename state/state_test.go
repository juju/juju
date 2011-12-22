// launchpad.net/juju/go/state
//
// Copyright (c) 2011 Canonical Ltd.

package state

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/goyaml"
	"launchpad.net/gozk/zookeeper"
	"testing"
	"time"
)

// testTopology is the starting topology as YAML string.
const testTopology = `
services:
    s-0:
        name: service-zero
        charm: my-charm-zero
        units:
            u-0:
                sequence: 0
            u-1:
                sequence: 1
    s-1:
        name: service-one
        charm: my-charm-one
        units:
    s-2:
        name: service-two
        charm: my-charm-two
        units:
unit-sequence:
    service-zero: 2
    service-one: 0
    service-two: 0
`

// modifiedTestTopology is the starting topology as YAML string.
const modifiedTestTopology = `
services:
    s-0:
        name: service-zero
        charm: my-charm-zero-modified
        units:
            u-0:
                sequence: 0
            u-1:
                sequence: 1
    s-1:
        name: service-one
        charm: my-charm-one
        units:
    s-2:
        name: service-two
        charm: my-charm-two
        units:
unit-sequence:
    service-zero: 2
    service-one: 0
    service-two: 0
`

// setTopology sets the topology nodes in ZooKeeper.
func setTopology(zk *zookeeper.Conn, topology string, c *C) {
	cf := func(ov string, os *zookeeper.Stat) (string, error) {
		return topology, nil
	}

	if err := zk.RetryChange("/topology", zookeeper.EPHEMERAL, zookeeper.WorldACL(zookeeper.PERM_ALL), cf); err != nil {
		c.Fatal("Cannot set topology in ZooKeeper: ", err)
	}
}

func TestPackage(t *testing.T) {
	TestingT(t)
}

// StateSuite for State and the related types.
type StateSuite struct {
	zkServer   *zookeeper.Server
	zkConn     *zookeeper.Conn
	zkTestRoot string
	zkTestPort int
	zkAddr     string
}

var _ = Suite(&StateSuite{})

// SetUpSuite starts and inits ZooKeeper.
func (s *StateSuite) SetUpSuite(c *C) {
	var err error

	// Start server.
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

	// Establish connections after 30 seconds."
	time.Sleep(30e9)

	s.zkConn, _, err = zookeeper.Dial(s.zkAddr, 5e9)

	if err != nil {
		c.Fatal("Cannot establish ZooKeeper connection: ", err)
	}

	setTopology(s.zkConn, testTopology, c)
}

// TearDownSuite stops ZooKeeper.
func (s *StateSuite) TearDownSuite(c *C) {
	if s.zkServer != nil {
		s.zkServer.Destroy()
	}
}

// TestService tests the Service  method of the State.
func (s StateSuite) TestService(c *C) {
	c.Log("Test service ...")

	var err error
	var state *State
	var service *Service

	state, err = Open(s.zkConn)

	c.Assert(err, IsNil)
	c.Assert(state, Not(IsNil))

	service, err = state.Service("service-one")

	c.Assert(err, IsNil)
	c.Assert(service, Not(IsNil))
	c.Assert(service.CharmId, Equals, "my-charm-one")
}

// TestUnit tests the Unit method of the Service.
func (s StateSuite) TestUnit(c *C) {
	c.Log("Test unit ...")

	var err error
	var state *State
	var service *Service
	var unit *Unit

	state, err = Open(s.zkConn)

	c.Assert(err, IsNil)
	c.Assert(state, Not(IsNil))

	service, err = state.Service("service-zero")

	c.Assert(err, IsNil)
	c.Assert(service, Not(IsNil))

	unit, err = service.Unit("u-1")

	c.Assert(err, IsNil)
	c.Assert(unit, Not(IsNil))
	c.Assert(unit.Sequence, Equals, 1)

	unit, err = service.Unit("illegal-id")

	c.Assert(err, Equals, ErrUnitNotFound)
	c.Assert(unit, IsNil)
}

// TestSync tests the synchronization after a topology modification,
// here w/o ZooKeeper.
func (s StateSuite) TestSyncWithoutZookeeper(c *C) {
	c.Log("Test sync without ZooKeeper ...")

	oldTD := newTopologyData()
	err := goyaml.Unmarshal([]byte(testTopology), oldTD)

	c.Assert(err, IsNil)

	service := oldTD.Services["s-0"]

	c.Assert(service.CharmId, Equals, "my-charm-zero")

	newTD := newTopologyData()
	err = goyaml.Unmarshal([]byte(modifiedTestTopology), newTD)

	c.Assert(err, IsNil)
	c.Assert(newTD.Services["s-0"].CharmId, Equals, "my-charm-zero-modified")

	oldTD.sync(newTD)

	c.Assert(service.CharmId, Equals, "my-charm-zero-modified")
}
