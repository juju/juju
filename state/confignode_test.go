package state

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/testing"
)

type ConfigNodeSuite struct {
	testing.MgoSuite
	state *State
	path  string
}

var _ = Suite(&ConfigNodeSuite{})

func (s *ConfigNodeSuite) SetUpTest(c *C) {
	s.MgoSuite.SetUpTest(c)
	state, err := Open(&Info{Addrs: []string{testing.MgoAddr}})
	c.Assert(err, IsNil)

	s.state = state
	s.path = "/config"
}

func (s *ConfigNodeSuite) TearDownTest(c *C) {
	s.state.Close()
	s.MgoSuite.TearDownTest(c)
}

func (s *ConfigNodeSuite) TestCreateEmptyConfigNode(c *C) {
	// Check that creating an empty node works correctly.
	node, err := createConfigNode(s.state, s.path, nil)
	c.Assert(err, IsNil)
	c.Assert(node.Keys(), DeepEquals, []string{})
}

func (s *ConfigNodeSuite) TestReadWithoutWrite(c *C) {
	// Check reading without writing.
	node, err := readConfigNode(s.state, s.path)
	c.Assert(err, IsNil)
	c.Assert(node, Not(IsNil))
}

func (s *ConfigNodeSuite) TestUpdateWithoutWrite(c *C) {
	// Check that config values can be updated.
	node, err := readConfigNode(s.state, s.path)
	c.Assert(err, IsNil)
	options := map[string]interface{}{"alpha": "beta", "one": 1}
	node.Update(options)
	c.Assert(node.Map(), DeepEquals, options)
	// Node data has to be empty.
	mgoData := []interface{}{}
	err = s.MgoSuite.Session.DB("juju").C("settings").FindId(s.path).All(&mgoData)
	c.Assert(err, IsNil)
	c.Assert(mgoData, HasLen, 0)
}

func (s *ConfigNodeSuite) TestUpdateWithWrite(c *C) {
	// Check that write updates the local and the server state.
	node, err := readConfigNode(s.state, s.path)
	c.Assert(err, IsNil)
	options := map[string]interface{}{"alpha": "beta", "one": 1}
	node.Update(options)
	changes, err := node.Write()
	c.Assert(err, IsNil)
	c.Assert(changes, DeepEquals, []ItemChange{
		{ItemAdded, "alpha", nil, "beta"},
		{ItemAdded, "one", nil, 1},
	})
	// Check local state.
	c.Assert(node.Map(), DeepEquals, options)

	// Check MongoDB state.
	mgoData := make(map[string]interface{}, 0)
	err = s.MgoSuite.Session.DB("juju").C("settings").FindId(s.path).One(&mgoData)
	c.Assert(err, IsNil)
	cleanMap(mgoData)
	c.Assert(mgoData, DeepEquals, options)
}

func (s *ConfigNodeSuite) TestConflictOnSet(c *C) {
	// Check version conflict errors.
	nodeOne, err := readConfigNode(s.state, s.path)
	c.Assert(err, IsNil)
	nodeTwo, err := readConfigNode(s.state, s.path)
	c.Assert(err, IsNil)

	optionsOld := map[string]interface{}{"alpha": "beta", "one": 1}
	nodeOne.Update(optionsOld)
	nodeOne.Write()

	nodeTwo.Update(optionsOld)
	changes, err := nodeTwo.Write()
	c.Assert(err, IsNil)
	c.Assert(changes, DeepEquals, []ItemChange{
		{ItemAdded, "alpha", nil, "beta"},
		{ItemAdded, "one", nil, 1},
	})

	// First test node one.
	c.Assert(nodeOne.Map(), DeepEquals, optionsOld)

	// Write on node one.
	optionsNew := map[string]interface{}{"alpha": "gamma", "one": "two"}
	nodeOne.Update(optionsNew)
	changes, err = nodeOne.Write()
	c.Assert(err, IsNil)
	c.Assert(changes, DeepEquals, []ItemChange{
		{ItemModified, "alpha", "beta", "gamma"},
		{ItemModified, "one", 1, "two"},
	})

	// Verify that node one reports as expected.
	c.Assert(nodeOne.Map(), DeepEquals, optionsNew)

	// Verify that node two has still the old data.
	c.Assert(nodeTwo.Map(), DeepEquals, optionsOld)

	// Now issue a Set/Write from node two. This will
	// merge the data deleting 'one' and updating
	// other values.
	optionsMerge := map[string]interface{}{"alpha": "cappa", "new": "next"}
	nodeTwo.Update(optionsMerge)
	nodeTwo.Delete("one")

	expected := map[string]interface{}{"alpha": "cappa", "new": "next"}
	changes, err = nodeTwo.Write()
	c.Assert(err, IsNil)
	c.Assert(changes, DeepEquals, []ItemChange{
		{ItemModified, "alpha", "beta", "cappa"},
		{ItemAdded, "new", nil, "next"},
		{ItemDeleted, "one", 1, nil},
	})
	c.Assert(expected, DeepEquals, nodeTwo.Map())

	// But node one still reflects the former data.
	c.Assert(nodeOne.Map(), DeepEquals, optionsNew)
}

