// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/txn"
)

// requestedNetworksDoc represents the network restrictions for a
// service or machine. The document ID field is the globalKey of a
// service or a machine.
type requestedNetworksDoc struct {
	Id              string   `bson:"_id"`
	IncludeNetworks []string `bson:"include"`
	ExcludeNetworks []string `bson:"exclude"`
}

func newRequestedNetworksDoc(includeNetworks, excludeNetworks []string) *requestedNetworksDoc {
	return &requestedNetworksDoc{
		IncludeNetworks: includeNetworks,
		ExcludeNetworks: excludeNetworks,
	}
}

func createRequestedNetworksOp(st *State, id string, includeNetworks, excludeNetworks []string) txn.Op {
	return txn.Op{
		C:      st.requestedNetworks.Name,
		Id:     id,
		Assert: txn.DocMissing,
		Insert: newRequestedNetworksDoc(includeNetworks, excludeNetworks),
	}
}

// While networks are immutable, there is no setNetworksOp function.

func removeRequestedNetworksOp(st *State, id string) txn.Op {
	return txn.Op{
		C:      st.requestedNetworks.Name,
		Id:     id,
		Remove: true,
	}
}

func readRequestedNetworks(st *State, id string) (includeNetworks, excludeNetworks []string, err error) {
	doc := requestedNetworksDoc{}
	if err = st.requestedNetworks.FindId(id).One(&doc); err == mgo.ErrNotFound {
		// In 1.17.7+ we always create a requestedNetworksDoc for each
		// service or machine we create, but in legacy databases this
		// is not the case. We ignore the error here for
		// backwards-compatibility.
		err = nil
	} else if err == nil {
		includeNetworks = doc.IncludeNetworks
		excludeNetworks = doc.ExcludeNetworks
	}
	return includeNetworks, excludeNetworks, err
}
