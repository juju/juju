// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"labix.org/v2/mgo/txn"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/testing/testbase"
)

type SettingsSuite struct {
	testbase.LoggingSuite
	testing.MgoSuite
	state *State
	key   string
}

var _ = gc.Suite(&SettingsSuite{})

// TestingStateInfo returns information suitable for
// connecting to the testing state server.
func TestingStateInfo() *Info {
	return &Info{
		Addrs:  []string{testing.MgoServer.Addr()},
		CACert: []byte(testing.CACert),
	}
}

// TestingDialOpts returns configuration parameters for
// connecting to the testing state server.
func TestingDialOpts() DialOpts {
	return DialOpts{
		Timeout: testing.LongWait,
	}
}

func (s *SettingsSuite) SetUpSuite(c *gc.C) {
	s.LoggingSuite.SetUpSuite(c)
	s.MgoSuite.SetUpSuite(c)
}

func (s *SettingsSuite) TearDownSuite(c *gc.C) {
	s.MgoSuite.TearDownSuite(c)
	s.LoggingSuite.TearDownSuite(c)
}

func (s *SettingsSuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
	s.MgoSuite.SetUpTest(c)
	// TODO(dfc) this logic is duplicated with the metawatcher_test.
	state, err := Open(TestingStateInfo(), TestingDialOpts(), Policy(nil))
	c.Assert(err, gc.IsNil)

	s.state = state
	s.key = "config"
}

func (s *SettingsSuite) TearDownTest(c *gc.C) {
	s.state.Close()
	s.MgoSuite.TearDownTest(c)
	s.LoggingSuite.TearDownTest(c)
}

func (s *SettingsSuite) TestCreateEmptySettings(c *gc.C) {
	node, err := createSettings(s.state, s.key, nil)
	c.Assert(err, gc.IsNil)
	c.Assert(node.Keys(), gc.DeepEquals, []string{})
}

func (s *SettingsSuite) TestCannotOverwrite(c *gc.C) {
	_, err := createSettings(s.state, s.key, nil)
	c.Assert(err, gc.IsNil)
	_, err = createSettings(s.state, s.key, nil)
	c.Assert(err, gc.ErrorMatches, "cannot overwrite existing settings")
}

func (s *SettingsSuite) TestCannotReadMissing(c *gc.C) {
	_, err := readSettings(s.state, s.key)
	c.Assert(err, gc.ErrorMatches, "settings not found")
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)
}

func (s *SettingsSuite) TestCannotWriteMissing(c *gc.C) {
	node, err := createSettings(s.state, s.key, nil)
	c.Assert(err, gc.IsNil)

	err = removeSettings(s.state, s.key)
	c.Assert(err, gc.IsNil)

	node.Set("foo", "bar")
	_, err = node.Write()
	c.Assert(err, gc.ErrorMatches, "settings not found")
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)
}

func (s *SettingsSuite) TestUpdateWithWrite(c *gc.C) {
	node, err := createSettings(s.state, s.key, nil)
	c.Assert(err, gc.IsNil)
	options := map[string]interface{}{"alpha": "beta", "one": 1}
	node.Update(options)
	changes, err := node.Write()
	c.Assert(err, gc.IsNil)
	c.Assert(changes, gc.DeepEquals, []ItemChange{
		{ItemAdded, "alpha", nil, "beta"},
		{ItemAdded, "one", nil, 1},
	})

	// Check local state.
	c.Assert(node.Map(), gc.DeepEquals, options)

	// Check MongoDB state.
	mgoData := make(map[string]interface{}, 0)
	err = s.MgoSuite.Session.DB("juju").C("settings").FindId(s.key).One(&mgoData)
	c.Assert(err, gc.IsNil)
	cleanSettingsMap(mgoData)
	c.Assert(mgoData, gc.DeepEquals, options)
}

