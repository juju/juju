// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"reflect"
	"sort"

	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"

	"github.com/juju/juju/core/settings"
	"github.com/juju/juju/internal/mongo/utils"
)

// settingsDoc is the mongo document representation for
// a settings.
type settingsDoc struct {
	DocID     string `bson:"_id"`
	ModelUUID string `bson:"model-uuid"`

	// Settings contains the settings. This must not be
	// omitempty, or migration cannot work correctly.
	Settings settingsMap `bson:"settings"`

	// Version is a version number for the settings,
	// and is increased every time the settings change.
	Version int64 `bson:"version"`
}

type settingsMap map[string]interface{}

func (m *settingsMap) SetBSON(raw bson.Raw) error {
	rawMap := make(map[string]interface{})
	if err := raw.Unmarshal(rawMap); err != nil {
		return err
	}
	*m = utils.UnescapeKeys(rawMap)
	return nil
}

func (m settingsMap) GetBSON() (interface{}, error) {
	escapedMap := utils.EscapeKeys(m)
	return escapedMap, nil
}

// A Settings manages changes to settings as a delta in memory and merges
// them back in the database when explicitly requested.
type Settings struct {
	db         Database
	collection string
	key        string

	// disk holds the values in the config node before
	// any keys have been changed. It is reset on Read and Write
	// operations.
	disk map[string]interface{}

	// cache holds the current values in the config node.
	// The difference between disk and core
	// determines the delta to be applied when Settings.Write
	// is called.
	core map[string]interface{}

	// version is the version corresponding to "disk"; i.e.
	// the value of the version field in the status document
	// when it was read.
	version int64
}

func newSettings(db Database, collection, key string) *Settings {
	return &Settings{
		db:         db,
		collection: collection,
		key:        key,
		core:       make(map[string]interface{}),
	}
}

