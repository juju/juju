// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"sort"

	"github.com/juju/errors"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"
	jc "github.com/juju/testing/checkers"
	jujutxn "github.com/juju/txn/v3"
	gc "gopkg.in/check.v1"

	coresettings "github.com/juju/juju/core/settings"
)

type SettingsSuite struct {
	internalStateSuite
	key        string
	collection string
}

var _ = gc.Suite(&SettingsSuite{})

func (s *SettingsSuite) SetUpTest(c *gc.C) {
	s.internalStateSuite.SetUpTest(c)
	s.key = "config"
	s.collection = settingsC
}

func (s *SettingsSuite) createSettings(key string, values map[string]interface{}) (*Settings, error) {
	return createSettings(s.state.db(), s.collection, key, values)
}

func (s *SettingsSuite) readSettings() (*Settings, error) {
	return readSettings(s.state.db(), s.collection, s.key)
}

func (s *SettingsSuite) TestCreateEmptySettings(c *gc.C) {
	node, err := s.createSettings(s.key, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(node.Keys(), gc.DeepEquals, []string{})
}

func (s *SettingsSuite) TestCannotOverwrite(c *gc.C) {
	_, err := s.createSettings(s.key, nil)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.createSettings(s.key, nil)
	c.Assert(err, gc.ErrorMatches, "cannot overwrite existing settings")
}

func (s *SettingsSuite) TestCannotReadMissing(c *gc.C) {
	_, err := s.readSettings()
	c.Assert(err, gc.ErrorMatches, "settings not found")
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *SettingsSuite) TestCannotWriteMissing(c *gc.C) {
	node, err := s.createSettings(s.key, nil)
	c.Assert(err, jc.ErrorIsNil)

	err = removeSettings(s.state.db(), s.collection, s.key)
	c.Assert(err, jc.ErrorIsNil)

	node.Set("foo", "bar")
	_, err = node.Write()
	c.Assert(err, gc.ErrorMatches, "settings not found")
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *SettingsSuite) TestUpdateWithWrite(c *gc.C) {
	node, err := s.createSettings(s.key, nil)
	c.Assert(err, jc.ErrorIsNil)
	options := map[string]interface{}{"alpha": "beta", "one": 1}
	node.Update(options)
	changes, err := node.Write()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes, gc.DeepEquals, coresettings.ItemChanges{
		coresettings.MakeAddition("alpha", "beta"),
		coresettings.MakeAddition("one", 1),
	})

	// Check local state.
	c.Assert(node.Map(), gc.DeepEquals, options)

	// Check MongoDB state.
	var mgoData struct {
		Settings settingsMap
	}
	settings, closer := s.state.db().GetCollection(settingsC)
	defer closer()
	err = settings.FindId(s.key).One(&mgoData)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(map[string]interface{}(mgoData.Settings), gc.DeepEquals, options)
}

func (s *SettingsSuite) TestConflictOnSet(c *gc.C) {
	// Check version conflict errors.
	nodeOne, err := s.createSettings(s.key, nil)
	c.Assert(err, jc.ErrorIsNil)
	nodeTwo, err := s.readSettings()
	c.Assert(err, jc.ErrorIsNil)

	optionsOld := map[string]interface{}{"alpha": "beta", "one": 1}
	nodeOne.Update(optionsOld)
	_, err = nodeOne.Write()
	c.Assert(err, jc.ErrorIsNil)

	nodeTwo.Update(optionsOld)
	changes, err := nodeTwo.Write()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes, gc.DeepEquals, coresettings.ItemChanges{
		coresettings.MakeAddition("alpha", "beta"),
		coresettings.MakeAddition("one", 1),
	})

	// First test node one.
	c.Assert(nodeOne.Map(), gc.DeepEquals, optionsOld)

	// Write on node one.
	optionsNew := map[string]interface{}{"alpha": "gamma", "one": "two"}
	nodeOne.Update(optionsNew)
	changes, err = nodeOne.Write()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes, gc.DeepEquals, coresettings.ItemChanges{
		coresettings.MakeModification("alpha", "beta", "gamma"),
		coresettings.MakeModification("one", 1, "two"),
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes, gc.DeepEquals, coresettings.ItemChanges{
		coresettings.MakeModification("alpha", "beta", "cappa"),
		coresettings.MakeAddition("new", "next"),
		coresettings.MakeDeletion("one", 1),
	})
	c.Assert(expected, gc.DeepEquals, nodeTwo.Map())

	// But node one still reflects the former data.
	c.Assert(nodeOne.Map(), gc.DeepEquals, optionsNew)
}

