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
	core map[string]interface{}

	// version is the version corresponding to "disk"; i.e.
	// the value of the version field in the status document
	// when it was read.
	version int64
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
		Id:     c.key,
		Assert: txn.DocExists,
		Update: setUnsetUpdateSettings(updates, deletions),
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

func newSettingsWithDoc(st *State, key string, doc *settingsDoc) *Settings {
	return &Settings{
		st:      st,
		key:     key,
		version: doc.Version,
		disk:    doc.Settings,
		core:    copyMap(doc.Settings, nil),
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
func (c *Settings) Read() error {
	doc, err := readSettingsDoc(c.st, c.key)
	if errors.IsNotFound(err) {
		c.disk = nil
		c.core = make(map[string]interface{})
		return err
	}
	if err != nil {
		return errors.Annotate(err, "cannot read settings")
	}
	c.version = doc.Version
	c.disk = doc.Settings
	c.core = copyMap(c.disk, nil)
	return nil
}

// readSettingsDoc reads the settings doc with the given key.
func readSettingsDoc(st *State, key string) (*settingsDoc, error) {
	var doc settingsDoc
	if err := readSettingsDocInto(st, key, &doc); err != nil {
		return nil, errors.Trace(err)
	}
	return &doc, nil
}

// readSettingsDocInto reads the settings doc with the given key
// into the provided output structure.
func readSettingsDocInto(st *State, key string, out interface{}) error {
	settings, closer := st.getRawCollection(settingsC)
	defer closer()

	err := settings.FindId(st.docID(key)).One(out)

	// This is required to allow loading of environ settings before the
	// model UUID migration has been applied to the settings collection.
	// Without this, an agent's version cannot be read, blocking the upgrade.
	if err == mgo.ErrNotFound && key == modelGlobalKey {
		err = settings.FindId(modelGlobalKey).One(out)
	}
	if err == mgo.ErrNotFound {
		err = errors.NotFoundf("settings")
	}
	return err
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

func createSettingsOp(key string, values map[string]interface{}) txn.Op {
	newValues := copyMap(values, escapeReplacer.Replace)
	return txn.Op{
		C:      settingsC,
		Id:     key,
		Assert: txn.DocMissing,
		Insert: &settingsDoc{
			Settings: newValues,
		},
	}
}

// createSettings writes an initial config node.
func createSettings(st *State, key string, values map[string]interface{}) (*Settings, error) {
	s := newSettings(st, key)
	s.core = copyMap(values, nil)
	ops := []txn.Op{createSettingsOp(key, values)}
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
	err := st.runTransaction([]txn.Op{removeSettingsOp(key)})
	if err == txn.ErrAborted {
		return errors.NotFoundf("settings")
	} else if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func removeSettingsOp(key string) txn.Op {
	return txn.Op{
		C:      settingsC,
		Id:     key,
		Assert: txn.DocExists,
		Remove: true,
	}
}

// listSettings returns all the settings with the specified key prefix.
func listSettings(st *State, keyPrefix string) (map[string]map[string]interface{}, error) {
	settings, closer := st.getRawCollection(settingsC)
	defer closer()

	var matchingSettings []settingsDoc
	findExpr := fmt.Sprintf("^%s.*$", st.docID(keyPrefix))
	if err := settings.Find(bson.D{{"_id", bson.D{{"$regex", findExpr}}}}).All(&matchingSettings); err != nil {
		return nil, err
	}
	result := make(map[string]map[string]interface{})
	for i := range matchingSettings {
		result[st.localID(matchingSettings[i].DocID)] = matchingSettings[i].Settings
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
	op.Update = setUnsetUpdateSettings(bson.M(newValues), deletes)
	assertFailed := func() (bool, error) {
		latest, err := readSettings(st, key)
		if err != nil {
			return false, err
		}
		return latest.version != s.version, nil
	}
	return op, assertFailed, nil
}

func (s *Settings) assertUnchangedOp() txn.Op {
	return txn.Op{
		C:      settingsC,
		Id:     s.key,
		Assert: bson.D{{"version", s.version}},
	}
}

func inSubdocReplacer(subdoc string) func(string) string {
	return func(key string) string {
		return subdoc + "." + key
	}
}

func inSubdocEscapeReplacer(subdoc string) func(string) string {
	return func(key string) string {
		return subdoc + "." + escapeReplacer.Replace(key)
	}
}

// setUnsetUpdateSettings returns a bson.D for use
// in a settingsC txn.Op's Update field, containing
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