func (s *SettingsSuite) TestConflictOnSet(c *gc.C) {
	// Check version conflict errors.
	nodeOne, err := createSettings(s.state, s.key, nil)
	c.Assert(err, gc.IsNil)
	nodeTwo, err := readSettings(s.state, s.key)
	c.Assert(err, gc.IsNil)

	optionsOld := map[string]interface{}{"alpha": "beta", "one": 1}
	nodeOne.Update(optionsOld)
	nodeOne.Write()

	nodeTwo.Update(optionsOld)
	changes, err := nodeTwo.Write()
	c.Assert(err, gc.IsNil)
	c.Assert(changes, gc.DeepEquals, []ItemChange{
		{ItemAdded, "alpha", nil, "beta"},
		{ItemAdded, "one", nil, 1},
	})

	// First test node one.
	c.Assert(nodeOne.Map(), gc.DeepEquals, optionsOld)

	// Write on node one.
	optionsNew := map[string]interface{}{"alpha": "gamma", "one": "two"}
	nodeOne.Update(optionsNew)
	changes, err = nodeOne.Write()
	c.Assert(err, gc.IsNil)
	c.Assert(changes, gc.DeepEquals, []ItemChange{
		{ItemModified, "alpha", "beta", "gamma"},
		{ItemModified, "one", 1, "two"},
	})

	// Verify that node one reports as expected.
	c.Assert(nodeOne.Map(), gc.DeepEquals, optionsNew)

	// Verify that node two has still the old data.
	c.Assert(nodeTwo.Map(), gc.DeepEquals, optionsOld)

	// Now issue a Set/Write from node two. This will
	// merge the data deleting 'one' and updating
	// other values.
	optionsMerge := map[string]interface{}{"alpha": "cappa", "new": "next"}
	nodeTwo.Update(optionsMerge)
	nodeTwo.Delete("one")

	expected := map[string]interface{}{"alpha": "cappa", "new": "next"}
	changes, err = nodeTwo.Write()
	c.Assert(err, gc.IsNil)
	c.Assert(changes, gc.DeepEquals, []ItemChange{
		{ItemModified, "alpha", "beta", "cappa"},
		{ItemAdded, "new", nil, "next"},
		{ItemDeleted, "one", 1, nil},
	})
	c.Assert(expected, gc.DeepEquals, nodeTwo.Map())

	// But node one still reflects the former data.
	c.Assert(nodeOne.Map(), gc.DeepEquals, optionsNew)
}

func (s *SettingsSuite) TestSetItem(c *gc.C) {
	// Check that Set works as expected.
	node, err := createSettings(s.state, s.key, nil)
	c.Assert(err, gc.IsNil)
	options := map[string]interface{}{"alpha": "beta", "one": 1}
	node.Set("alpha", "beta")
	node.Set("one", 1)
	changes, err := node.Write()
	c.Assert(err, gc.IsNil)
	c.Assert(changes, gc.DeepEquals, []ItemChange{
		{ItemAdded, "alpha", nil, "beta"},
		{ItemAdded, "one", nil, 1},
	})
	// Check local state.
	c.Assert(node.Map(), gc.DeepEquals, options)
	// Check MongoDB state.
	mgoData := make(map[string]interface{}, 0)
	err = s.MgoSuite.Session.DB("juju").C("settings").FindId(s.key).One(&mgoData)
	c.Assert(err, gc.IsNil)
	cleanSettingsMap(mgoData)
	c.Assert(mgoData, gc.DeepEquals, options)
}