func (s *SettingsSuite) TestSetItem(c *gc.C) {
	// Check that Set works as expected.
	node, err := s.createSettings(s.key, nil)
	c.Assert(err, jc.ErrorIsNil)
	options := map[string]interface{}{"alpha": "beta", "one": 1}
	node.Set("alpha", "beta")
	node.Set("one", 1)
	changes, err := node.Write()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes, gc.DeepEquals, coresettings.ItemChanges{
		coresettings.MakeAddition("alpha", "beta"),
		coresettings.MakeAddition("one", 1),
	})
	// Check local state.
	c.Assert(node.Map(), gc.DeepEquals, options)
	// Check MongoDB state.
	var mgoData struct {
		Settings settingsMap
	}
	settings, closer := s.state.db().GetCollection(settingsC)
	defer closer()
	err = settings.FindId(s.key).One(&mgoData)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(map[string]interface{}(mgoData.Settings), gc.DeepEquals, options)
}

func (s *SettingsSuite) TestSetItemEscape(c *gc.C) {
	// Check that Set works as expected.
	node, err := s.createSettings(s.key, nil)
	c.Assert(err, jc.ErrorIsNil)
	options := map[string]interface{}{"$bar": 1, "foo.alpha": "beta"}
	node.Set("foo.alpha", "beta")
	node.Set("$bar", 1)
	changes, err := node.Write()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes, gc.DeepEquals, coresettings.ItemChanges{
		coresettings.MakeAddition("$bar", 1),
		coresettings.MakeAddition("foo.alpha", "beta"),
	})
	// Check local state.
	c.Assert(node.Map(), gc.DeepEquals, options)

	// Check MongoDB state.
	mgoOptions := map[string]interface{}{"\uff04bar": 1, "foo\uff0ealpha": "beta"}
	var mgoData struct {
		Settings map[string]interface{}
	}
	settings, closer := s.state.db().GetCollection(settingsC)
	defer closer()
	err = settings.FindId(s.key).One(&mgoData)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mgoData.Settings, gc.DeepEquals, mgoOptions)

	// Now get another state by reading from the database instance and
	// check read state has replaced '.' and '$' after fetching from
	// MongoDB.
	nodeTwo, err := s.readSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(nodeTwo.disk, gc.DeepEquals, options)
	c.Assert(nodeTwo.core, gc.DeepEquals, options)
}

func (s *SettingsSuite) TestRawSettingsMapEncodeDecode(c *gc.C) {
	smap := &settingsMap{
		"$dollar":    1,
		"dotted.key": 2,
	}
	asBSON, err := bson.Marshal(smap)
	c.Assert(err, jc.ErrorIsNil)
	var asMap map[string]interface{}
	// unmarshalling into a map doesn't do the custom decoding so we get the raw escaped keys
	err = bson.Unmarshal(asBSON, &asMap)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(asMap, gc.DeepEquals, map[string]interface{}{
		"\uff04dollar":    1,
		"dotted\uff0ekey": 2,
	})
	// unmarshalling into a settingsMap will give us the right decoded keys
	var asSettingsMap settingsMap
	err = bson.Unmarshal(asBSON, &asSettingsMap)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(map[string]interface{}(asSettingsMap), gc.DeepEquals, map[string]interface{}{
		"$dollar":    1,
		"dotted.key": 2,
	})
}

