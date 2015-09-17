// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"sort"
	"strings"

	"github.com/juju/errors"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

// See: http://docs.mongodb.org/manual/faq/developers/#faq-dollar-sign-escaping
// for why we're using those replacements.
const (
	fullWidthDot    = "\uff0e"
	fullWidthDollar = "\uff04"
)

var (
	escapeReplacer   = strings.NewReplacer(".", fullWidthDot, "$", fullWidthDollar)
	unescapeReplacer = strings.NewReplacer(fullWidthDot, ".", fullWidthDollar, "$")
)

const (
	ItemAdded = iota
	ItemModified
	ItemDeleted
)

// ItemChange represents the change of an item in a settings.
type ItemChange struct {
	Type     int
	Key      string
	OldValue interface{}
	NewValue interface{}
}

// String returns the item change in a readable format.
func (ic *ItemChange) String() string {
	switch ic.Type {
	case ItemAdded:
		return fmt.Sprintf("setting added: %v = %v", ic.Key, ic.NewValue)
	case ItemModified:
		return fmt.Sprintf("setting modified: %v = %v (was %v)",
			ic.Key, ic.NewValue, ic.OldValue)
	case ItemDeleted:
		return fmt.Sprintf("setting deleted: %v (was %v)", ic.Key, ic.OldValue)
	}
	return fmt.Sprintf("unknown setting change type %d: %v = %v (was %v)",
		ic.Type, ic.Key, ic.NewValue, ic.OldValue)
}

// itemChangeSlice contains a slice of item changes in a config node.
// It implements the sort interface to sort the items changes by key.
type itemChangeSlice []ItemChange

func (ics itemChangeSlice) Len() int           { return len(ics) }
func (ics itemChangeSlice) Less(i, j int) bool { return ics[i].Key < ics[j].Key }
func (ics itemChangeSlice) Swap(i, j int)      { ics[i], ics[j] = ics[j], ics[i] }

// A Settings manages changes to settings as a delta in memory and merges
// them back in the database when explicitly requested.
type Settings struct {
	st  *State
	key string
	// disk holds the values in the config node before
	// any keys have been changed. It is reset on Read and Write
	// operations.
	disk map[string]interface{}
	// cache holds the current values in the config node.
	// The difference between disk and core
	// determines the delta to be applied when Settings.Write
	// is called.
	core     map[string]interface{}
	txnRevno int64
}

