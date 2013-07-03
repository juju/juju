// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/testing/checkers"
	"time"
)

type SettingsSuite struct {
	testing.LoggingSuite
	testing.MgoSuite
	state *State
	key   string
}

var _ = Suite(&SettingsSuite{})

// TestingStateInfo returns information suitable for
// connecting to the testing state server.
func TestingStateInfo() *Info {
	return &Info{
		Addrs:  []string{testing.MgoAddr},
		CACert: []byte(testing.CACert),
	}
}

// TestingDialOpts returns configuration parameters for
// connecting to the testing state server.
func TestingDialOpts() DialOpts {
	return DialOpts{
		Timeout: 100 * time.Millisecond,
	}
}

func (s *SettingsSuite) SetUpSuite(c *C) {
	s.LoggingSuite.SetUpSuite(c)
	s.MgoSuite.SetUpSuite(c)
}

func (s *SettingsSuite) TearDownSuite(c *C) {
	s.MgoSuite.TearDownSuite(c)
	s.LoggingSuite.TearDownSuite(c)
}

func (s *SettingsSuite) SetUpTest(c *C) {
	s.LoggingSuite.SetUpTest(c)
	s.MgoSuite.SetUpTest(c)
	// TODO(dfc) this logic is duplicated with the metawatcher_test.
	state, err := Open(TestingStateInfo(), TestingDialOpts())
	c.Assert(err, IsNil)

	s.state = state
	s.key = "config"
}

func (s *SettingsSuite) TearDownTest(c *C) {
	s.state.Close()
	s.MgoSuite.TearDownTest(c)
	s.LoggingSuite.TearDownTest(c)
}

func (s *SettingsSuite) TestCreateEmptySettings(c *C) {
	node, err := createSettings(s.state, s.key, nil)
	c.Assert(err, IsNil)
	c.Assert(node.Keys(), DeepEquals, []string{})
}

func (s *SettingsSuite) TestCannotOverwrite(c *C) {
	_, err := createSettings(s.state, s.key, nil)
	c.Assert(err, IsNil)
	_, err = createSettings(s.state, s.key, nil)
	c.Assert(err, ErrorMatches, "cannot overwrite existing settings")
}

func (s *SettingsSuite) TestCannotReadMissing(c *C) {
	_, err := readSettings(s.state, s.key)
	c.Assert(err, ErrorMatches, "settings not found")
	c.Assert(err, checkers.Satisfies, errors.IsNotFoundError)
}

func (s *SettingsSuite) TestCannotWriteMissing(c *C) {
	node, err := createSettings(s.state, s.key, nil)
	c.Assert(err, IsNil)

	err = removeSettings(s.state, s.key)
	c.Assert(err, IsNil)

	node.Set("foo", "bar")
	_, err = node.Write()
	c.Assert(err, ErrorMatches, "settings not found")
	c.Assert(err, checkers.Satisfies, errors.IsNotFoundError)
}

func (s *SettingsSuite) TestUpdateWithWrite(c *C) {
	node, err := createSettings(s.state, s.key, nil)
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
	err = s.MgoSuite.Session.DB("juju").C("settings").FindId(s.key).One(&mgoData)
	c.Assert(err, IsNil)
	cleanSettingsMap(mgoData)
	c.Assert(mgoData, DeepEquals, options)
}

func (s *SettingsSuite) TestConflictOnSet(c *C) {
	// Check version conflict errors.
	nodeOne, err := createSettings(s.state, s.key, nil)
	c.Assert(err, IsNil)
	nodeTwo, err := readSettings(s.state, s.key)
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

func (s *SettingsSuite) TestSetItem(c *C) {
	// Check that Set works as expected.
	node, err := createSettings(s.state, s.key, nil)
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
	err = s.MgoSuite.Session.DB("juju").C("settings").FindId(s.key).One(&mgoData)
	c.Assert(err, IsNil)
	cleanSettingsMap(mgoData)
	c.Assert(mgoData, DeepEquals, options)
}

func (s *SettingsSuite) TestMultipleReads(c *C) {
	// Check that reads without writes always resets the data.
	nodeOne, err := createSettings(s.state, s.key, nil)
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
	nodeTwo, err := readSettings(s.state, s.key)
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

func (s *SettingsSuite) TestDeleteEmptiesState(c *C) {
	node, err := createSettings(s.state, s.key, nil)
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

func (s *SettingsSuite) TestReadResync(c *C) {
	// Check that read pulls the data into the node.
	nodeOne, err := createSettings(s.state, s.key, nil)
	c.Assert(err, IsNil)
	nodeOne.Set("a", "foo")
	changes, err := nodeOne.Write()
	c.Assert(err, IsNil)
	c.Assert(changes, DeepEquals, []ItemChange{
		{ItemAdded, "a", nil, "foo"},
	})
	nodeTwo, err := readSettings(s.state, s.key)
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

func (s *SettingsSuite) TestMultipleWrites(c *C) {
	// Check that multiple writes only do the right changes.
	node, err := createSettings(s.state, s.key, nil)
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

func (s *SettingsSuite) TestMultipleWritesAreStable(c *C) {
	node, err := createSettings(s.state, s.key, nil)
	c.Assert(err, IsNil)
	node.Update(map[string]interface{}{"foo": "bar", "this": "that"})
	_, err = node.Write()
	c.Assert(err, IsNil)

	mgoData := make(map[string]interface{})
	err = s.MgoSuite.Session.DB("juju").C("settings").FindId(s.key).One(&mgoData)
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
	err = s.MgoSuite.Session.DB("juju").C("settings").FindId(s.key).One(&mgoData)
	c.Assert(err, IsNil)
	newVersion := mgoData["version"]
	c.Assert(version, Equals, newVersion)
}

func (s *SettingsSuite) TestWriteTwice(c *C) {
	// Check the correct writing into a node by two config nodes.
	nodeOne, err := createSettings(s.state, s.key, nil)
	c.Assert(err, IsNil)
	nodeOne.Set("a", "foo")
	changes, err := nodeOne.Write()
	c.Assert(err, IsNil)
	c.Assert(changes, DeepEquals, []ItemChange{
		{ItemAdded, "a", nil, "foo"},
	})

	nodeTwo, err := readSettings(s.state, s.key)
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
	c.Assert(nodeOne.key, Equals, nodeTwo.key)
	c.Assert(nodeOne.disk, DeepEquals, nodeTwo.disk)
	c.Assert(nodeOne.core, DeepEquals, nodeTwo.core)
}
