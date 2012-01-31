// launchpad.net/juju/go/state
//
// Copyright (c) 2011-2012 Canonical Ltd.

package state

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/goyaml"
	"launchpad.net/gozk/zookeeper"
	"testing"
)

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
	c.Assert(s.t.HasService("s-0"), Equals, false)
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

func (s TopologySuite) TestRemoveNonExistentService(c *C) {
	// Check that the removing of a non-existent service fails.
	err := s.t.RemoveService("n-0")
	c.Assert(err, ErrorMatches, `service with key "n-0" cannot be found`)
}

func (s TopologySuite) TestAddUnit(c *C) {
	// Check that the adding of a unit works correctly.
	err := s.t.AddService("s-0", "wordpress")
	c.Assert(err, IsNil)
	err = s.t.AddService("s-1", "mysql")
	c.Assert(err, IsNil)
	seq, err := s.t.AddUnit("s-0", "u-05")
	c.Assert(err, IsNil)
	c.Assert(seq, Equals, 0)
	seq, err = s.t.AddUnit("s-0", "u-12")
	c.Assert(err, IsNil)
	c.Assert(seq, Equals, 1)
	seq, err = s.t.AddUnit("s-1", "u-07")
	c.Assert(err, IsNil)
	c.Assert(seq, Equals, 0)
	keys, err := s.t.UnitKeys("s-0")
	c.Assert(err, IsNil)
	c.Assert(keys, Equals, []string{"u-05", "u-12"})
	keys, err = s.t.UnitKeys("s-1")
	c.Assert(err, IsNil)
	c.Assert(keys, Equals, []string{"u-07"})
}

func (s TopologySuite) TestGlobalUniqueUnitNames(c *C) {
	// Check that even if the underlying service is destroyed
	// and a new one with the same name is created we'll never
	// get a duplicate unit name.
	err := s.t.AddService("s-0", "wordpress")
	c.Assert(err, IsNil)
	seq, err := s.t.AddUnit("s-0", "u-0")
	c.Assert(err, IsNil)
	c.Assert(seq, Equals, 0)
	seq, err = s.t.AddUnit("s-0", "u-1")
	c.Assert(err, IsNil)
	c.Assert(seq, Equals, 1)
	err = s.t.RemoveService("s-0")
	c.Assert(err, IsNil)
	err = s.t.AddService("s-0", "wordpress")
	c.Assert(err, IsNil)
	seq, err = s.t.AddUnit("s-0", "u-1")
	c.Assert(err, IsNil)
	c.Assert(seq, Equals, 2)
	name, err := s.t.UnitName("s-0", "u-1")
	c.Assert(err, IsNil)
	c.Assert(name, Equals, "wordpress/2")
}

func (s TopologySuite) TestAddDuplicatedUnit(c *C) {
	// Check that it's not possible to add a unit twice.
	err := s.t.AddService("s-0", "wordpress")
	c.Assert(err, IsNil)
	_, err = s.t.AddUnit("s-0", "u-0")
	c.Assert(err, IsNil)
	_, err = s.t.AddUnit("s-0", "u-0")
	c.Assert(err, ErrorMatches, `unit "u-0" already in use in service "s-0"`)
}

func (s TopologySuite) TestAddUnitToNonExistingService(c *C) {
	// Check that the adding of a unit to a non-existing services
	// fails correctly.
	_, err := s.t.AddUnit("s-0", "u-0")
	c.Assert(err, ErrorMatches, `service with key "s-0" cannot be found`)
}

func (s TopologySuite) TestAddUnitToDifferentService(c *C) {
	// Check that the adding of the same unit to two different
	// services fails correctly.
	err := s.t.AddService("s-0", "wordpress")
	c.Assert(err, IsNil)
	err = s.t.AddService("s-1", "mysql")
	c.Assert(err, IsNil)
	_, err = s.t.AddUnit("s-0", "u-0")
	c.Assert(err, IsNil)
	_, err = s.t.AddUnit("s-1", "u-0")
	c.Assert(err, ErrorMatches, `unit "u-0" already in use in service "s-0"`)
}

