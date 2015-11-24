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

// endpointBindingsForCharmOp returns the op needed to create new or update
// existing endpoint bindings for the specified charm metadata. If givenMap is
// not empty, any specified bindings there will be merged with the existing
// bindings or created (if missing).
func endpointBindingsForCharmOp(st *State, key string, givenMap map[string]string, meta *charm.Meta) (txn.Op, error) {
	if st == nil {
		return txn.Op{}, errors.Errorf("nil state")
	}
	if meta == nil {
		return txn.Op{}, errors.Errorf("nil charm metadata")
	}

	// Prepare the effective new bindings map.
	defaults, err := defaultEndpointBindingsForCharm(meta)
	if err != nil {
		return txn.Op{}, errors.Trace(err)
	}
	newMap := make(map[string]string)
	// Apply defaults first.
	for key, defaultValue := range defaults {
		newMap[key] = defaultValue
	}
	// Now override with any given.
	for key, givenValue := range givenMap {
		newMap[key] = givenValue
	}
	// Validate the bindings before updating.
	if err := validateEndpointBindingsForCharm(st, newMap, meta); err != nil {
		return txn.Op{}, errors.Trace(err)
	}

	return replaceEndpointBindingsOp(st, key, newMap)
}

// replaceEndpointBindingsOp returns an op that merges the existing bindings
// with newBindings, setting changed or new endpoints or unsetting endpoints
// having an empty value in newBindings. If no bindings exist yet, they will be
// created from newBindings.
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
	for key, oldValue := range existingMap {
		newKey := escapeReplacer.Replace(key)
		newValue, given := newBindings[key]
		if given && newValue != oldValue && newValue != network.DefaultSpace {
			// existing endpoints are already bound to the default space, so
			// only update if needed.
			updates["bindings."+newKey] = newValue
		} else if !given {
			// since endpoints should have been validated against the charm
			// already, missing ones need to go.
			deletes["bindings."+newKey] = 1
		}
	}
	for key, newValue := range newBindings {
		newKey := escapeReplacer.Replace(key)
		oldValue, exists := existingMap[key]
		if exists && newValue != oldValue && newValue != network.DefaultSpace {
			// only update existing if changed.
			updates["bindings."+newKey] = newValue
		} else if !exists {
			// add new endpoints, previously missing.
			updates["bindings."+newKey] = newValue
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
// key.
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
	if st == nil {
		return nil, 0, errors.Errorf("nil state")
	}

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

// validateEndpointBindingsForCharm returns no error all endpoints in the given
// bindings for the given charm metadata are explicitly bound to an existing
// space, otherwise it returns an error satisfying errors.IsNotValid().
func validateEndpointBindingsForCharm(st *State, bindings map[string]string, charmMeta *charm.Meta) error {
	if st == nil {
		return errors.Errorf("nil state")
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
		}
	}
	// Ensure no extra, unknown endpoints and/or spaces are given.
	for endpoint, space := range bindings {
		knownEndpoint := endpointsNamesSet.Contains(endpoint)
		knownSpace := spacesNamesSet.Contains(space)
		switch {
		case !knownEndpoint && !knownSpace && space == "":
			return errors.NotValidf("unknown endpoint %q not bound to a space", endpoint)
		case !knownEndpoint && !knownSpace:
			return errors.NotValidf("unknown endpoint %q bound to unknown space %q", endpoint, space)
		case !knownEndpoint:
			return errors.NotValidf("unknown endpoint %q bound to space %q", endpoint, space)
		case !knownSpace:
			return errors.NotValidf("endpoint %q bound to unknown space %q", endpoint, space)
		default:
			// both endpoint and space are known and valid.
		}
	}
	return nil
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