func (s *SettingsSuite) TestReplaceSettingsEscape(c *gc.C) {
	// Check that replaceSettings works as expected.
	node, err := s.createSettings(s.key, nil)
	c.Assert(err, jc.ErrorIsNil)
	node.Set("foo.alpha", "beta")
	node.Set("$bar", 1)
	_, err = node.Write()
	c.Assert(err, jc.ErrorIsNil)

	options := map[string]interface{}{"$baz": 1, "foo.bar": "beta"}
	rop, settingsChanged, err := replaceSettingsOp(s.state.db(), s.collection, s.key, options)
	c.Assert(err, jc.ErrorIsNil)
	ops := []txn.Op{rop}
	err = node.db.RunTransaction(ops)
	c.Assert(err, jc.ErrorIsNil)

	changed, err := settingsChanged()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changed, jc.IsTrue)

	// Check MongoDB state.
	mgoOptions := map[string]interface{}{"\uff04baz": 1, "foo\uff0ebar": "beta"}
	var mgoData struct {
		Settings map[string]interface{}
	}
	settings, closer := s.state.db().GetCollection(settingsC)
	defer closer()
	err = settings.FindId(s.key).One(&mgoData)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mgoData.Settings, gc.DeepEquals, mgoOptions)
}

func (s *SettingsSuite) TestCreateSettingsEscape(c *gc.C) {
	// Check that createSettings works as expected.
	options := map[string]interface{}{"$baz": 1, "foo.bar": "beta"}
	node, err := s.createSettings(s.key, options)
	c.Assert(err, jc.ErrorIsNil)

	// Check local state.
	c.Assert(node.Map(), gc.DeepEquals, options)

	// Check MongoDB state.
	mgoOptions := map[string]interface{}{"\uff04baz": 1, "foo\uff0ebar": "beta"}
	var mgoData struct {
		Settings map[string]interface{}
	}
	settings, closer := s.state.db().GetCollection(settingsC)
	defer closer()

	err = settings.FindId(s.key).One(&mgoData)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mgoData.Settings, gc.DeepEquals, mgoOptions)
}

func (s *SettingsSuite) TestMultipleReads(c *gc.C) {
	// Check that reads without writes always resets the data.
	nodeOne, err := s.createSettings(s.key, nil)
	c.Assert(err, jc.ErrorIsNil)
	nodeOne.Update(map[string]interface{}{"alpha": "beta", "foo": "bar"})
	value, ok := nodeOne.Get("alpha")
	c.Assert(ok, jc.IsTrue)
	c.Assert(value, gc.Equals, "beta")
	value, ok = nodeOne.Get("foo")
	c.Assert(ok, jc.IsTrue)
	c.Assert(value, gc.Equals, "bar")
	_, ok = nodeOne.Get("baz")
	c.Assert(ok, jc.IsFalse)

	// A read resets the data to the empty state.
	err = nodeOne.Read()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(nodeOne.Map(), gc.DeepEquals, map[string]interface{}{})
	nodeOne.Update(map[string]interface{}{"alpha": "beta", "foo": "bar"})
	changes, err := nodeOne.Write()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes, gc.DeepEquals, coresettings.ItemChanges{
		coresettings.MakeAddition("alpha", "beta"),
		coresettings.MakeAddition("foo", "bar"),
	})

	// A write retains the newly set values.
	value, ok = nodeOne.Get("alpha")
	c.Assert(ok, jc.IsTrue)
	c.Assert(value, gc.Equals, "beta")
	value, ok = nodeOne.Get("foo")
	c.Assert(ok, jc.IsTrue)
	c.Assert(value, gc.Equals, "bar")

	// Now get another state instance and change underlying state.
	nodeTwo, err := s.readSettings()
	c.Assert(err, jc.ErrorIsNil)
	nodeTwo.Update(map[string]interface{}{"foo": "different"})
	changes, err = nodeTwo.Write()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes, gc.DeepEquals, coresettings.ItemChanges{
		coresettings.MakeModification("foo", "bar", "different"),
	})

	// This should pull in the new state into node one.
	err = nodeOne.Read()
	c.Assert(err, jc.ErrorIsNil)
	value, ok = nodeOne.Get("alpha")
	c.Assert(ok, jc.IsTrue)
	c.Assert(value, gc.Equals, "beta")
	value, ok = nodeOne.Get("foo")
	c.Assert(ok, jc.IsTrue)
	c.Assert(value, gc.Equals, "different")
}

func (s *SettingsSuite) TestDeleteEmptiesState(c *gc.C) {
	node, err := s.createSettings(s.key, nil)
	c.Assert(err, jc.ErrorIsNil)
	node.Set("a", "foo")
	changes, err := node.Write()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes, gc.DeepEquals, coresettings.ItemChanges{
		coresettings.MakeAddition("a", "foo"),
	})
	node.Delete("a")
	changes, err = node.Write()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes, gc.DeepEquals, coresettings.ItemChanges{
		coresettings.MakeDeletion("a", "foo"),
	})
	c.Assert(node.Map(), gc.DeepEquals, map[string]interface{}{})
}

