// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/txn"
)

// requestedNetworksDoc represents the network restrictions for a
// service or machine. The document ID field is the globalKey of a
// service or a machine.
type requestedNetworksDoc struct {
	DocID    string   `bson:"_id"`
	Id       string   `bson:"requestednetworkid"`
	EnvUUID  string   `bson:"env-uuid"`
	Networks []string `bson:"networks"`
}

func (st *State) newRequestedNetworksDoc(id string, networks []string) *requestedNetworksDoc {
	return &requestedNetworksDoc{
		DocID:    st.docID(id),
		EnvUUID:  st.EnvironUUID(),
		Networks: networks,
	}
}

func createRequestedNetworksOp(st *State, id string, networks []string) txn.Op {
	doc := st.newRequestedNetworksDoc(id, networks)
	return txn.Op{
		C:      requestedNetworksC,
		Id:     doc.DocID,
		Assert: txn.DocMissing,
		Insert: doc,
	}
}

// While networks are immutable, there is no setNetworksOp function.

func removeRequestedNetworksOp(st *State, id string) txn.Op {
	return txn.Op{
		C:      requestedNetworksC,
		Id:     st.docID(id),
		Remove: true,
	}
}

func readRequestedNetworks(st *State, id string) ([]string, error) {
	requestedNetworks, closer := st.getCollection(requestedNetworksC)
	defer closer()

	doc := requestedNetworksDoc{}
	err := requestedNetworks.FindId(id).One(&doc)
	if err == mgo.ErrNotFound {
		// In 1.17.7+ we always create a requestedNetworksDoc for each
		// service or machine we create, but in legacy databases this
		// is not the case. We ignore the error here for
		// backwards-compatibility.
		return nil, nil
	}
	return doc.Networks, err
}
