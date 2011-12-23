// launchpad.net/juju/go/state
//
// Copyright (c) 2011 Canonical Ltd.

package state

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/gozk/zookeeper"
	"testing"
	"time"
)

// testTopology is the starting topology as YAML string.
const testTopology = `
services:
    s-0:
        name: service-zero
        units:
            u-0:
                sequence: 0
            u-1:
                sequence: 1
    s-1:
        name: service-one
        units:
    s-2:
        name: service-two
        units:
unit-sequence:
    service-zero: 2
    service-one: 0
    service-two: 0
`

// initZooKeeper writes the initial test data to ZK.
func initZooKeeper(zk *zookeeper.Conn, c *C) {
	create := func(p, v string) {
		if _, err := zk.Create(p, v, 0, zookeeper.WorldACL(zookeeper.PERM_ALL)); err != nil {
			c.Fatal("Cannot set path '"+p+"' in ZooKeeper: ", err)
		}
	}
	// Create nodes.
	create("/services", "")
	create("/services/s-0", "charm: my-charm-zero")
	create("/services/s-1", "charm: my-charm-one")
	create("/services/s-2", "charm: my-charm-two")
	create("/services/s-2/exposed", "")
	create("/topology", testTopology)
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

	initZooKeeper(s.zkConn, c)
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
	var charmId string

	state, err = Open(s.zkConn)

	c.Assert(err, IsNil)
	c.Assert(state, Not(IsNil))

	service, err = state.Service("service-zero")

	c.Assert(err, IsNil)
	c.Assert(service, Not(IsNil))

	charmId, err = service.CharmId()

	c.Assert(err, IsNil)
	c.Assert(charmId, Equals, "my-charm-zero")
}