func (s *SettingsSuite) TestReadReSync(c *gc.C) {
	// Check that read pulls the data into the node.
	nodeOne, err := s.createSettings(s.key, nil)
	c.Assert(err, jc.ErrorIsNil)
	nodeOne.Set("a", "foo")
	changes, err := nodeOne.Write()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes, gc.DeepEquals, coresettings.ItemChanges{
		coresettings.MakeAddition("a", "foo"),
	})
	nodeTwo, err := s.readSettings()
	c.Assert(err, jc.ErrorIsNil)
	nodeTwo.Delete("a")
	changes, err = nodeTwo.Write()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes, gc.DeepEquals, coresettings.ItemChanges{
		coresettings.MakeDeletion("a", "foo"),
	})
	nodeTwo.Set("a", "bar")
	changes, err = nodeTwo.Write()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes, gc.DeepEquals, coresettings.ItemChanges{
		coresettings.MakeAddition("a", "bar"),
	})
	// Read of node one should pick up the new value.
	err = nodeOne.Read()
	c.Assert(err, jc.ErrorIsNil)
	value, ok := nodeOne.Get("a")
	c.Assert(ok, jc.IsTrue)
	c.Assert(value, gc.Equals, "bar")
}

func (s *SettingsSuite) TestMultipleWrites(c *gc.C) {
	// Check that multiple writes only do the right changes.
	node, err := s.createSettings(s.key, nil)
	c.Assert(err, jc.ErrorIsNil)
	node.Update(map[string]interface{}{"foo": "bar", "this": "that"})
	changes, err := node.Write()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes, gc.DeepEquals, coresettings.ItemChanges{
		coresettings.MakeAddition("foo", "bar"),
		coresettings.MakeAddition("this", "that"),
	})
	node.Delete("this")
	node.Set("another", "value")
	changes, err = node.Write()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes, gc.DeepEquals, coresettings.ItemChanges{
		coresettings.MakeAddition("another", "value"),
		coresettings.MakeDeletion("this", "that"),
	})

	expected := map[string]interface{}{"foo": "bar", "another": "value"}
	c.Assert(expected, gc.DeepEquals, node.Map())

	changes, err = node.Write()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes, gc.DeepEquals, coresettings.ItemChanges(nil))

	err = node.Read()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(expected, gc.DeepEquals, node.Map())

	changes, err = node.Write()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes, gc.DeepEquals, coresettings.ItemChanges(nil))
}

func (s *SettingsSuite) TestMultipleWritesAreStable(c *gc.C) {
	node, err := s.createSettings(s.key, nil)
	c.Assert(err, jc.ErrorIsNil)
	node.Update(map[string]interface{}{"foo": "bar", "this": "that"})
	_, err = node.Write()
	c.Assert(err, jc.ErrorIsNil)

	var mgoData struct {
		Settings map[string]interface{}
	}
	settings, closer := s.state.db().GetCollection(settingsC)
	defer closer()
	err = settings.FindId(s.key).One(&mgoData)
	c.Assert(err, jc.ErrorIsNil)
	version := mgoData.Settings["version"]
	for i := 0; i < 100; i++ {
		node.Set("value", i)
		node.Set("foo", "bar")
		node.Delete("value")
		node.Set("this", "that")
		_, err := node.Write()
		c.Assert(err, jc.ErrorIsNil)
	}
	mgoData.Settings = make(map[string]interface{})
	err = settings.FindId(s.key).One(&mgoData)
	c.Assert(err, jc.ErrorIsNil)
	newVersion := mgoData.Settings["version"]
	c.Assert(version, gc.Equals, newVersion)
}

