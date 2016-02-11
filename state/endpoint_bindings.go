// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"github.com/juju/utils/set"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

// endpointBindingsDoc represents how a service endpoints are bound to spaces.
// The DocID field contains the service's global key, so there is always one
// endpointBindingsDoc per service.
type endpointBindingsDoc struct {
	// DocID is always the same as a service's global key.
	DocID   string `bson:"_id"`
	EnvUUID string `bson:"env-uuid"`

	// Bindings maps a service endpoint name to the space name it is bound to.
	Bindings bindingsMap `bson:"bindings"`

	// TxnRevno is used to assert the collection have not changed since this
	// document was fetched.
	TxnRevno int64 `bson:"txn-revno"`
}

// bindingsMap is the underlying type stored in mongo for bindings.
type bindingsMap map[string]string

// SetBSON ensures any special characters ($ or .) are unescaped in keys after
// unmarshalling the raw BSON coming from the stored document.
func (bp *bindingsMap) SetBSON(raw bson.Raw) error {
	rawMap := make(map[string]string)
	if err := raw.Unmarshal(rawMap); err != nil {
		return err
	}
	for key, value := range rawMap {
		newKey := unescapeReplacer.Replace(key)
		if newKey != key {
			delete(rawMap, key)
		}
		rawMap[newKey] = value
	}
	*bp = bindingsMap(rawMap)
	return nil
}

// GetBSON ensures any special characters ($ or .) are escaped in keys before
// marshalling the map into BSON and storing in mongo.
func (b bindingsMap) GetBSON() (interface{}, error) {
	if b == nil || len(b) == 0 {
		// We need to return a non-nil map otherwise bson.Unmarshal
		// call will fail when reading the doc back.
		return make(map[string]string), nil
	}
	rawMap := make(map[string]string, len(b))
	for key, value := range b {
		newKey := escapeReplacer.Replace(key)
		rawMap[newKey] = value
	}
	return rawMap, nil
}

// mergeBindings returns the effective bindings, by combining the default
// bindings based on the given charm metadata, overriding them first with
// matching oldMap values, and then with newMap values (for the same keys).
// newMap and oldMap are both optional and will ignored when empty. Returns a
// map containing only those bindings that need updating, and a sorted slice of
// keys to remove (if any) - those are present in oldMap but missing in both
// newMap and defaults.
func mergeBindings(newMap, oldMap map[string]string, meta *charm.Meta) (map[string]string, []string, error) {

	defaultsMap, err := defaultEndpointBindingsForCharm(meta)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	// defaultsMap contains all endpoints that must be bound for the given charm
	// metadata, but we need to figure out which value to use for each key.
	updated := make(map[string]string)
	for key, defaultValue := range defaultsMap {
		effectiveValue := defaultValue

		oldValue, hasOld := oldMap[key]
		if hasOld && oldValue != effectiveValue {
			effectiveValue = oldValue
		}

		newValue, hasNew := newMap[key]
		if hasNew && newValue != effectiveValue {
			effectiveValue = newValue
		}

		updated[key] = effectiveValue
	}

	// Any extra bindings in newMap are most likely extraneous, but add them
	// anyway and let the validation handle them.
	for key, newValue := range newMap {
		if _, defaultExists := defaultsMap[key]; !defaultExists {
			updated[key] = newValue
		}
	}

	// All defaults were processed, so anything else in oldMap not about to be
	// updated and not having a default for the given metadata needs to be
	// removed.
	removedKeys := set.NewStrings()
	for key := range oldMap {
		if _, updating := updated[key]; !updating {
			removedKeys.Add(key)
		}
		if _, defaultExists := defaultsMap[key]; !defaultExists {
			removedKeys.Add(key)
		}
	}
	removed := removedKeys.SortedValues()
	return updated, removed, nil
}

// createEndpointBindingsOp returns the op needed to create new endpoint
// bindings using the optional givenMap and the specified charm metadata to for
// determining defaults and to validate the effective bindings.
func createEndpointBindingsOp(st *State, key string, givenMap map[string]string, meta *charm.Meta) (txn.Op, error) {

	// No existing map to merge, just use the defaults.
	initialMap, _, err := mergeBindings(givenMap, nil, meta)
	if err != nil {
		return txn.Op{}, errors.Trace(err)
	}

	// Validate the bindings before inserting.
	if err := validateEndpointBindingsForCharm(st, initialMap, meta); err != nil {
		return txn.Op{}, errors.Trace(err)
	}

	return txn.Op{
		C:      endpointBindingsC,
		Id:     key,
		Assert: txn.DocMissing,
		Insert: endpointBindingsDoc{
			Bindings: initialMap,
		},
	}, nil
}

