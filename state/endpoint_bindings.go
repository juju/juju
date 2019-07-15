// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/mongo/utils"
)

// defaultEndpointName is the key in the bindings map that stores the
// space name that endpoints should be bound to if they aren't found
// individually.
const defaultEndpointName = ""

// endpointBindingsDoc represents how a application's endpoints are bound to spaces.
// The DocID field contains the applications's global key, so there is always one
// endpointBindingsDoc per application.
type endpointBindingsDoc struct {
	// DocID is always the same as a application's global key.
	DocID string `bson:"_id"`

	// Bindings maps an application endpoint name to the space name it is bound to.
	Bindings bindingsMap `bson:"bindings"`

	// TxnRevno is used to assert the collection have not changed since this
	// document was fetched.
	TxnRevno int64 `bson:"txn-revno"`
}

// bindingsMap is the underlying type stored in mongo for bindings.
type bindingsMap map[string]string

// SetBSON ensures any special characters ($ or .) are unescaped in keys after
// unmarshalling the raw BSON coming from the stored document.
func (b *bindingsMap) SetBSON(raw bson.Raw) error {
	rawMap := make(map[string]string)
	if err := raw.Unmarshal(rawMap); err != nil {
		return err
	}
	for key, value := range rawMap {
		newKey := utils.UnescapeKey(key)
		if newKey != key {
			delete(rawMap, key)
		}
		rawMap[newKey] = value
	}
	*b = bindingsMap(rawMap)
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
		newKey := utils.EscapeKey(key)
		rawMap[newKey] = value
	}

	return rawMap, nil
}

// mergeBindings returns the effective bindings, by combining the default
// bindings based on the given charm metadata, overriding them first with
// matching oldMap values, and then with newMap values (for the same keys).
// newMap and oldMap are both optional and will ignored when empty. Returns a
// map containing the combined finalized bindings.
// Returns true/false if there are any actual differences.
func mergeBindings(newMap, oldMap map[string]string, meta *charm.Meta) (map[string]string, bool, error) {
	defaultsMap := DefaultEndpointBindingsForCharm(meta)
	defaultBinding := oldMap[defaultEndpointName]
	if newDefaultBinding, ok := newMap[defaultEndpointName]; ok {
		// new default binding supersedes the old default binding
		defaultBinding = newDefaultBinding
	}

	// defaultsMap contains all endpoints that must be bound for the given charm
	// metadata, but we need to figure out which value to use for each key.
	updated := make(map[string]string)
	for key, defaultValue := range defaultsMap {
		effectiveValue := defaultValue

		oldValue, hasOld := oldMap[key]
		if hasOld {
			if oldValue != effectiveValue {
				effectiveValue = oldValue
			}
		} else {
			// Old didn't talk about this value, but maybe we have a default
			effectiveValue = defaultBinding
		}

		newValue, hasNew := newMap[key]
		if hasNew && newValue != effectiveValue {
			effectiveValue = newValue
		}

		updated[key] = effectiveValue
	}
	if defaultBinding != network.DefaultSpaceName {
		updated[defaultEndpointName] = defaultBinding
	}

	// Any other bindings in newMap are most likely extraneous, but add them
	// anyway and let the validation handle them.
	for key, newValue := range newMap {
		if _, defaultExists := defaultsMap[key]; !defaultExists {
			updated[key] = newValue
		}
	}
	isModified := false
	if len(updated) != len(oldMap) {
		isModified = true
	} else {
		// If the len() is identical, then we know as long as we iterate all entries, then there is no way to
		// miss an entry. Either they have identical keys and we check all the values, or there is an identical
		// number of new keys and missing keys and we'll notice a missing key.
		for key, val := range updated {
			if oldVal, existed := oldMap[key]; !existed || oldVal != val {
				isModified = true
				break
			}
		}
	}
	logger.Debugf("merged endpoint bindings modified: %t, default: %v, old: %v, new: %v, result: %v",
		isModified, defaultsMap, oldMap, newMap, updated)
	return updated, isModified, nil
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
	updatedMap, isModified, err := mergeBindings(givenMap, existingMap, newMeta)
	if err != nil {
		return txn.Op{}, errors.Trace(err)
	}

	// Validate the bindings before updating.
	if err := validateEndpointBindingsForCharm(st, updatedMap, newMeta); err != nil {
		return txn.Op{}, errors.Trace(err)
	}

	if !isModified {
		return txn.Op{}, jujutxn.ErrNoOperations
	}

	// Prepare the update operations.
	escaped := make(bson.M, len(updatedMap))
	for endpoint, space := range updatedMap {
		escaped[utils.EscapeKey(endpoint)] = space
	}

	updateOp := txn.Op{
		C:      endpointBindingsC,
		Id:     key,
		Update: bson.M{"$set": bson.M{"bindings": escaped}},
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
// application global key, or an error satisfying errors.IsNotFound() otherwise.
func readEndpointBindings(st *State, key string) (map[string]string, int64, error) {
	endpointBindings, closer := st.db().GetCollection(endpointBindingsC)
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

	allBindings := DefaultEndpointBindingsForCharm(charmMeta)
	endpointsNamesSet := set.NewStrings()
	for name := range allBindings {
		endpointsNamesSet.Add(name)
	}

	// Ensure there are no unknown endpoints and/or spaces specified.
	//
	// TODO(dimitern): This assumes spaces cannot be deleted when they are used
	// in bindings. In follow-up, this will be enforced by using refcounts on
	// spaces.
	for endpoint, space := range bindings {
		if endpoint != defaultEndpointName && !endpointsNamesSet.Contains(endpoint) {
			return errors.NotValidf("unknown endpoint %q", endpoint)
		}
		if space != network.DefaultSpaceName && !spacesNamesSet.Contains(space) {
			return errors.NotValidf("unknown space %q", space)
		}
	}
	return nil
}

// DefaultEndpointBindingsForCharm populates a bindings map containing each
// endpoint of the given charm metadata (relation name or extra-binding name)
// bound to an empty space.
func DefaultEndpointBindingsForCharm(charmMeta *charm.Meta) map[string]string {
	allRelations := charmMeta.CombinedRelations()
	bindings := make(map[string]string, len(allRelations)+len(charmMeta.ExtraBindings))
	for name := range allRelations {
		bindings[name] = network.DefaultSpaceName
	}
	for name := range charmMeta.ExtraBindings {
		bindings[name] = network.DefaultSpaceName
	}
	return bindings
}