func (s *SettingsSuite) TestWriteTwice(c *gc.C) {
	// Check the correct writing into a node by two config nodes.
	nodeOne, err := s.createSettings(s.key, nil)
	c.Assert(err, jc.ErrorIsNil)
	nodeOne.Set("a", "foo")
	changes, err := nodeOne.Write()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes, gc.DeepEquals, coresettings.ItemChanges{
		coresettings.MakeAddition("a", "foo"),
	})

	nodeTwo, err := s.readSettings()
	c.Assert(err, jc.ErrorIsNil)
	nodeTwo.Set("a", "bar")
	changes, err = nodeTwo.Write()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes, gc.DeepEquals, coresettings.ItemChanges{
		coresettings.MakeModification("a", "foo", "bar"),
	})

	// Shouldn't write again. Changes were already
	// flushed and acted upon by other parties.
	changes, err = nodeOne.Write()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes, gc.DeepEquals, coresettings.ItemChanges(nil))

	err = nodeOne.Read()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(nodeOne.key, gc.Equals, nodeTwo.key)
	c.Assert(nodeOne.disk, gc.DeepEquals, nodeTwo.disk)
	c.Assert(nodeOne.core, gc.DeepEquals, nodeTwo.core)
}

func (s *SettingsSuite) TestWriteTwiceUsingModelOperation(c *gc.C) {
	// Check the correct writing into a node by two config nodes.
	nodeOne, err := s.createSettings(s.key, nil)
	c.Assert(err, jc.ErrorIsNil)
	nodeOne.Set("a", "foo")
	err = s.state.ApplyOperation(nodeOne.WriteOperation())
	c.Assert(err, jc.ErrorIsNil)

	nodeTwo, err := s.readSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(nodeTwo.Map(), gc.DeepEquals, map[string]interface{}{
		"a": "foo",
	}, gc.Commentf("model operation failed to update db"))
	nodeTwo.Set("a", "bar")
	err = s.state.ApplyOperation(nodeTwo.WriteOperation())
	c.Assert(err, jc.ErrorIsNil)

	// Shouldn't write again. Changes were already
	// flushed and acted upon by other parties.
	_, err = nodeOne.WriteOperation().Build(0)
	c.Assert(err, gc.Equals, jujutxn.ErrNoOperations)

	err = nodeOne.Read()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(nodeOne.key, gc.Equals, nodeTwo.key)
	c.Assert(nodeOne.disk, gc.DeepEquals, nodeTwo.disk)
	c.Assert(nodeOne.core, gc.DeepEquals, nodeTwo.core)
}

func (s *SettingsSuite) TestList(c *gc.C) {
	_, err := s.createSettings("key#1", map[string]interface{}{"foo1": "bar1"})
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.createSettings("key#2", map[string]interface{}{"foo2": "bar2"})
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.createSettings("another#1", map[string]interface{}{"foo2": "bar2"})
	c.Assert(err, jc.ErrorIsNil)

	nodes, err := listSettings(s.state, s.collection, "key#")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(nodes, jc.DeepEquals, map[string]map[string]interface{}{
		"key#1": {"foo1": "bar1"},
		"key#2": {"foo2": "bar2"},
	})
}

func (s *SettingsSuite) TestReplaceSettings(c *gc.C) {
	_, err := s.createSettings(s.key, map[string]interface{}{"foo1": "bar1", "foo2": "bar2"})
	c.Assert(err, jc.ErrorIsNil)
	options := map[string]interface{}{"alpha": "beta", "foo2": "zap100"}
	err = replaceSettings(s.state.db(), s.collection, s.key, options)
	c.Assert(err, jc.ErrorIsNil)

	// Check MongoDB state.
	var mgoData struct {
		Settings settingsMap
	}
	settings, closer := s.state.db().GetCollection(settingsC)
	defer closer()
	err = settings.FindId(s.key).One(&mgoData)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(
		map[string]interface{}(mgoData.Settings),
		gc.DeepEquals,
		map[string]interface{}{
			"alpha": "beta", "foo2": "zap100",
		})
}

func (s *SettingsSuite) TestReplaceSettingsNotFound(c *gc.C) {
	options := map[string]interface{}{"alpha": "beta", "foo2": "zap100"}
	err := replaceSettings(s.state.db(), s.collection, s.key, options)
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *SettingsSuite) TestUpdatingInterfaceSliceValue(c *gc.C) {
	// When storing config values that are coerced from schemas as
	// List(Something), the value will always be a []interface{}. Make
	// sure we can safely update settings with those values.
	s1, err := s.createSettings(s.key, map[string]interface{}{
		"foo1": []interface{}{"bar1"},
	})
	c.Assert(err, jc.ErrorIsNil)
	_, err = s1.Write()
	c.Assert(err, jc.ErrorIsNil)

	s2, err := s.readSettings()
	c.Assert(err, jc.ErrorIsNil)
	s2.Set("foo1", []interface{}{"bar1", "bar2"})
	_, err = s2.Write()
	c.Assert(err, jc.ErrorIsNil)

	s3, err := s.readSettings()
	c.Assert(err, jc.ErrorIsNil)
	value, found := s3.Get("foo1")
	c.Assert(found, gc.Equals, true)
	c.Assert(value, gc.DeepEquals, []interface{}{"bar1", "bar2"})
}