func (s TopologySuite) TestUnitKeys(c *C) {
	// Check if registered units from a service are returned correctly.
	err := s.t.AddService("s-0", "wordpress")
	c.Assert(err, IsNil)
	err = s.t.AddService("s-1", "mysql")
	c.Assert(err, IsNil)
	units, err := s.t.UnitKeys("s-0")
	c.Assert(err, IsNil)
	c.Assert(units, Equals, []string{})
	_, err = s.t.AddUnit("s-0", "u-0")
	c.Assert(err, IsNil)
	_, err = s.t.AddUnit("s-0", "u-1")
	c.Assert(err, IsNil)
	_, err = s.t.AddUnit("s-1", "u-2")
	c.Assert(err, IsNil)
	units, err = s.t.UnitKeys("s-0")
	c.Assert(err, IsNil)
	c.Assert(units, Equals, []string{"u-0", "u-1"})
	units, err = s.t.UnitKeys("s-1")
	c.Assert(err, IsNil)
	c.Assert(units, Equals, []string{"u-2"})
}

func (s TopologySuite) TestUnitKeysWithNonExistingService(c *C) {
	// Check if the retrieving of unit keys from a non-existing
	// service fails correctly.
	_, err := s.t.UnitKeys("s-0")
	c.Assert(err, ErrorMatches, `service with key "s-0" cannot be found`)
}

func (s TopologySuite) TestHasUnit(c *C) {
	// Check that the test for a unit in a service works correctly.
	err := s.t.AddService("s-0", "wordpress")
	c.Assert(err, IsNil)
	found := s.t.HasUnit("s-0", "u-0")
	c.Assert(err, IsNil)
	c.Assert(found, Equals, false)
	_, err = s.t.AddUnit("s-0", "u-0")
	c.Assert(err, IsNil)
	found = s.t.HasUnit("s-0", "u-0")
	c.Assert(found, Equals, true)
	found = s.t.HasUnit("s-0", "u-1")
	c.Assert(found, Equals, false)
}

func (s TopologySuite) TestUnitName(c *C) {
	// Check that the human readable names are returned correctly.
	err := s.t.AddService("s-0", "wordpress")
	c.Assert(err, IsNil)
	err = s.t.AddService("s-1", "mysql")
	c.Assert(err, IsNil)
	_, err = s.t.AddUnit("s-0", "u-0")
	c.Assert(err, IsNil)
	_, err = s.t.AddUnit("s-0", "u-1")
	c.Assert(err, IsNil)
	_, err = s.t.AddUnit("s-1", "u-2")
	c.Assert(err, IsNil)
	name, err := s.t.UnitName("s-0", "u-0")
	c.Assert(err, IsNil)
	c.Assert(name, Equals, "wordpress/0")
	name, err = s.t.UnitName("s-0", "u-1")
	c.Assert(err, IsNil)
	c.Assert(name, Equals, "wordpress/1")
	name, err = s.t.UnitName("s-1", "u-2")
	c.Assert(err, IsNil)
	c.Assert(name, Equals, "mysql/0")
}

func (s TopologySuite) TestUnitNameWithNonExistingServiceOrUnit(c *C) {
	// Check if the retrieval of unit names fails if the service
	// or the unit doesn't exist.
	_, err := s.t.UnitName("s-0", "u-1")
	c.Assert(err, ErrorMatches, `service with key "s-0" cannot be found`)
	err = s.t.AddService("s-0", "wordpress")
	c.Assert(err, IsNil)
	_, err = s.t.UnitName("s-0", "u-1")
	c.Assert(err, ErrorMatches, `unit with key "u-1" cannot be found`)
	_, err = s.t.AddUnit("s-0", "u-0")
	c.Assert(err, IsNil)
	_, err = s.t.UnitName("s-0", "u-1")
	c.Assert(err, ErrorMatches, `unit with key "u-1" cannot be found`)
}