func (s *SettingsSuite) TestSetItemEscape(c *gc.C) {
	// Check that Set works as expected.
	node, err := createSettings(s.state, s.key, nil)
	c.Assert(err, gc.IsNil)
	options := map[string]interface{}{"$bar": 1, "foo.alpha": "beta"}
	node.Set("foo.alpha", "beta")
	node.Set("$bar", 1)
	changes, err := node.Write()
	c.Assert(err, gc.IsNil)
	c.Assert(changes, gc.DeepEquals, []ItemChange{
		{ItemAdded, "$bar", nil, 1},
		{ItemAdded, "foo.alpha", nil, "beta"},
	})
	// Check local state.
	c.Assert(node.Map(), gc.DeepEquals, options)

	// Check MongoDB state.
	mgoOptions := map[string]interface{}{"\uff04bar": 1, "foo\uff0ealpha": "beta"}
	mgoData := make(map[string]interface{}, 0)
	err = s.MgoSuite.Session.DB("juju").C("settings").FindId(s.key).One(&mgoData)
	c.Assert(err, gc.IsNil)
	cleanMgoSettings(mgoData)
	c.Assert(mgoData, gc.DeepEquals, mgoOptions)

	// Now get another state by reading from the database instance and
	// check read state has replaced '.' and '$' after fetching from
	// MongoDB.
	nodeTwo, err := readSettings(s.state, s.key)
	c.Assert(err, gc.IsNil)
	c.Assert(nodeTwo.disk, gc.DeepEquals, options)
	c.Assert(nodeTwo.core, gc.DeepEquals, options)
}

func (s *SettingsSuite) TestReplaceSettingsEscape(c *gc.C) {
	// Check that replaceSettings works as expected.
	node, err := createSettings(s.state, s.key, nil)
	c.Assert(err, gc.IsNil)
	node.Set("foo.alpha", "beta")
	node.Set("$bar", 1)
	_, err = node.Write()
	c.Assert(err, gc.IsNil)

	options := map[string]interface{}{"$baz": 1, "foo.bar": "beta"}
	rop, settingsChanged, err := replaceSettingsOp(s.state, s.key, options)
	c.Assert(err, gc.IsNil)
	ops := []txn.Op{rop}
	err = node.st.runTransaction(ops)
	c.Assert(err, gc.IsNil)

	changed, err := settingsChanged()
	c.Assert(err, gc.IsNil)
	c.Assert(changed, gc.Equals, true)

	// Check MongoDB state.
	mgoOptions := map[string]interface{}{"\uff04baz": 1, "foo\uff0ebar": "beta"}
	mgoData := make(map[string]interface{}, 0)
	err = s.MgoSuite.Session.DB("juju").C("settings").FindId(s.key).One(&mgoData)
	c.Assert(err, gc.IsNil)
	cleanMgoSettings(mgoData)
	c.Assert(mgoData, gc.DeepEquals, mgoOptions)
}

func (s *SettingsSuite) TestCreateSettingsEscape(c *gc.C) {
	// Check that createSettings works as expected.
	options := map[string]interface{}{"$baz": 1, "foo.bar": "beta"}
	node, err := createSettings(s.state, s.key, options)
	c.Assert(err, gc.IsNil)

	// Check local state.
	c.Assert(node.Map(), gc.DeepEquals, options)

	// Check MongoDB state.
	mgoOptions := map[string]interface{}{"\uff04baz": 1, "foo\uff0ebar": "beta"}
	mgoData := make(map[string]interface{}, 0)
	err = s.MgoSuite.Session.DB("juju").C("settings").FindId(s.key).One(&mgoData)
	c.Assert(err, gc.IsNil)
	cleanMgoSettings(mgoData)
	c.Assert(mgoData, gc.DeepEquals, mgoOptions)
}

