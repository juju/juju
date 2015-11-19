// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/txn"
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
		Insert: &endpointBindingsDoc{
			Bindings: bindings,
		},
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
