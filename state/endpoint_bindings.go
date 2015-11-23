// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/utils/set"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/network"
)

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
func (bp *bindingsMap) GetBSON() (interface{}, error) {
	if bp == nil || len(*bp) == 0 {
		return nil, nil
	}
	rawMap := make(map[string]string, len(*bp))
	for key, value := range *bp {
		newKey := escapeReplacer.Replace(key)
		if newKey != key {
			delete(rawMap, key)
		}
		rawMap[newKey] = value
	}
	return rawMap, nil
}

// endpointBindingsDoc represents how a service endpoints are bound to spaces.
// The DocID field contains the service's global key, so there is always one
// endpointBindingsDoc per service.
type endpointBindingsDoc struct {
	// DocID is always the same as a service's global key.
	DocID   string `bson:"_id"`
	EnvUUID string `bson:"env-uuid"`

	// Bindings maps a service endpoint name to the space name it is bound to.
	Bindings bindingsMap `bson:"bindings"`
}

// replaceEndpointBindingsOp returns an op that sets and/or unsets existing
// endpoint bindings as needed, so the effective stored bindings match
// newBindings for the given key. If no bindings exist yet, they will be created
// from newBindings.
func replaceEndpointBindingsOp(st *State, key string, newBindings map[string]string) (txn.Op, error) {
	existingMap, txnRevno, err := readEndpointBindings(st, key)
	if errors.IsNotFound(err) {
		// No bindings yet, just create them.
		newMap := make(bindingsMap)
		for key, value := range newBindings {
			newMap[key] = value
		}
		return txn.Op{
			C:      endpointBindingsC,
			Id:     key,
			Assert: txn.DocMissing,
			Insert: &endpointBindingsDoc{
				Bindings: newMap,
			},
		}, nil
	}
	if err != nil {
		return txn.Op{}, errors.Trace(err)
	}
	updates := make(bson.M)
	deletes := make(bson.M)
	for key, value := range existingMap {
		newValue, found := newBindings[key]
		if !found {
			deletes[key] = 1
		} else if newValue != value {
			updates[key] = newValue
		}
	}
	op := txn.Op{
		C:      endpointBindingsC,
		Id:     key,
		Assert: bson.D{{"txn-revno", txnRevno}},
	}
	var update bson.D
	if len(updates) > 0 {
		update = append(update, bson.DocElem{"$set", updates})
	}
	if len(deletes) > 0 {
		update = append(update, bson.DocElem{"$unset", deletes})
	}
	if len(update) > 0 {
		op.Update = update
	}
	return op, nil
}

// removeEndpointBindingsOp returns an op removing the bindings for the given
// key, asserting the document exists.
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
	txnRevno, err := getTxnRevno(endpointBindings, doc.DocID)
	if err != nil {
		return nil, 0, errors.Trace(err)
	}

	return doc.Bindings, txnRevno, nil
}

// defaultEndpointBindingsForCharm populates a bindings map containing each
// endpoint of the given charm metadata bound to the default space.
func defaultEndpointBindingsForCharm(charmMeta *charm.Meta) (map[string]string, error) {
	if charmMeta == nil {
		return nil, errors.Errorf("nil charm metadata")
	}
	bindings := make(map[string]string)
	for name := range combinedCharmRelations(charmMeta) {
		bindings[name] = network.DefaultSpace
	}
	return bindings, nil
}

// validateEndpointBindingsForCharm returns no error all endpoints in the given
// bindings for the given charm metadata are explicitly bound to an existing
// space, otherwise it returns an error satisfying errors.IsNotValid().
func validateEndpointBindingsForCharm(st *State, bindings map[string]string, charmMeta *charm.Meta) error {
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
	// TODO(dimitern): Do not treat the default space specially here, this is
	// temporary only to reduce the fallout across state tests and will be fixed
	// in a follow-up.
	if !spacesNamesSet.Contains(network.DefaultSpace) {
		spacesNamesSet.Add(network.DefaultSpace)
	}
	endpointsNamesSet := set.NewStrings()

	for name := range combinedCharmRelations(charmMeta) {
		endpointsNamesSet.Add(name)
		if space, isSet := bindings[name]; !isSet || space == "" {
			return errors.NotValidf("endpoint %q not bound to a space", name)
		} else if isSet && space != "" && !spacesNamesSet.Contains(space) {
			return errors.NotValidf("endpoint %q bound to unknown space %q", name, space)
		}
	}
	// Ensure no extra, unknown endpoints are given.
	for endpoint, space := range bindings {
		if !endpointsNamesSet.Contains(endpoint) {
			return errors.NotValidf("unknown endpoint %q binding to space %q", endpoint, space)
		}
		if !spacesNamesSet.Contains(space) {
			return errors.NotValidf("endpoint %q bound to unknown space %q", endpoint, space)
		}
	}
	return nil
}

// endpointBindingsForCharmOp returns the op needed to create new or update
// existing endpoint bindings for the specified charm metadata. If givenMap is
// not empty, any specified bindings there will override the defaults.
func endpointBindingsForCharmOp(st *State, key string, givenMap map[string]string, meta *charm.Meta) (txn.Op, error) {
	if meta == nil {
		return txn.Op{}, errors.Errorf("nil charm metadata")
	}

	// Prepare the effective new bindings map.
	defaults, err := defaultEndpointBindingsForCharm(meta)
	if err != nil {
		return txn.Op{}, errors.Trace(err)
	}
	newMap := make(map[string]string)
	if len(givenMap) > 0 {
		// Combine the given bindings with defaults.
		for key, defaultValue := range defaults {
			if givenValue, found := givenMap[key]; !found {
				newMap[key] = defaultValue
			} else {
				newMap[key] = givenValue
			}
		}
	} else {
		// No bindings given, just use the defaults.
		newMap = defaults
	}
	// Validate the bindings before updating.
	if err := validateEndpointBindingsForCharm(st, newMap, meta); err != nil {
		return txn.Op{}, errors.Trace(err)
	}

	return replaceEndpointBindingsOp(st, key, newMap)
}

// combinedCharmRelations returns the relations defined in the given charm
// metadata (from Provides, Requires, and Peers) in a single map. This works
// because charm relation names must be unique regarless of their kind. This
// helper is only used internally, and it will panic if charmMeta is nil.
func combinedCharmRelations(charmMeta *charm.Meta) map[string]charm.Relation {
	if charmMeta == nil {
		panic("nil charm metadata")
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
	return combined
}
