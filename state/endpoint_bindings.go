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

// SetBSON ensures any special characters ($ or .) are unescaped in keys before
// unmarshalling the raw BSON coming from the stored document.
func (b *bindingsMap) SetBSON(raw bson.Raw) error {
	rawMap := make(map[string]string)
	if err := raw.Unmarshal(rawMap); err != nil {
		return err
	}
	for key, value := range rawMap {
		newKey := unescapeReplacer.Replace(key)
		if newKey != key {
			delete(rawMap, key)
			rawMap[newKey] = value
		}
	}
	*b = bindingsMap(rawMap)
	return nil
}

// GetBSON ensures any special characters ($ or .) are escaped in keys before
// marshalling the map into BSON and storing in mongo.
func (b *bindingsMap) GetBSON() (interface{}, error) {
	if b == nil {
		return nil, nil
	}
	rawMap := make(map[string]string)
	for key, value := range *b {
		rawMap[escapeReplacer.Replace(key)] = value
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

	// TxnRevno is used to assert the contents of the collection hasn't changed
	// since the document was retrieved.
	TxnRevno int64 `bson:"txn-revno"`
}

// createEndpointBindingsOp returns an op inserting the given bindings for the
// given key, asserting it does not exist yet.
func createEndpointBindingsOp(st *State, key string, bindings map[string]string) txn.Op {
	return txn.Op{
		C:      endpointBindingsC,
		Id:     st.docID(key),
		Assert: txn.DocMissing,
		Insert: &endpointBindingsDoc{
			Bindings: bindingsMap(bindings),
		},
	}
}

// removeEndpointBindingsOp returns an op removing the bindings for the given
// key, asserting they exist.
func removeEndpointBindingsOp(st *State, key string) txn.Op {
	return txn.Op{
		C:      endpointBindingsC,
		Id:     st.docID(key),
		Assert: txn.DocExists,
		Remove: true,
	}
}

// assertEndpointBindingsUnchangedOp returns an op asserting the bindings for
// the given key still have the same txnRevno and are therefore unchanged.
func assertEndpointBindingsUnchangedOp(st *State, key string, txnRevno int64) txn.Op {
	return txn.Op{
		C:      endpointBindingsC,
		Id:     st.docID(key),
		Assert: bson.D{{"txn-revno", txnRevno}},
	}
}

// readEndpointBindings returns the binings map and TxnRevno for the given
// service global key if they exist.
func readEndpointBindings(st *State, key string) (map[string]string, int64, error) {
	endpointBindings, closer := st.getCollection(endpointBindingsC)
	defer closer()

	doc := endpointBindingsDoc{}
	err := endpointBindings.FindId(key).One(&doc)
	switch err {
	case mgo.ErrNotFound:
		return nil, 0, errors.NotFoundf("endpoint bindings for %q", key)
	case nil:
		return doc.Bindings, doc.TxnRevno, nil
	}
	return nil, 0, errors.Annotatef(err, "cannot get endpoint bindings for %q", key)
}

// addDefaultEndpointBindings fills in the default space for all unspecified
// endpoint bindings, based on the given charm metadata. Any invalid bindings
// yield an error satisfying errors.IsNotValid.
func addDefaultEndpointBindings(st *State, givenBindings map[string]string, charmMeta *charm.Meta) error {
	if givenBindings == nil {
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
	endpointsNamesSet := set.NewStrings()

	processRelations := func(relations map[string]charm.Relation) error {
		for name, _ := range relations {
			endpointsNamesSet.Add(name)
			if space, isSet := givenBindings[name]; !isSet || space == "" {
				givenBindings[name] = network.DefaultSpace
			} else if isSet && space != "" && !spacesNamesSet.Contains(space) {
				return errors.NotValidf("endpoint %q bound to unknown space %q", name, space)
			}
		}
		return nil
	}
	if err := processRelations(charmMeta.Provides); err != nil {
		return errors.Trace(err)
	}
	if err := processRelations(charmMeta.Requires); err != nil {
		return errors.Trace(err)
	}
	if err := processRelations(charmMeta.Peers); err != nil {
		return errors.Trace(err)
	}
	// Ensure no extra, unknown endpoints are given.
	for endpoint, space := range givenBindings {
		if !endpointsNamesSet.Contains(endpoint) {
			return errors.NotValidf("unknown endpoint %q binding to space %q", endpoint, space)
		}
		if !spacesNamesSet.Contains(space) {
			return errors.NotValidf("endpoint %q bound to unknown space %q", endpoint, space)
		}
	}
	return nil
}

// updateEndpointBindingsForNewCharmOps returns the ops needed to update the
// existing endpoint bindings for a service when its charm is changed, ensuring
// endpoints not present in the new charm are removed, while new ones are added
// and bound to the default space.
func updateEndpointBindingsForNewCharmOps(st *State, key string, newCharmMeta *charm.Meta) ([]txn.Op, error) {
	existingMap, txnRevno, err := readEndpointBindings(st, key)
	if err != nil {
		return nil, errors.Trace(err)
	}
	newMap := make(map[string]string)
	if err := addDefaultEndpointBindings(st, newMap, newCharmMeta); err != nil {
		return nil, errors.Trace(err)
	}
	// Ensure existing endpoint bindings are preserved.
	for newEndpoint, _ := range newMap {
		if oldSpace, doesExist := existingMap[newEndpoint]; doesExist {
			newMap[newEndpoint] = oldSpace
		}
	}
	// Before changing anything assert the existing endpoints haven't changed
	// yet before replacing them with the new ones.
	return []txn.Op{
		assertEndpointBindingsUnchangedOp(st, key, txnRevno),
		removeEndpointBindingsOp(st, key),
		createEndpointBindingsOp(st, key, newMap),
	}, nil
}
