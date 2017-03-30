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
	replaceKeys(rawMap, unescapeReplacer.Replace)
	*m = settingsMap(rawMap)
	return nil
}

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
	backend    modelBackend
	db         Database // For convenience
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

// Keys returns the current keys in alphabetical order.
func (s *Settings) Keys() []string {
	keys := []string{}
	for key := range s.core {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// Get returns the value of key and whether it was found.
func (s *Settings) Get(key string) (value interface{}, found bool) {
	value, found = s.core[key]
	return
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

// settingsUpdateOps returns the item changes and txn ops necessary
// to write the changes made to c back onto its node.
func (s *Settings) settingsUpdateOps() ([]ItemChange, []txn.Op) {
	changes := []ItemChange{}
	updates := bson.M{}
	deletions := bson.M{}
	for key := range cacheKeys(s.disk, s.core) {
		old, ondisk := s.disk[key]
		new, incore := s.core[key]
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
		C:      s.collection,
		Id:     s.key,
		Assert: txn.DocExists,
		Update: setUnsetUpdateSettings(updates, deletions),
	}}
	return changes, ops
}

func (s *Settings) write(ops []txn.Op) error {
	err := s.db.RunTransaction(ops)
	if err == txn.ErrAborted {
		return errors.NotFoundf("settings")
	}
	if err != nil {
		return fmt.Errorf("cannot write settings: %v", err)
	}
	s.disk = copyMap(s.core, nil)
	return nil
}

// Write writes changes made to c back onto its node.  Changes are written
// as a delta applied on top of the latest version of the node, to prevent
// overwriting unrelated changes made to the node since it was last read.
func (s *Settings) Write() ([]ItemChange, error) {
	changes, ops := s.settingsUpdateOps()
	err := s.write(ops)
	if err != nil {
		return nil, err
	}
	return changes, nil
}

func newSettings(backend modelBackend, collection, key string) *Settings {
	return &Settings{
		backend:    backend,
		db:         backend.db(),
		collection: collection,
		key:        key,
		core:       make(map[string]interface{}),
	}
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
func (s *Settings) Read() error {
	doc, err := readSettingsDoc(s.backend, s.collection, s.key)
	if errors.IsNotFound(err) {
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
func readSettingsDoc(st modelBackend, collection, key string) (*settingsDoc, error) {
	var doc settingsDoc
	if err := readSettingsDocInto(st, collection, key, &doc); err != nil {
		return nil, errors.Trace(err)
	}
	return &doc, nil
}

// readSettingsDocInto reads the settings doc with the given key
// into the provided output structure.
func readSettingsDocInto(st modelBackend, collection, key string, out interface{}) error {
	settings, closer := st.db().GetCollection(collection)
	defer closer()

	err := settings.FindId(key).One(out)
	if err == mgo.ErrNotFound {
		err = errors.NotFoundf("settings")
	}
	return err
}

// ReadSettings returns the settings for the given key.
func (st *State) ReadSettings(collection, key string) (*Settings, error) {
	return readSettings(st, collection, key)
}

// readSettings returns the Settings for key.
func readSettings(backend modelBackend, collection, key string) (*Settings, error) {
	s := newSettings(backend, collection, key)
	if err := s.Read(); err != nil {
		return nil, err
	}
	return s, nil
}

var errSettingsExist = errors.New("cannot overwrite existing settings")

func createSettingsOp(collection, key string, values map[string]interface{}) txn.Op {
	newValues := copyMap(values, escapeReplacer.Replace)
	return txn.Op{
		C:      collection,
		Id:     key,
		Assert: txn.DocMissing,
		Insert: &settingsDoc{
			Settings: newValues,
		},
	}
}

// createSettings writes an initial config node.
func createSettings(backend modelBackend, collection, key string, values map[string]interface{}) (*Settings, error) {
	s := newSettings(backend, collection, key)
	s.core = copyMap(values, nil)
	ops := []txn.Op{createSettingsOp(collection, key, values)}
	err := s.db.RunTransaction(ops)
	if err == txn.ErrAborted {
		return nil, errSettingsExist
	}
	if err != nil {
		return nil, fmt.Errorf("cannot create settings: %v", err)
	}
	return s, nil
}

// removeSettings removes the Settings for key.
func removeSettings(backend modelBackend, collection, key string) error {
	err := backend.db().RunTransaction([]txn.Op{removeSettingsOp(collection, key)})
	if err == txn.ErrAborted {
		return errors.NotFoundf("settings")
	} else if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func removeSettingsOp(collection, key string) txn.Op {
	return txn.Op{
		C:      collection,
		Id:     key,
		Assert: txn.DocExists,
		Remove: true,
	}
}

// listSettings returns all the settings with the specified key prefix.
func listSettings(backend modelBackend, collection, keyPrefix string) (map[string]map[string]interface{}, error) {
	settings, closer := backend.db().GetRawCollection(collection)
	defer closer()

	var matchingSettings []settingsDoc
	findExpr := fmt.Sprintf("^%s.*$", backend.docID(keyPrefix))
	if err := settings.Find(bson.D{{"_id", bson.D{{"$regex", findExpr}}}}).All(&matchingSettings); err != nil {
		return nil, err
	}
	result := make(map[string]map[string]interface{})
	for i := range matchingSettings {
		result[backend.localID(matchingSettings[i].DocID)] = matchingSettings[i].Settings
	}
	return result, nil
}

// replaceSettingsOp returns a txn.Op that deletes the document's contents and
// replaces it with the supplied values, and a function that should be called on
// txn failure to determine whether this operation failed (due to a concurrent
// settings change).
func replaceSettingsOp(backend modelBackend, collection, key string, values map[string]interface{}) (txn.Op, func() (bool, error), error) {
	s, err := readSettings(backend, collection, key)
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
	op.Update = setUnsetUpdateSettings(bson.M(newValues), deletes)
	assertFailed := func() (bool, error) {
		latest, err := readSettings(backend, collection, key)
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

func inSubdocReplacer(subdoc string) func(string) string {
	return func(key string) string {
		return subdoc + "." + key
	}
}

// setUnsetUpdateSettings returns a bson.D for use
// in a s.collection txn.Op's Update field, containing
// $set and $unset operators if the corresponding
// operands are non-empty.
func setUnsetUpdateSettings(set, unset bson.M) bson.D {
	var update bson.D
	replace := inSubdocReplacer("settings")
	if len(set) > 0 {
		set = bson.M(copyMap(map[string]interface{}(set), replace))
		update = append(update, bson.DocElem{"$set", set})
	}
	if len(unset) > 0 {
		unset = bson.M(copyMap(map[string]interface{}(unset), replace))
		update = append(update, bson.DocElem{"$unset", unset})
	}
	if len(update) > 0 {
		update = append(update, bson.DocElem{"$inc", bson.D{{"version", 1}}})
	}
	return update
}

// StateSettings is used to expose various settings APIs outside of the state package.
type StateSettings struct {
	backend    modelBackend
	collection string
}

// NewStateSettings creates a StateSettings from a modelBackend (e.g. State).
func NewStateSettings(backend *State) *StateSettings {
	return &StateSettings{backend, settingsC}
}

// CreateSettings exposes createSettings on state for use outside the state package.
func (s *StateSettings) CreateSettings(key string, settings map[string]interface{}) error {
	_, err := createSettings(s.backend, s.collection, key, settings)
	return err
}

// ReadSettings exposes readSettings on state for use outside the state package.
func (s *StateSettings) ReadSettings(key string) (map[string]interface{}, error) {
	if settings, err := readSettings(s.backend, s.collection, key); err != nil {
		return nil, err
	} else {
		return settings.Map(), nil
	}
}

// RemoveSettings exposes removeSettings on state for use outside the state package.
func (s *StateSettings) RemoveSettings(key string) error {
	return removeSettings(s.backend, s.collection, key)
}

// ListSettings exposes listSettings on state for use outside the state package.
func (s *StateSettings) ListSettings(keyPrefix string) (map[string]map[string]interface{}, error) {
	return listSettings(s.backend, s.collection, keyPrefix)
}