func (s *SettingsSuite) TestMultipleReads(c *gc.C) {
	// Check that reads without writes always resets the data.
	nodeOne, err := createSettings(s.state, s.key, nil)
	c.Assert(err, gc.IsNil)
	nodeOne.Update(map[string]interface{}{"alpha": "beta", "foo": "bar"})
	value, ok := nodeOne.Get("alpha")
	c.Assert(ok, gc.Equals, true)
	c.Assert(value, gc.Equals, "beta")
	value, ok = nodeOne.Get("foo")
	c.Assert(ok, gc.Equals, true)
	c.Assert(value, gc.Equals, "bar")
	value, ok = nodeOne.Get("baz")
	c.Assert(ok, gc.Equals, false)

	// A read resets the data to the empty state.
	err = nodeOne.Read()
	c.Assert(err, gc.IsNil)
	c.Assert(nodeOne.Map(), gc.DeepEquals, map[string]interface{}{})
	nodeOne.Update(map[string]interface{}{"alpha": "beta", "foo": "bar"})
	changes, err := nodeOne.Write()
	c.Assert(err, gc.IsNil)
	c.Assert(changes, gc.DeepEquals, []ItemChange{
		{ItemAdded, "alpha", nil, "beta"},
		{ItemAdded, "foo", nil, "bar"},
	})

	// A write retains the newly set values.
	value, ok = nodeOne.Get("alpha")
	c.Assert(ok, gc.Equals, true)
	c.Assert(value, gc.Equals, "beta")
	value, ok = nodeOne.Get("foo")
	c.Assert(ok, gc.Equals, true)
	c.Assert(value, gc.Equals, "bar")

	// Now get another state instance and change underlying state.
	nodeTwo, err := readSettings(s.state, s.key)
	c.Assert(err, gc.IsNil)
	nodeTwo.Update(map[string]interface{}{"foo": "different"})
	changes, err = nodeTwo.Write()
	c.Assert(err, gc.IsNil)
	c.Assert(changes, gc.DeepEquals, []ItemChange{
		{ItemModified, "foo", "bar", "different"},
	})

	// This should pull in the new state into node one.
	err = nodeOne.Read()
	c.Assert(err, gc.IsNil)
	value, ok = nodeOne.Get("alpha")
	c.Assert(ok, gc.Equals, true)
	c.Assert(value, gc.Equals, "beta")
	value, ok = nodeOne.Get("foo")
	c.Assert(ok, gc.Equals, true)
	c.Assert(value, gc.Equals, "different")
}

func (s *SettingsSuite) TestDeleteEmptiesState(c *gc.C) {
	node, err := createSettings(s.state, s.key, nil)
	c.Assert(err, gc.IsNil)
	node.Set("a", "foo")
	changes, err := node.Write()
	c.Assert(err, gc.IsNil)
	c.Assert(changes, gc.DeepEquals, []ItemChange{
		{ItemAdded, "a", nil, "foo"},
	})
	node.Delete("a")
	changes, err = node.Write()
	c.Assert(err, gc.IsNil)
	c.Assert(changes, gc.DeepEquals, []ItemChange{
		{ItemDeleted, "a", "foo", nil},
	})
	c.Assert(node.Map(), gc.DeepEquals, map[string]interface{}{})
}

func (s *SettingsSuite) TestReadResync(c *gc.C) {
	// Check that read pulls the data into the node.
	nodeOne, err := createSettings(s.state, s.key, nil)
	c.Assert(err, gc.IsNil)
	nodeOne.Set("a", "foo")
	changes, err := nodeOne.Write()
	c.Assert(err, gc.IsNil)
	c.Assert(changes, gc.DeepEquals, []ItemChange{
		{ItemAdded, "a", nil, "foo"},
	})
	nodeTwo, err := readSettings(s.state, s.key)
	c.Assert(err, gc.IsNil)
	nodeTwo.Delete("a")
	changes, err = nodeTwo.Write()
	c.Assert(err, gc.IsNil)
	c.Assert(changes, gc.DeepEquals, []ItemChange{
		{ItemDeleted, "a", "foo", nil},
	})
	nodeTwo.Set("a", "bar")
	changes, err = nodeTwo.Write()
	c.Assert(err, gc.IsNil)
	c.Assert(changes, gc.DeepEquals, []ItemChange{
		{ItemAdded, "a", nil, "bar"},
	})
	// Read of node one should pick up the new value.
	err = nodeOne.Read()
	c.Assert(err, gc.IsNil)
	value, ok := nodeOne.Get("a")
	c.Assert(ok, gc.Equals, true)
	c.Assert(value, gc.Equals, "bar")
}