// Keys returns the current keys in alphabetical order.
func (c *Settings) Keys() []string {
	keys := []string{}
	for key := range c.core {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// Get returns the value of key and whether it was found.
func (c *Settings) Get(key string) (value interface{}, found bool) {
	value, found = c.core[key]
	return
}

// Map returns all keys and values of the node.
func (c *Settings) Map() map[string]interface{} {
	return copyMap(c.core, nil)
}

// Set sets key to value
func (c *Settings) Set(key string, value interface{}) {
	c.core[key] = value
}

// Update sets multiple key/value pairs.
func (c *Settings) Update(kv map[string]interface{}) {
	for key, value := range kv {
		c.core[key] = value
	}
}

// Delete removes key.
func (c *Settings) Delete(key string) {
	delete(c.core, key)
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

// Write writes changes made to c back onto its node.  Changes are written
// as a delta applied on top of the latest version of the node, to prevent
// overwriting unrelated changes made to the node since it was last read.
func (c *Settings) Write() ([]ItemChange, error) {
	changes := []ItemChange{}
	updates := bson.M{}
	deletions := bson.M{}
	for key := range cacheKeys(c.disk, c.core) {
		old, ondisk := c.disk[key]
		new, incore := c.core[key]
		if new == old {
			continue
		}
		var change ItemChange
		escapedKey := escapeReplacer.Replace(key)
		switch {
		case incore && ondisk:
			change = ItemChange{ItemModified, key, old, new}
			updates[escapedKey] = new
		case incore && !ondisk:
			change = ItemChange{ItemAdded, key, nil, new}
			updates[escapedKey] = new
		case ondisk && !incore:
			change = ItemChange{ItemDeleted, key, old, nil}
			deletions[escapedKey] = 1
		default:
			panic("unreachable")
		}
		changes = append(changes, change)
	}
	if len(changes) == 0 {
		return []ItemChange{}, nil
	}
	sort.Sort(itemChangeSlice(changes))
	ops := []txn.Op{{
		C:      settingsC,
		Id:     c.st.docID(c.key),
		Assert: txn.DocExists,
		Update: setUnsetUpdate(updates, deletions),
	}}
	err := c.st.runTransaction(ops)
	if err == txn.ErrAborted {
		return nil, errors.NotFoundf("settings")
	}
	if err != nil {
		return nil, fmt.Errorf("cannot write settings: %v", err)
	}
	c.disk = copyMap(c.core, nil)
	return changes, nil
}

func newSettings(st *State, key string) *Settings {
	return &Settings{
		st:   st,
		key:  key,
		core: make(map[string]interface{}),
	}
}

// cleanSettingsMap cleans the map of version, env-uuid and _id fields and
// also unescapes keys coming out of MongoDB.
func cleanSettingsMap(in map[string]interface{}) {
	delete(in, "env-uuid")
	delete(in, "_id")
	delete(in, "txn-revno")
	delete(in, "txn-queue")
	replaceKeys(in, unescapeReplacer.Replace)
}

// replaceKeys will modify the provided map in place by replacing keys with
// their replacement if they have been modified.
func replaceKeys(m map[string]interface{}, replace func(string) string) {
	for key, value := range m {
		if newKey := replace(key); newKey != key {
			delete(m, key)
			m[newKey] = value
		}
	}
	return
}

// copyMap copies the keys and values of one map into a new one.  If replace
// is non-nil, for each old key k, the new key will be replace(k).
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

// Read (re)reads the node data into c.
func (c *Settings) Read() error {
	config, txnRevno, err := readSettingsDoc(c.st, c.key)
	if err == mgo.ErrNotFound {
		c.disk = nil
		c.core = make(map[string]interface{})
		return errors.NotFoundf("settings")
	}
	if err != nil {
		return fmt.Errorf("cannot read settings: %v", err)
	}
	c.txnRevno = txnRevno
	c.disk = config
	c.core = copyMap(config, nil)
	return nil
}

// readSettingsDoc reads the settings with the given
// key. It returns the settings and the current rxnRevno.
func readSettingsDoc(st *State, key string) (map[string]interface{}, int64, error) {
	settings, closer := st.getRawCollection(settingsC)
	defer closer()

	config := map[string]interface{}{}

	err := settings.FindId(st.docID(key)).One(config)

	// This is required to allow loading of environ settings before the
	// environment UUID migration has been applied to the settings collection.
	// Without this, an agent's version cannot be read, blocking the upgrade.
	if err == mgo.ErrNotFound && key == environGlobalKey {
		err := settings.FindId(environGlobalKey).One(config)
		if err != nil {
			return nil, 0, err
		}
	} else if err != nil {
		return nil, 0, err
	}
	txnRevno := config["txn-revno"].(int64)
	cleanSettingsMap(config)
	return config, txnRevno, nil
}

// ReadSettings returns the settings for the given key.
func (st *State) ReadSettings(key string) (*Settings, error) {
	return readSettings(st, key)
}

// readSettings returns the Settings for key.
func readSettings(st *State, key string) (*Settings, error) {
	s := newSettings(st, key)
	if err := s.Read(); err != nil {
		return nil, err
	}
	return s, nil
}

var errSettingsExist = fmt.Errorf("cannot overwrite existing settings")

func createSettingsOp(st *State, key string, values map[string]interface{}) txn.Op {
	newValues := copyMap(values, escapeReplacer.Replace)
	newValues["env-uuid"] = st.EnvironUUID()
	return txn.Op{
		C:      settingsC,
		Id:     st.docID(key),
		Assert: txn.DocMissing,
		Insert: newValues,
	}
}

// createSettings writes an initial config node.
func createSettings(st *State, key string, values map[string]interface{}) (*Settings, error) {
	s := newSettings(st, key)
	s.core = copyMap(values, nil)
	ops := []txn.Op{createSettingsOp(st, key, values)}
	err := s.st.runTransaction(ops)
	if err == txn.ErrAborted {
		return nil, errSettingsExist
	}
	if err != nil {
		return nil, fmt.Errorf("cannot create settings: %v", err)
	}
	return s, nil
}

// removeSettings removes the Settings for key.
func removeSettings(st *State, key string) error {
	ops := []txn.Op{{
		C:      settingsC,
		Id:     st.docID(key),
		Assert: txn.DocExists,
		Remove: true,
	}}
	err := st.runTransaction(ops)
	if err == txn.ErrAborted {
		return errors.NotFoundf("settings")
	} else if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// listSettings returns all the settings with the specified key prefix.
func listSettings(st *State, keyPrefix string) (map[string]map[string]interface{}, error) {
	settings, closer := st.getRawCollection(settingsC)
	defer closer()

	var matchingSettings []map[string]interface{}
	findExpr := fmt.Sprintf("^%s.*$", st.docID(keyPrefix))
	if err := settings.Find(bson.D{{"_id", bson.D{{"$regex", findExpr}}}}).All(&matchingSettings); err != nil {
		return nil, err
	}
	result := make(map[string]map[string]interface{})
	for i := range matchingSettings {
		id := matchingSettings[i]["_id"].(string)
		cleanSettingsMap(matchingSettings[i])
		result[st.localID(id)] = matchingSettings[i]
	}
	return result, nil
}

// replaceSettingsOp returns a txn.Op that deletes the document's contents and
// replaces it with the supplied values, and a function that should be called on
// txn failure to determine whether this operation failed (due to a concurrent
// settings change).
func replaceSettingsOp(st *State, key string, values map[string]interface{}) (txn.Op, func() (bool, error), error) {
	s, err := readSettings(st, key)
	if err != nil {
		return txn.Op{}, nil, err
	}
	deletes := bson.M{}
	for k := range s.disk {
		if _, found := values[k]; !found {
			deletes[escapeReplacer.Replace(k)] = 1
		}
	}
	newValues := copyMap(values, escapeReplacer.Replace)
	op := s.assertUnchangedOp()
	op.Update = setUnsetUpdate(bson.M(newValues), deletes)
	assertFailed := func() (bool, error) {
		latest, err := readSettings(st, key)
		if err != nil {
			return false, err
		}
		return latest.txnRevno != s.txnRevno, nil
	}
	return op, assertFailed, nil
}

func (s *Settings) assertUnchangedOp() txn.Op {
	return txn.Op{
		C:      settingsC,
		Id:     s.st.docID(s.key),
		Assert: bson.D{{"txn-revno", s.txnRevno}},
	}
}

// setUnsetUpdate returns a bson.D for use in
// a txn.Op's Update field, containing $set and
// $unset operators if the corresponding operands
// are non-empty.
func setUnsetUpdate(set, unset bson.M) bson.D {
	var update bson.D
	if len(set) > 0 {
		update = append(update, bson.DocElem{"$set", set})
	}
	if len(unset) > 0 {
		update = append(update, bson.DocElem{"$unset", unset})
	}
	return update
}

// StateSettings is used to expose various settings APIs outside of the state package.
type StateSettings struct {
	st *State
}

// NewStateSettings creates a StateSettings from state.
func NewStateSettings(st *State) *StateSettings {
	return &StateSettings{st}
}

// CreateSettings exposes createSettings on state for use outside the state package.
func (s *StateSettings) CreateSettings(key string, settings map[string]interface{}) error {
	_, err := createSettings(s.st, key, settings)
	return err
}

// ReadSettings exposes readSettings on state for use outside the state package.
func (s *StateSettings) ReadSettings(key string) (map[string]interface{}, error) {
	if settings, err := readSettings(s.st, key); err != nil {
		return nil, err
	} else {
		return settings.Map(), nil
	}
}

// RemoveSettings exposes removeSettings on state for use outside the state package.
func (s *StateSettings) RemoveSettings(key string) error {
	return removeSettings(s.st, key)
}

// ListSettings exposes listSettings on state for use outside the state package.
func (s *StateSettings) ListSettings(keyPrefix string) (map[string]map[string]interface{}, error) {
	return listSettings(s.st, keyPrefix)
}