func (s *ConfigNodeSuite) TestSetItem(c *C) {
	// Check that Set works as expected.
	node, err := readConfigNode(s.state, s.path)
	c.Assert(err, IsNil)
	options := map[string]interface{}{"alpha": "beta", "one": 1}
	node.Set("alpha", "beta")
	node.Set("one", 1)
	changes, err := node.Write()
	c.Assert(err, IsNil)
	c.Assert(changes, DeepEquals, []ItemChange{
		{ItemAdded, "alpha", nil, "beta"},
		{ItemAdded, "one", nil, 1},
	})
	// Check local state.
	c.Assert(node.Map(), DeepEquals, options)
	// Check MongoDB state.
	mgoData := make(map[string]interface{}, 0)
	err = s.MgoSuite.Session.DB("juju").C("settings").FindId(s.path).One(&mgoData)
	c.Assert(err, IsNil)
	cleanMap(mgoData)
	c.Assert(mgoData, DeepEquals, options)
}

func (s *ConfigNodeSuite) TestMultipleReads(c *C) {
	// Check that reads without writes always resets the data.
	nodeOne, err := readConfigNode(s.state, s.path)
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
	err = nodeOne.Read()
	c.Assert(err, IsNil)
	c.Assert(nodeOne.Map(), DeepEquals, map[string]interface{}{})
	nodeOne.Update(map[string]interface{}{"alpha": "beta", "foo": "bar"})
	changes, err := nodeOne.Write()
	c.Assert(err, IsNil)
	c.Assert(changes, DeepEquals, []ItemChange{
		{ItemAdded, "alpha", nil, "beta"},
		{ItemAdded, "foo", nil, "bar"},
	})

	// A write retains the newly set values.
	value, ok = nodeOne.Get("alpha")
	c.Assert(ok, Equals, true)
	c.Assert(value, Equals, "beta")
	value, ok = nodeOne.Get("foo")
	c.Assert(ok, Equals, true)
	c.Assert(value, Equals, "bar")

	// Now get another state instance and change underlying state.
	nodeTwo, err := readConfigNode(s.state, s.path)
	c.Assert(err, IsNil)
	nodeTwo.Update(map[string]interface{}{"foo": "different"})
	changes, err = nodeTwo.Write()
	c.Assert(err, IsNil)
	c.Assert(changes, DeepEquals, []ItemChange{
		{ItemModified, "foo", "bar", "different"},
	})

	// This should pull in the new state into node one.
	err = nodeOne.Read()
	c.Assert(err, IsNil)
	value, ok = nodeOne.Get("alpha")
	c.Assert(ok, Equals, true)
	c.Assert(value, Equals, "beta")
	value, ok = nodeOne.Get("foo")
	c.Assert(ok, Equals, true)
	c.Assert(value, Equals, "different")
}

func (s *ConfigNodeSuite) TestDeleteEmptiesState(c *C) {
	// Check that delete creates an empty state.
	node, err := readConfigNode(s.state, s.path)
	c.Assert(err, IsNil)
	node.Set("a", "foo")
	changes, err := node.Write()
	c.Assert(err, IsNil)
	c.Assert(changes, DeepEquals, []ItemChange{
		{ItemAdded, "a", nil, "foo"},
	})
	node.Delete("a")
	changes, err = node.Write()
	c.Assert(err, IsNil)
	c.Assert(changes, DeepEquals, []ItemChange{
		{ItemDeleted, "a", "foo", nil},
	})
	c.Assert(node.Map(), DeepEquals, map[string]interface{}{})
}

