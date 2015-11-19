// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/utils/set"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/network"
)

// endpointBindingsDoc represents how a service endpoints are bound to spaces.
// The DocID field contains the service's global key, so there is always one
// endpointBindingsDoc per service.
type endpointBindingsDoc struct {
	DocID   string `bson:"_id"`
	EnvUUID string `bson:"env-uuid"`
	// Bindings maps a service endpoint name to the space name it is bound to.
	Bindings map[string]string `bson:"bindings"`
}

func createEndpointBindingsOp(st *State, key string, bindings map[string]string) txn.Op {
	return txn.Op{
		C:      endpointBindingsC,
		Id:     st.docID(key),
		Assert: txn.DocMissing,
		Insert: &endpointBindingsDoc{Bindings: bindings},
	}
}

func removeEndpointBindingsOp(st *State, key string) txn.Op {
	return txn.Op{
		C:      endpointBindingsC,
		Id:     st.docID(key),
		Remove: true,
	}
}

func readEndpointBindings(st *State, key string) (map[string]string, error) {
	endpointBindings, closer := st.getCollection(endpointBindingsC)
	defer closer()

	doc := endpointBindingsDoc{}
	err := endpointBindings.FindId(key).One(&doc)
	switch err {
	case mgo.ErrNotFound:
		return nil, errors.NotFoundf("endpoint bindings for %q", key)
	case nil:
		return doc.Bindings, nil
	}
	return nil, errors.Annotatef(err, "cannot get endpoint bindings for %q", key)
}

// addDefaultEndpointBindings fills in the default space for all unspecified
// endpoint bindings, based on the given charm metadata. Any invalid bindings
// yield an error satisfying errors.IsNotValid.
func addDefaultEndpointBindings(st *State, givenBindings map[string]string, charmMeta *charm.Meta) error {
	if givenBindings == nil {
		return errors.NotValidf("nil bindings")
	}
	spaces, err := st.AllSpaces()
	if err != nil {
		return errors.Trace(err)
	}
	spacesNamesSet := set.NewStrings()
	for _, space := range spaces {
		spacesNamesSet.Add(space.Name())
	}

	processRelations := func(relations map[string]charm.Relation) error {
		for name, _ := range relations {
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
	return nil
}