func (s TopologySuite) TestRemoveUnit(c *C) {
	// Check that the removing of a unit works correctly.
	err := s.t.AddService("s-0", "wordpress")
	c.Assert(err, IsNil)
	_, err = s.t.AddUnit("s-0", "u-0")
	c.Assert(err, IsNil)
	_, err = s.t.AddUnit("s-0", "u-1")
	c.Assert(err, IsNil)
	err = s.t.RemoveUnit("s-0", "u-0")
	c.Assert(err, IsNil)
	found := s.t.HasUnit("s-0", "u-0")
	c.Assert(found, Equals, false)
	found = s.t.HasUnit("s-0", "u-1")
	c.Assert(found, Equals, true)
}

func (s TopologySuite) TestRemoveNonExistingUnit(c *C) {
	// Check that the removing of non-existing units fails.
	err := s.t.RemoveUnit("s-0", "u-0")
	c.Assert(err, ErrorMatches, `service with key "s-0" cannot be found`)
	err = s.t.AddService("s-0", "wordpress")
	c.Assert(err, IsNil)
	err = s.t.RemoveUnit("s-0", "u-0")
	c.Assert(err, ErrorMatches, `unit with key "u-0" cannot be found`)
}

func (s TopologySuite) TestUnitKeyFromSequence(c *C) {
	// Check that the retrieving of a unit key by service key
	// and sequence number works correctly.
	err := s.t.AddService("s-0", "wordpress")
	c.Assert(err, IsNil)
	_, err = s.t.AddUnit("s-0", "u-0")
	c.Assert(err, IsNil)
	_, err = s.t.AddUnit("s-0", "u-1")
	c.Assert(err, IsNil)
	key, err := s.t.UnitKeyFromSequence("s-0", 0)
	c.Assert(err, IsNil)
	c.Assert(key, Equals, "u-0")
	key, err = s.t.UnitKeyFromSequence("s-0", 1)
	c.Assert(err, IsNil)
	c.Assert(key, Equals, "u-1")
	key, err = s.t.UnitKeyFromSequence("s-0", 2)
	c.Assert(err, ErrorMatches, `unit with sequence number 2 cannot be found`)
}

func (s TopologySuite) TestUnitKeyFromNonExistingService(c *C) {
	_, err := s.t.UnitKeyFromSequence("s-0", 0)
	c.Assert(err, ErrorMatches, `service with key "s-0" cannot be found`)
}

type ConfigNodeSuite struct {
	zkServer    *zookeeper.Server
	zkTestRoot  string
	zkTestPort  int
	zkAddr      string
	zkConn      *zookeeper.Conn
	zkEventChan <-chan zookeeper.Event
	path        string
}

var _ = Suite(&ConfigNodeSuite{})

func (s *ConfigNodeSuite) SetUpSuite(c *C) {
	var err error
	s.zkTestRoot = c.MkDir() + "/zookeeper"
	s.zkTestPort = 21812
	s.zkAddr = fmt.Sprint("localhost:", s.zkTestPort)
	s.path = "/config"

	s.zkServer, err = zookeeper.CreateServer(s.zkTestPort, s.zkTestRoot, "")
	if err != nil {
		c.Fatal("Cannot set up ZooKeeper server environment: ", err)
	}
	err = s.zkServer.Start()
	if err != nil {
		c.Fatal("Cannot start ZooKeeper server: ", err)
	}
}

func (s *ConfigNodeSuite) TearDownSuite(c *C) {
	if s.zkServer != nil {
		s.zkServer.Destroy()
	}
}

func (s *ConfigNodeSuite) SetUpTest(c *C) {
	var err error
	// Connect the server.
	s.zkConn, s.zkEventChan, err = zookeeper.Dial(s.zkAddr, 5e9)
	c.Assert(err, IsNil)
	// Wait for connect signal.
	event := <-s.zkEventChan
	c.Assert(event.Type, Equals, zookeeper.EVENT_SESSION)
	c.Assert(event.State, Equals, zookeeper.STATE_CONNECTED)
}

