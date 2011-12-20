// launchpad.net/juju/go/state
//
// Copyright (c) 2011 Canonical Ltd.

package state

// --------------------
// IMPORT
// --------------------

import (
	"errors"
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/goyaml"
	"launchpad.net/gozk/zookeeper"
	"testing"
	"time"
)

// --------------------
// TEST DATA
// --------------------

// testTopology is a topology as YAML string.
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

// unmarshalTestTopology returns the testTopology as
// topologyNodes.
func unmarshalTestTopology(c *C) topologyNodes {
	tn := newTopologyNodes()

	if err := goyaml.Unmarshal([]byte(testTopology), &tn); err != nil {
		c.Fail()

		return nil
	}

	return tn
}

// setTestTopology sets the topology nodes in ZooKeeper.
func setTestTopology(zkc *zookeeper.Conn, c *C) {
	_, err := zkc.Create("/topology", testTopology, zookeeper.EPHEMERAL, zookeeper.WorldACL(zookeeper.PERM_ALL))

	if err != nil {
		c.Fatal("Cannot set test topology in ZooKeeper: ", err)
	}
}

// --------------------
// TESTING
// --------------------

func TestPackage(t *testing.T) {
	TestingT(t)
}

// --------------------
// TOPOLOGY NODE
// --------------------

// TopologyNodeSuite for topologyNode.
type TopologyNodeSuite struct{}

var _ = Suite(&TopologyNodeSuite{})

// TestFind tests the find method of the topologyNodes.
func (tn TopologyNodeSuite) TestFind(c *C) {
	n := unmarshalTestTopology(c)

	var data interface{}
	var err error

	data, err = n.find([]string{"services", "s-0", "name"})

	c.Assert(data, Equals, "service-zero")
	c.Assert(err, IsNil)

	data, err = n.find([]string{"services", "s-0"})

	c.Assert(data, FitsTypeOf, newTopologyNodes())
	c.Assert(err, IsNil)

	data, err = n.find([]string{"unknown", "path", "with", "no", "result"})

	c.Assert(data, IsNil)
	c.Assert(err, FitsTypeOf, errors.New(""))
}

// TestGetString tests the getString method of the topologyNodes.
func (tn TopologyNodeSuite) TestGetString(c *C) {
	n := unmarshalTestTopology(c)

	var data string
	var err error

	data, err = n.getString("/services/s-0/name/")

	c.Assert(data, Equals, "service-zero")
	c.Assert(err, IsNil)

	data, err = n.getString("/services/s-0")

	c.Assert(data, Equals, "")
	c.Assert(err, FitsTypeOf, errors.New(""))

	data, err = n.getString("/unknown/path/with/no/result")

	c.Assert(data, Equals, "")
	c.Assert(err, FitsTypeOf, errors.New(""))
}

// TestGetNodes tests the getNodes method of the topologyNodes.
func (tn TopologyNodeSuite) TestGetNodes(c *C) {
	n := unmarshalTestTopology(c)

	var data topologyNodes
	var err error

	data, err = n.getNodes("/services/s-0/name/")

	c.Assert(data, IsNil)
	c.Assert(err, FitsTypeOf, errors.New(""))

	data, err = n.getNodes("/services/s-0")

	c.Assert(data, FitsTypeOf, newTopologyNodes())
	c.Assert(err, IsNil)

	data, err = n.getNodes("/unknown/path/with/no/result")

	c.Assert(data, IsNil)
	c.Assert(err, FitsTypeOf, errors.New(""))
}

// TestSearch tests the search method of the topologyNodes.
func (tn TopologyNodeSuite) TestSearch(c *C) {
	n := unmarshalTestTopology(c)

	path, value, err := n.search(func(p []string, v interface{}) bool {
		if len(p) == 3 && p[0] == "services" && p[len(p)-1] == "name" && v == "service-one" {
			return true
		}

		return false
	})

	c.Assert(err, IsNil)
	c.Assert(path[1], Equals, "s-1")
	c.Assert(value, Not(IsNil))
}

// --------------------
// STATE
// --------------------

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

	// Establish connection after around a minute."
	time.Sleep(60e9)

	s.zkConn, _, err = zookeeper.Dial(s.zkAddr, 5e9)

	if err != nil {
		c.Fatal("Cannot establish ZooKeeper connection: ", err)
	}

	setTestTopology(s.zkConn, c)
}

// TearDownSuite stops ZooKeeper.
func (s *StateSuite) TearDownSuite(c *C) {
	if s.zkServer != nil {
		if s.zkConn != nil {
			// s.zkConn.Close()
		}

		s.zkServer.Destroy()
	}
}

// TestService tests the Service  method of the State.
func (s StateSuite) TestService(c *C) {
	var err error
	var state *State
	var service *Service

	state, err = Open(s.zkConn)

	c.Assert(err, IsNil)
	c.Assert(state, Not(IsNil))

	service, err = state.Service("service-one")

	c.Assert(err, IsNil)
	c.Assert(service, Not(IsNil))
	c.Assert(service.CharmId(), Equals, "my-charm-one")
}

// EOF