func (s *SettingsSuite) TestMultipleWrites(c *gc.C) {
	// Check that multiple writes only do the right changes.
	node, err := createSettings(s.state, s.key, nil)
	c.Assert(err, gc.IsNil)
	node.Update(map[string]interface{}{"foo": "bar", "this": "that"})
	changes, err := node.Write()
	c.Assert(err, gc.IsNil)
	c.Assert(changes, gc.DeepEquals, []ItemChange{
		{ItemAdded, "foo", nil, "bar"},
		{ItemAdded, "this", nil, "that"},
	})
	node.Delete("this")
	node.Set("another", "value")
	changes, err = node.Write()
	c.Assert(err, gc.IsNil)
	c.Assert(changes, gc.DeepEquals, []ItemChange{
		{ItemAdded, "another", nil, "value"},
		{ItemDeleted, "this", "that", nil},
	})

	expected := map[string]interface{}{"foo": "bar", "another": "value"}
	c.Assert(expected, gc.DeepEquals, node.Map())

	changes, err = node.Write()
	c.Assert(err, gc.IsNil)
	c.Assert(changes, gc.DeepEquals, []ItemChange{})

	err = node.Read()
	c.Assert(err, gc.IsNil)
	c.Assert(expected, gc.DeepEquals, node.Map())

	changes, err = node.Write()
	c.Assert(err, gc.IsNil)
	c.Assert(changes, gc.DeepEquals, []ItemChange{})
}

func (s *SettingsSuite) TestMultipleWritesAreStable(c *gc.C) {
	node, err := createSettings(s.state, s.key, nil)
	c.Assert(err, gc.IsNil)
	node.Update(map[string]interface{}{"foo": "bar", "this": "that"})
	_, err = node.Write()
	c.Assert(err, gc.IsNil)

	mgoData := make(map[string]interface{})
	err = s.MgoSuite.Session.DB("juju").C("settings").FindId(s.key).One(&mgoData)
	c.Assert(err, gc.IsNil)
	version := mgoData["version"]
	for i := 0; i < 100; i++ {
		node.Set("value", i)
		node.Set("foo", "bar")
		node.Delete("value")
		node.Set("this", "that")
		_, err := node.Write()
		c.Assert(err, gc.IsNil)
	}
	mgoData = make(map[string]interface{})
	err = s.MgoSuite.Session.DB("juju").C("settings").FindId(s.key).One(&mgoData)
	c.Assert(err, gc.IsNil)
	newVersion := mgoData["version"]
	c.Assert(version, gc.Equals, newVersion)
}

func (s *SettingsSuite) TestWriteTwice(c *gc.C) {
	// Check the correct writing into a node by two config nodes.
	nodeOne, err := createSettings(s.state, s.key, nil)
	c.Assert(err, gc.IsNil)
	nodeOne.Set("a", "foo")
	changes, err := nodeOne.Write()
	c.Assert(err, gc.IsNil)
	c.Assert(changes, gc.DeepEquals, []ItemChange{
		{ItemAdded, "a", nil, "foo"},
	})

	nodeTwo, err := readSettings(s.state, s.key)
	c.Assert(err, gc.IsNil)
	nodeTwo.Set("a", "bar")
	changes, err = nodeTwo.Write()
	c.Assert(err, gc.IsNil)
	c.Assert(changes, gc.DeepEquals, []ItemChange{
		{ItemModified, "a", "foo", "bar"},
	})

	// Shouldn't write again. Changes were already
	// flushed and acted upon by other parties.
	changes, err = nodeOne.Write()
	c.Assert(err, gc.IsNil)
	c.Assert(changes, gc.DeepEquals, []ItemChange{})

	err = nodeOne.Read()
	c.Assert(err, gc.IsNil)
	c.Assert(nodeOne.key, gc.Equals, nodeTwo.key)
	c.Assert(nodeOne.disk, gc.DeepEquals, nodeTwo.disk)
	c.Assert(nodeOne.core, gc.DeepEquals, nodeTwo.core)
}

// cleanMgoSettings will remove MongoDB-specific settings but not unescape any
// keys, as opposed to cleanSettingsMap which does unescape keys.
func cleanMgoSettings(in map[string]interface{}) {
	delete(in, "_id")
	delete(in, "txn-revno")
	delete(in, "txn-queue")
}