func (s *ConfigNodeSuite) TearDownTest(c *C) {
	// Delete possible nodes, ignore errors.
	zkRemoveTree(s.zkConn, s.path)
	s.zkConn.Close()
}

func (s ConfigNodeSuite) TestCreateEmptyConfigNode(c *C) {
	// Check that creating an empty node works correctly.
	node, err := createConfigNode(s.zkConn, s.path, nil)
	c.Assert(err, IsNil)
	c.Assert(node.Keys(), Equals, []string{})
}

func (s ConfigNodeSuite) TestReadWithoutWrite(c *C) {
	// Check reading without writing.
	node, err := readConfigNode(s.zkConn, s.path, false)
	c.Assert(err, IsNil)
	c.Assert(node, Not(IsNil))
}

func (s ConfigNodeSuite) TestSetWithoutWrite(c *C) {
	// Check that config values can be set.
	_, err := s.zkConn.Create(s.path, "", 0, zkPermAll)
	c.Assert(err, IsNil)
	node, err := readConfigNode(s.zkConn, s.path, false)
	c.Assert(err, IsNil)
    	options := map[string]interface{}{"alpha": "beta", "one": 1}
    	node.Update(options)
    	c.Assert(node.Data(), Equals, options)
	// Node data has to be empty.
	yaml, _, err := s.zkConn.Get("/config")
	c.Assert(err, IsNil)
	c.Assert(yaml, Equals, "")
}

func (s ConfigNodeSuite) TestSetWithWrite(c *C) {
	// Check that write updates the local and the ZooKeeper state.
	node, err := readConfigNode(s.zkConn, s.path, false)
	c.Assert(err, IsNil)
	options := map[string]interface{}{"alpha": "beta", "one": 1}
    	node.Update(options)
    	changes, err := node.Write()
    	c.Assert(err, IsNil)
    	c.Assert(len(changes), Equals, 2)
    	// Check local state.
    	c.Assert(node.Data(), Equals, options)
    	// Check ZooKeeper state.
	yaml, _, err := s.zkConn.Get(s.path)
	c.Assert(err, IsNil)
	zkData := make(map[string]interface{})
	err = goyaml.Unmarshal([]byte(yaml), zkData)
	c.Assert(zkData, Equals, options)
}

func (s ConfigNodeSuite) TestConflictOnSet(c *C) {
	// Check version conflict errors.
	nodeOne, err := readConfigNode(s.zkConn, s.path, false)
	c.Assert(err, IsNil)
	nodeTwo, err := readConfigNode(s.zkConn, s.path, false)
	c.Assert(err, IsNil)

	optionsOld := map[string]interface{}{"alpha": "beta", "one": 1}
	nodeOne.Update(optionsOld)
	nodeOne.Write()

	nodeTwo.Update(optionsOld)
	changes, err := nodeTwo.Write()
    	c.Assert(err, IsNil)
    	c.Assert(len(changes), Equals, 2)

    	// First test node one.
    	c.Assert(nodeOne.Data(), Equals, optionsOld)

    	// Write on node one.
    	optionsNew := map[string]interface{}{"alpha": "gamma", "one": "two"}
	nodeOne.Update(optionsNew)
	changes, err = nodeOne.Write()
    	c.Assert(err, IsNil)
    	c.Assert(len(changes), Equals, 2)

    	// Verify that node one reports as expected.
    	c.Assert(nodeOne.Data(), Equals, optionsNew)

    	// Verify that node two has still the old data.
    	c.Assert(nodeTwo.Data(), Equals, optionsOld)

    	// Now issue a Set/Write from node two. This will
    	// merge the data deleting 'one' and updating
    	// other values.
	optionsMerge := map[string]interface{}{"alpha": "cappa", "new": "next"}
	nodeTwo.Update(optionsMerge)
	nodeTwo.Delete("one")

	expected := map[string]interface{}{"alpha": "cappa", "new": "next"}
	changes, err = nodeTwo.Write()
    	c.Assert(err, IsNil)
    	c.Assert(len(changes), Equals, 3)
    	c.Assert(expected, Equals, nodeTwo.Data())

    	// But node one still reflects the former data.
    	c.Assert(nodeOne.Data(), Equals, optionsNew)
}