func (s *ConfigNodeSuite) TestReadResync(c *C) {
	// Check that read pulls the data into the node.
	nodeOne, err := readConfigNode(s.state, s.path)
	c.Assert(err, IsNil)
	nodeOne.Set("a", "foo")
	changes, err := nodeOne.Write()
	c.Assert(err, IsNil)
	c.Assert(changes, DeepEquals, []ItemChange{
		{ItemAdded, "a", nil, "foo"},
	})
	nodeTwo, err := readConfigNode(s.state, s.path)
	c.Assert(err, IsNil)
	nodeTwo.Delete("a")
	changes, err = nodeTwo.Write()
	c.Assert(err, IsNil)
	c.Assert(changes, DeepEquals, []ItemChange{
		{ItemDeleted, "a", "foo", nil},
	})
	nodeTwo.Set("a", "bar")
	changes, err = nodeTwo.Write()
	c.Assert(err, IsNil)
	c.Assert(changes, DeepEquals, []ItemChange{
		{ItemAdded, "a", nil, "bar"},
	})
	// Read of node one should pick up the new value.
	err = nodeOne.Read()
	c.Assert(err, IsNil)
	value, ok := nodeOne.Get("a")
	c.Assert(ok, Equals, true)
	c.Assert(value, Equals, "bar")
}

func (s *ConfigNodeSuite) TestMultipleWrites(c *C) {
	// Check that multiple writes only do the right changes.
	node, err := readConfigNode(s.state, s.path)
	c.Assert(err, IsNil)
	node.Update(map[string]interface{}{"foo": "bar", "this": "that"})
	changes, err := node.Write()
	c.Assert(err, IsNil)
	c.Assert(changes, DeepEquals, []ItemChange{
		{ItemAdded, "foo", nil, "bar"},
		{ItemAdded, "this", nil, "that"},
	})
	node.Delete("this")
	node.Set("another", "value")
	changes, err = node.Write()
	c.Assert(err, IsNil)
	c.Assert(changes, DeepEquals, []ItemChange{
		{ItemAdded, "another", nil, "value"},
		{ItemDeleted, "this", "that", nil},
	})

	expected := map[string]interface{}{"foo": "bar", "another": "value"}
	c.Assert(expected, DeepEquals, node.Map())

	changes, err = node.Write()
	c.Assert(err, IsNil)
	c.Assert(changes, DeepEquals, []ItemChange{})

	err = node.Read()
	c.Assert(err, IsNil)
	c.Assert(expected, DeepEquals, node.Map())

	changes, err = node.Write()
	c.Assert(err, IsNil)
	c.Assert(changes, DeepEquals, []ItemChange{})
}

func (s *ConfigNodeSuite) TestMultipleWritesAreStable(c *C) {
	node, err := readConfigNode(s.state, s.path)
	c.Assert(err, IsNil)
	node.Update(map[string]interface{}{"foo": "bar", "this": "that"})
	_, err = node.Write()
	c.Assert(err, IsNil)

	mgoData := make(map[string]interface{})
	err = s.MgoSuite.Session.DB("juju").C("settings").FindId(s.path).One(&mgoData)
	c.Assert(err, IsNil)
	version := mgoData["version"]
	for i := 0; i < 100; i++ {
		node.Set("value", i)
		node.Set("foo", "bar")
		node.Delete("value")
		node.Set("this", "that")
		_, err := node.Write()
		c.Assert(err, IsNil)
	}
	mgoData = make(map[string]interface{})
	err = s.MgoSuite.Session.DB("juju").C("settings").FindId(s.path).One(&mgoData)
	c.Assert(err, IsNil)
	newVersion := mgoData["version"]
	c.Assert(version, Equals, newVersion)
}

func (s *ConfigNodeSuite) TestWriteTwice(c *C) {
	// Check the correct writing into a node by two config nodes.
	nodeOne, err := readConfigNode(s.state, s.path)
	c.Assert(err, IsNil)
	nodeOne.Set("a", "foo")
	changes, err := nodeOne.Write()
	c.Assert(err, IsNil)
	c.Assert(changes, DeepEquals, []ItemChange{
		{ItemAdded, "a", nil, "foo"},
	})

	nodeTwo, err := readConfigNode(s.state, s.path)
	c.Assert(err, IsNil)
	nodeTwo.Set("a", "bar")
	changes, err = nodeTwo.Write()
	c.Assert(err, IsNil)
	c.Assert(changes, DeepEquals, []ItemChange{
		{ItemModified, "a", "foo", "bar"},
	})

	// Shouldn't write again. Changes were already
	// flushed and acted upon by other parties.
	changes, err = nodeOne.Write()
	c.Assert(err, IsNil)
	c.Assert(changes, DeepEquals, []ItemChange{})

	err = nodeOne.Read()
	c.Assert(err, IsNil)
	c.Assert(nodeOne.path, Equals, nodeTwo.path)
	c.Assert(nodeOne.disk, DeepEquals, nodeTwo.disk)
	c.Assert(nodeOne.core, DeepEquals, nodeTwo.core)
}
