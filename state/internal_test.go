// launchpad.net/juju/go/state
//
// Copyright (c) 2011-2012 Canonical Ltd.

package state

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/gozk/zookeeper"
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

type TopologySuite struct {
	zkServer    *zookeeper.Server
	zkTestRoot  string
	zkTestPort  int
	zkAddr      string
	zkConn      *zookeeper.Conn
	zkEventChan <-chan zookeeper.Event
	t           *topology
}

var _ = Suite(&TopologySuite{})

func (s *TopologySuite) SetUpSuite(c *C) {
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

func (s *TopologySuite) TearDownSuite(c *C) {
	if s.zkServer != nil {
		s.zkServer.Destroy()
	}
}

func (s *TopologySuite) SetUpTest(c *C) {
	var err error
	// Connect the server.
	s.zkConn, s.zkEventChan, err = zookeeper.Dial(s.zkAddr, 5e9)
	c.Assert(err, IsNil)
	// Wait for connect signal.
	event := <-s.zkEventChan
	c.Assert(event.Type, Equals, zookeeper.EVENT_SESSION)
	c.Assert(event.State, Equals, zookeeper.STATE_CONNECTED)
	// Read the toplogy.
	s.t, err = readTopology(s.zkConn)
	c.Assert(err, IsNil)
}

func (s *TopologySuite) TearDownTest(c *C) {
	// Delete possible nodes, ignore errors.
	zkRemoveTree(s.zkConn, "/topology")
	s.zkConn.Close()
}

func (s TopologySuite) TestAddService(c *C) {
	// Check that adding services works correctly.
	err := s.t.AddService("s-0", "wordpress")
	c.Assert(err, IsNil)
	err = s.t.AddService("s-1", "mysql")
	c.Assert(err, IsNil)
	c.Assert(s.t.HasService("s-0"), Equals, true)
	c.Assert(s.t.HasService("s-1"), Equals, true)
}

func (s TopologySuite) TestAddDuplicateService(c *C) {
	// Check that adding a duplicate service by key or name fails.
	err := s.t.AddService("s-0", "wordpress")
	c.Assert(err, IsNil)
	err = s.t.AddService("s-0", "mysql")
	c.Assert(err, ErrorMatches, `attempted to add duplicated service "s-0"`)
	err = s.t.AddService("s-1", "wordpress")
	c.Assert(err, ErrorMatches, `service name "wordpress" already in use`)
}

func (s TopologySuite) TestHasService(c *C) {
	// Check that the test for a service by key works correctly.
	c.Assert(s.t.HasService("s-0"), Equals, false)
	err := s.t.AddService("s-0", "wordpress")
	c.Assert(err, IsNil)
	c.Assert(s.t.HasService("s-0"), Equals, true)
}

func (s TopologySuite) TestServiceKey(c *C) {
	// Check that the key retrieval for a service name works correctly.
	key, err := s.t.ServiceKey("wordpress")
	c.Assert(err, ErrorMatches, `service with name "wordpress" cannot be found`)
	err = s.t.AddService("s-0", "wordpress")
	c.Assert(err, IsNil)
	key, err = s.t.ServiceKey("wordpress")
	c.Assert(err, IsNil)
	c.Assert(key, Equals, "s-0")
}

func (s TopologySuite) TestServiceName(c *C) {
	// Check that the name retrieval for a service name works correctly.
	name, err := s.t.ServiceName("s-0")
	c.Assert(err, ErrorMatches, `service with key "s-0" cannot be found`)
	err = s.t.AddService("s-0", "wordpress")
	c.Assert(err, IsNil)
	name, err = s.t.ServiceName("s-0")
	c.Assert(err, IsNil)
	c.Assert(name, Equals, "wordpress")
}

func (s TopologySuite) TestRemoveService(c *C) {
	// Check that the removing of a service works correctly.
	err := s.t.AddService("s-0", "wordpress")
	c.Assert(err, IsNil)
	err = s.t.AddService("s-1", "mysql")
	c.Assert(err, IsNil)
	err = s.t.RemoveService("s-0")
	c.Assert(err, IsNil)
	c.Assert(s.t.HasService("s-0"), Equals, false)
	c.Assert(s.t.HasService("s-1"), Equals, true)
}