func (s ConfigNodeSuite) TestSetItem(c *C) {
	// Check that Set works as expected.
	node, err := readConfigNode(s.zkConn, s.path, false)
	c.Assert(err, IsNil)
	options := map[string]interface{}{"alpha": "beta", "one": 1}
	node.Set("alpha", "beta")
	node.Set("one", 1)
	changes, err := node.Write()
    	c.Assert(err, IsNil)
    	c.Assert(len(changes), Equals, 2)
	// Check local state.
    	c.Assert(node.Data(), Equals, options)
    	// Check ZooKeeper state.
	yaml, _, err := s.zkConn.Get(s.path)
	c.Assert(err, IsNil)
	zkData := make(map[string]interface{})
	err = goyaml.Unmarshal([]byte(yaml), zkData)
	c.Assert(zkData, Equals, options)
}

func (s ConfigNodeSuite) TestMultipleReads(c *C) {
	// Check that reads without writes always resets the data.
	nodeOne, err := readConfigNode(s.zkConn, s.path, false)
	c.Assert(err, IsNil)
	nodeOne.Update(map[string]interface{}{"alpha": "beta", "foo": "bar"})
	value, ok := nodeOne.Get("alpha")
	c.Assert(ok, Equals, true)
	c.Assert(value, Equals, "beta")
	value, ok = nodeOne.Get("foo")
	c.Assert(ok, Equals, true)
	c.Assert(value, Equals, "bar")
	value, ok = nodeOne.Get("baz")
	c.Assert(ok, Equals, false)

        // A read resets the data to the empty state.
	nodeOne, err = readConfigNode(s.zkConn, s.path, false)
	c.Assert(err, IsNil)
	c.Assert(nodeOne.Data(), Equals, map[string]interface{}{})
	nodeOne.Update(map[string]interface{}{"alpha": "beta", "foo": "bar"})
	changes, err := nodeOne.Write()
    	c.Assert(err, IsNil)
    	c.Assert(len(changes), Equals, 2)

    	// A write retains the newly set values.
    	value, ok = nodeOne.Get("alpha")
	c.Assert(ok, Equals, true)
	c.Assert(value, Equals, "beta")
	value, ok = nodeOne.Get("foo")
	c.Assert(ok, Equals, true)
	c.Assert(value, Equals, "bar")

        // Now get another state instance and change ZooKeeper state.
	nodeTwo, err := readConfigNode(s.zkConn, s.path, false)
	c.Assert(err, IsNil)
	nodeTwo.Update(map[string]interface{}{"foo": "different"})
	changes, err = nodeTwo.Write()
    	c.Assert(err, IsNil)
    	c.Assert(len(changes), Equals, 1)

    	// This should pull in the new state into node one.
    	err = nodeOne.Read(false)
   	c.Assert(err, IsNil)
    	value, ok = nodeOne.Get("alpha")
	c.Assert(ok, Equals, true)
	c.Assert(value, Equals, "beta")
	value, ok = nodeOne.Get("foo")
	c.Assert(ok, Equals, true)
	c.Assert(value, Equals, "different")
}

func (s ConfigNodeSuite) TestDeleteEmptiesState(c *C) {
	// Check that delete creates an empty state.
	node, err := readConfigNode(s.zkConn, s.path, false)
	c.Assert(err, IsNil)
	node.Set("a", "foo")
	changes, err := node.Write()
    	c.Assert(err, IsNil)
    	c.Assert(len(changes), Equals, 1)
    	node.Delete("a")
    	changes, err = node.Write()
    	c.Assert(err, IsNil)
    	c.Assert(len(changes), Equals, 1)
	c.Assert(node.Data(), Equals, map[string]interface{}{})
}