// updateEndpointBindingsOp returns an op that merges the existing bindings with
// givenMap, using newMeta to validate the merged bindings, and asserting the
// existing ones haven't changed in the since we fetched them.
func updateEndpointBindingsOp(st *State, key string, givenMap map[string]string, newMeta *charm.Meta) (txn.Op, error) {
	// Fetch existing bindings.
	existingMap, txnRevno, err := readEndpointBindings(st, key)
	if err != nil && !errors.IsNotFound(err) {
		return txn.Op{}, errors.Trace(err)
	}

	// Merge existing with given as needed.
	updatedMap, removedKeys, err := mergeBindings(givenMap, existingMap, newMeta)
	if err != nil {
		return txn.Op{}, errors.Trace(err)
	}

	// Validate the bindings before updating.
	if err := validateEndpointBindingsForCharm(st, updatedMap, newMeta); err != nil {
		return txn.Op{}, errors.Trace(err)
	}

	// Prepare the update operations.
	sanitize := inSubdocEscapeReplacer("bindings")
	changes := make(bson.M, len(updatedMap))
	for endpoint, space := range updatedMap {
		changes[sanitize(endpoint)] = space
	}
	deletes := make(bson.M, len(removedKeys))
	for _, endpoint := range removedKeys {
		deletes[sanitize(endpoint)] = 1
	}

	var update bson.D
	if len(changes) != 0 {
		update = append(update, bson.DocElem{Name: "$set", Value: changes})
	}
	if len(deletes) != 0 {
		update = append(update, bson.DocElem{Name: "$unset", Value: deletes})
	}
	if len(update) == 0 {
		return txn.Op{}, jujutxn.ErrNoOperations
	}
	updateOp := txn.Op{
		C:      endpointBindingsC,
		Id:     key,
		Update: update,
	}
	if existingMap != nil {
		// Only assert existing haven't changed when they actually exist.
		updateOp.Assert = bson.D{{"txn-revno", txnRevno}}
	}
	return updateOp, nil
}

// removeEndpointBindingsOp returns an op removing the bindings for the given
// key, without asserting they exist in the first place.
func removeEndpointBindingsOp(key string) txn.Op {
	return txn.Op{
		C:      endpointBindingsC,
		Id:     key,
		Remove: true,
	}
}

// readEndpointBindings returns the stored bindings and TxnRevno for the given
// service global key, or an error satisfying errors.IsNotFound() otherwise.
func readEndpointBindings(st *State, key string) (map[string]string, int64, error) {
	endpointBindings, closer := st.getCollection(endpointBindingsC)
	defer closer()

	var doc endpointBindingsDoc
	err := endpointBindings.FindId(key).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, 0, errors.NotFoundf("endpoint bindings for %q", key)
	}
	if err != nil {
		return nil, 0, errors.Annotatef(err, "cannot get endpoint bindings for %q", key)
	}

	return doc.Bindings, doc.TxnRevno, nil
}

// validateEndpointBindingsForCharm verifies that all endpoint names in bindings
// are valid for the given charm metadata, and each endpoint is bound to a known
// space - otherwise an error satisfying errors.IsNotValid() will be returned.
func validateEndpointBindingsForCharm(st *State, bindings map[string]string, charmMeta *charm.Meta) error {
	if st == nil {
		return errors.NotValidf("nil state")
	}
	if bindings == nil {
		return errors.NotValidf("nil bindings")
	}
	if charmMeta == nil {
		return errors.NotValidf("nil charm metadata")
	}
	spaces, err := st.AllSpaces()
	if err != nil {
		return errors.Trace(err)
	}

	spacesNamesSet := set.NewStrings()
	for _, space := range spaces {
		spacesNamesSet.Add(space.Name())
	}

	allRelations, err := CombinedCharmRelations(charmMeta)
	if err != nil {
		return errors.Trace(err)
	}
	endpointsNamesSet := set.NewStrings()
	for name := range allRelations {
		endpointsNamesSet.Add(name)
	}

	// Ensure there are no unknown endpoints and/or spaces specified.
	//
	// TODO(dimitern): This assumes spaces cannot be deleted when they are used
	// in bindings. In follow-up, this will be enforced by using refcounts on
	// spaces.
	for endpoint, space := range bindings {
		if !endpointsNamesSet.Contains(endpoint) {
			return errors.NotValidf("unknown endpoint %q", endpoint)
		}
		if space != "" && !spacesNamesSet.Contains(space) {
			return errors.NotValidf("unknown space %q", space)
		}
	}
	return nil
}

// defaultEndpointBindingsForCharm populates a bindings map containing each
// endpoint of the given charm metadata bound to an empty space.
func defaultEndpointBindingsForCharm(charmMeta *charm.Meta) (map[string]string, error) {
	allRelations, err := CombinedCharmRelations(charmMeta)
	if err != nil {
		return nil, errors.Trace(err)
	}
	bindings := make(map[string]string, len(allRelations))
	for name := range allRelations {
		bindings[name] = ""
	}
	return bindings, nil
}

// CombinedCharmRelations returns the relations defined in the given charm
// metadata (from Provides, Requires, and Peers) in a single map. This works
// because charm relation names must be unique regarless of their kind.
//
// TODO(dimitern): 2015-11-27 bug http://pad.lv/1520623
// This should be moved directly into the charm repo, as it's
// generally useful.
func CombinedCharmRelations(charmMeta *charm.Meta) (map[string]charm.Relation, error) {
	if charmMeta == nil {
		return nil, errors.Errorf("nil charm metadata")
	}
	combined := make(map[string]charm.Relation)
	for name, relation := range charmMeta.Provides {
		combined[name] = relation
	}
	for name, relation := range charmMeta.Requires {
		combined[name] = relation
	}
	for name, relation := range charmMeta.Peers {
		combined[name] = relation
	}
	return combined, nil
}