// Keys returns the current keys in alphabetical order.
func (s *Settings) Keys() []string {
	keys := []string{}
	for key := range s.core {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// Map returns all keys and values of the node.
func (s *Settings) Map() map[string]interface{} {
	return copyMap(s.core, nil)
}

// Set sets key to value
func (s *Settings) Set(key string, value interface{}) {
	s.core[key] = value
}

// Update sets multiple key/value pairs.
func (s *Settings) Update(kv map[string]interface{}) {
	for key, value := range kv {
		s.core[key] = value
	}
}

// Delete removes key.
func (s *Settings) Delete(key string) {
	delete(s.core, key)
}

// settingsUpdateOps returns the item changes and txn ops necessary
// to write the changes made to settings back to the database.
func (s *Settings) settingsUpdateOps() (settings.ItemChanges, []txn.Op) {
	changes := s.changes()
	if len(changes) == 0 {
		return changes, nil
	}

	updates := bson.M{}
	deletions := bson.M{}
	for _, ch := range changes {
		k := utils.EscapeKey(ch.Key)
		switch {
		case ch.IsAddition(), ch.IsModification():
			updates[k] = ch.NewValue
		case ch.IsDeletion():
			deletions[k] = 1
		}
	}

	ops := []txn.Op{{
		C:      s.collection,
		Id:     s.key,
		Assert: txn.DocExists,
		Update: setUnsetUpdateSettings(updates, deletions),
	}}
	return changes, ops
}

// changes compares the live settings with those that were retrieved from the
// database in order to generate a set of changes.
func (s *Settings) changes() settings.ItemChanges {
	var changes settings.ItemChanges

	for key := range cacheKeys(s.disk, s.core) {
		old, onDisk := s.disk[key]
		live, inCore := s.core[key]
		if reflect.DeepEqual(live, old) {
			continue
		}

		var change settings.ItemChange
		switch {
		case inCore && onDisk:
			change = settings.MakeModification(key, old, live)
		case inCore && !onDisk:
			change = settings.MakeAddition(key, live)
		case onDisk && !inCore:
			change = settings.MakeDeletion(key, old)
		default:
			panic("unreachable")
		}
		changes = append(changes, change)
	}
	sort.Sort(changes)
	return changes
}

// cacheKeys returns the keys of all caches as a key=>true map.
func cacheKeys(caches ...map[string]interface{}) map[string]bool {
	keys := make(map[string]bool)
	for _, cache := range caches {
		for key := range cache {
			keys[key] = true
		}
	}
	return keys
}

func (s *Settings) write(ops []txn.Op) error {
	err := s.db.RunTransaction(ops)
	if err == txn.ErrAborted {
		return errors.NotFoundf("settings")
	}
	if err != nil {
		return errors.Annotate(err, "writing settings")
	}
	s.disk = copyMap(s.core, nil)
	return nil
}

// Write writes changes made to c back onto its node.  Changes are written
// as a delta applied on top of the latest version of the node, to prevent
// overwriting unrelated changes made to the node since it was last read.
func (s *Settings) Write() (settings.ItemChanges, error) {
	changes, ops := s.settingsUpdateOps()
	if len(ops) > 0 {
		err := s.write(ops)
		if err != nil {
			return nil, err
		}
	}
	return changes, nil
}

// Read (re)reads the node data into c.
func (s *Settings) Read() error {
	doc, err := readSettingsDoc(s.db, s.collection, s.key)
	if errors.Is(err, errors.NotFound) {
		s.disk = nil
		s.core = make(map[string]interface{})
		return err
	}
	if err != nil {
		return errors.Annotate(err, "cannot read settings")
	}
	s.version = doc.Version
	s.disk = doc.Settings
	s.core = copyMap(s.disk, nil)
	return nil
}

// readSettingsDoc reads the settings doc with the given key.
func readSettingsDoc(db Database, collection, key string) (*settingsDoc, error) {
	var doc settingsDoc
	col, closer := db.GetCollection(collection)
	defer closer()

	err := col.FindId(key).One(&doc)
	if err == mgo.ErrNotFound {
		err = errors.NotFoundf("settings")
	}
	return &doc, err
}

// readSettings returns the Settings for key.
func readSettings(db Database, collection, key string) (*Settings, error) {
	s := newSettings(db, collection, key)
	if err := s.Read(); err != nil {
		return nil, err
	}
	return s, nil
}

func createSettingsOp(collection, key string, values map[string]interface{}) txn.Op {
	newValues := utils.EscapeKeys(values)
	return txn.Op{
		C:      collection,
		Id:     key,
		Assert: txn.DocMissing,
		Insert: &settingsDoc{
			Settings: newValues,
		},
	}
}

func removeSettingsOp(collection, key string) txn.Op {
	return txn.Op{
		C:      collection,
		Id:     key,
		Assert: txn.DocExists,
		Remove: true,
	}
}

// replaceSettingsOp returns a txn.Op that deletes the document's contents and
// replaces it with the supplied values, and a function that should be called on
// txn failure to determine whether this operation failed (due to a concurrent
// settings change).
func replaceSettingsOp(db Database, collection, key string, values map[string]interface{}) (txn.Op, func() (bool, error), error) {
	s, err := readSettings(db, collection, key)
	if err != nil {
		return txn.Op{}, nil, err
	}
	deletes := bson.M{}
	for k := range s.disk {
		if _, found := values[k]; !found {
			deletes[utils.EscapeKey(k)] = 1
		}
	}
	newValues := utils.EscapeKeys(values)
	op := s.assertUnchangedOp()
	op.Update = setUnsetUpdateSettings(newValues, deletes)
	assertFailed := func() (bool, error) {
		latest, err := readSettings(db, collection, key)
		if err != nil {
			return false, err
		}
		return latest.version != s.version, nil
	}
	return op, assertFailed, nil
}

func (s *Settings) assertUnchangedOp() txn.Op {
	return txn.Op{
		C:      s.collection,
		Id:     s.key,
		Assert: bson.D{{"version", s.version}},
	}
}

// setUnsetUpdateSettings returns a bson.D for use
// in a s.collection txn.Op's Update field, containing
// $set and $unset operators if the corresponding
// operands are non-empty.
func setUnsetUpdateSettings(set, unset bson.M) bson.D {
	var update bson.D
	if len(set) > 0 {
		set = subDocKeys(set, "settings")
		update = append(update, bson.DocElem{Name: "$set", Value: set})
	}
	if len(unset) > 0 {
		unset = subDocKeys(unset, "settings")
		update = append(update, bson.DocElem{Name: "$unset", Value: unset})
	}
	if len(update) > 0 {
		update = append(update, bson.DocElem{Name: "$inc", Value: bson.D{{"version", 1}}})
	}
	return update
}

// subDocKeys returns a new map based on the input,
// with keys indicating nesting within an MongoDB sub-document.
func subDocKeys(m map[string]interface{}, subDoc string) map[string]interface{} {
	return copyMap(m, subDocReplacer(subDoc))
}

// copyMap copies the keys and values of one map into a new one.
// If the input replacement function is non-nil, each key in the new map will
// be the result of applying the function to its original.
func copyMap(in map[string]interface{}, replace func(string) string) (out map[string]interface{}) {
	out = make(map[string]interface{})
	for key, value := range in {
		if replace != nil {
			key = replace(key)
		}
		out[key] = value
	}
	return
}

// subDocReplacer returns a replacement function suitable for modifying input
// keys to indicate MongoDB sub-documents.
func subDocReplacer(subDoc string) func(string) string {
	return func(key string) string {
		return subDoc + "." + key
	}
}