func (s ConfigNodeSuite) TestReadResync(c *C) {
	// Check that read pulls the data into the node.
	nodeOne, err := readConfigNode(s.zkConn, s.path, false)
	c.Assert(err, IsNil)
	nodeOne.Set("a", "foo")
	changes, err := nodeOne.Write()
    	c.Assert(err, IsNil)
    	c.Assert(len(changes), Equals, 1)

	nodeTwo, err := readConfigNode(s.zkConn, s.path, false)
	c.Assert(err, IsNil)
	nodeTwo.Delete("a")
	changes, err = nodeTwo.Write()
    	c.Assert(err, IsNil)
    	c.Assert(len(changes), Equals, 1)
    	nodeTwo.Set("a", "bar")
    	changes, err = nodeTwo.Write()
    	c.Assert(err, IsNil)
    	c.Assert(len(changes), Equals, 1)
    	// Multiple reads, the one of node one should pick up the 
    	// new value on the read.
    	_, _, err = s.zkConn.Get(s.path)
	c.Assert(err, IsNil)
	err = nodeOne.Read(false)
	c.Assert(err, IsNil)
    	_, _, err = s.zkConn.Get(s.path)
	c.Assert(err, IsNil)
	value, ok := nodeOne.Get("a")
	c.Assert(ok, Equals, true)
	c.Assert(value, Equals, "bar")
}

func (s ConfigNodeSuite) TestMultipleWrites(c *C) {
	// Check that multiple writes only does the right changes.
	node, err := readConfigNode(s.zkConn, s.path, false)
	c.Assert(err, IsNil)
	node.Update(map[string]interface{}{"foo": "bar", "this": "that"})
	changes, err := node.Write()
    	c.Assert(err, IsNil)
    	c.Assert(len(changes), Equals, 2)

    	node.Delete("this")
    	node.Set("another", "value")
	changes, err = node.Write()
    	c.Assert(err, IsNil)
    	c.Assert(len(changes), Equals, 2)

	expected := map[string]interface{}{"foo": "bar", "another": "value"}
    	c.Assert(expected, Equals, node.Data())
    		
    	changes, err = node.Write()
    	c.Assert(err, IsNil)
    	c.Assert(len(changes), Equals, 0)

    	err = node.Read(false)
    	c.Assert(err, IsNil)
    	c.Assert(expected, Equals, node.Data())

	changes, err = node.Write()
    	c.Assert(err, IsNil)
    	c.Assert(len(changes), Equals, 0)
}

func (s ConfigNodeSuite) TestWriteTwice(c *C) {
	// Check the correct writing into a node by two config nodes.
	nodeOne, err := readConfigNode(s.zkConn, s.path, false)
	c.Assert(err, IsNil)
    	nodeOne.Set("a", "foo")
	changes, err := nodeOne.Write()
    	c.Assert(err, IsNil)
    	c.Assert(len(changes), Equals, 1)

    	nodeTwo, err := readConfigNode(s.zkConn, s.path, false)
	c.Assert(err, IsNil)
    	nodeTwo.Set("a", "bar")
	changes, err = nodeTwo.Write()
    	c.Assert(err, IsNil)
    	c.Assert(len(changes), Equals, 1)

        // Shouldn't write again. Changes were already
        // flushed and acted upon by other parties.
        changes, err = nodeOne.Write()
    	c.Assert(err, IsNil)
    	c.Assert(len(changes), Equals, 0)

	err = nodeOne.Read(false)
	c.Assert(err, IsNil)
	c.Assert(nodeOne, Equals, nodeTwo)
}

func (s ConfigNodeSuite) TestReadRequiresNode(c *C) {
	// Check that reading a non-existing config node fails
	// if 'required' is set to true.
	_, err := readConfigNode(s.zkConn, s.path, true)
	c.Assert(err, ErrorMatches, `config "/config" not found`)
}