func (s *SettingsSuite) TestApplyAndRetrieveChanges(c *gc.C) {
	s1, err := s.createSettings(s.key, map[string]interface{}{
		"foo.dot":      "bar",
		"alpha$dollar": "beta",
		"number":       1,
	})
	c.Assert(err, jc.ErrorIsNil)
	_, err = s1.Write()
	c.Assert(err, jc.ErrorIsNil)

	s2, err := s.readSettings()
	c.Assert(err, jc.ErrorIsNil)

	// Add, update, update one not present, delete, delete one not present,
	// leave one alone.
	s2.applyChanges(coresettings.ItemChanges{
		coresettings.MakeModification("foo.dot", "no-matter", "new-bar"),
		coresettings.MakeModification("make", "no-matter", "new"),
		coresettings.MakeDeletion("alpha$dollar", "no-matter"),
		coresettings.MakeDeletion("what", "the"),
		coresettings.MakeAddition("new", "noob"),
	})

	// Updating one not present = addition, deleting one not present = no-op.
	exp := coresettings.ItemChanges{
		coresettings.MakeModification("foo.dot", "bar", "new-bar"),
		coresettings.MakeAddition("make", "new"),
		coresettings.MakeDeletion("alpha$dollar", "beta"),
		coresettings.MakeAddition("new", "noob"),
	}
	sort.Sort(exp)

	c.Assert(s2.changes(), gc.DeepEquals, exp)
}

func (s *SettingsSuite) TestDeltaOpsSuccess(c *gc.C) {
	s1, err := s.createSettings(s.key, map[string]interface{}{
		"foo": []interface{}{"bar"},
	})
	c.Assert(err, jc.ErrorIsNil)
	_, err = s1.Write()
	c.Assert(err, jc.ErrorIsNil)

	delta := coresettings.ItemChanges{
		coresettings.MakeModification("foo", "bar", "new-bar"),
		coresettings.MakeAddition("new", "value"),
	}

	settings := s.state.NewSettings()
	ops, err := settings.DeltaOps(s.key, delta)
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.db().RunTransaction(ops)
	c.Assert(err, jc.ErrorIsNil)

	s1.Read()
	c.Assert(s1.Map(), gc.DeepEquals, map[string]interface{}{
		"foo": "new-bar",
		"new": "value",
	})
}

func (s *SettingsSuite) TestDeltaOpsNoChanges(c *gc.C) {
	s1, err := s.createSettings(s.key, map[string]interface{}{
		"foo": []interface{}{"bar"},
	})
	c.Assert(err, jc.ErrorIsNil)
	_, err = s1.Write()
	c.Assert(err, jc.ErrorIsNil)

	settings := s.state.NewSettings()
	ops, err := settings.DeltaOps(s.key, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ops, gc.IsNil)
}

func (s *SettingsSuite) TestDeltaOpsChangedError(c *gc.C) {
	s1, err := s.createSettings(s.key, map[string]interface{}{
		"foo": []interface{}{"bar"},
	})
	c.Assert(err, jc.ErrorIsNil)
	_, err = s1.Write()
	c.Assert(err, jc.ErrorIsNil)

	settings := s.state.NewSettings()

	delta := coresettings.ItemChanges{
		coresettings.MakeModification("foo", "bar", "new-bar"),
	}

	ops, err := settings.DeltaOps(s.key, delta)
	c.Assert(err, jc.ErrorIsNil)

	// Change after settings above is materialised.
	s1.Set("foo", "changed-bar")
	_, err = s1.Write()
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.db().RunTransaction(ops)
	c.Assert(err, gc.ErrorMatches, "transaction aborted")
}
