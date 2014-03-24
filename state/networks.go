// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/txn"

	"launchpad.net/juju-core/errors"
)

// networksDoc represents the network restrictions for a service or machine.
// The document ID field is the globalKey of a service or a machine.
type networksDoc struct {
	NetworksToInclude []string `bson:"included"`
	NetworksToExclude []string `bson:"excluded"`
}

func newNetworksDoc(includedNetworks, excludedNetworks []string) *networksDoc {
	return &networksDoc{
		NetworksToInclude: includedNetworks,
		NetworksToExclude: excludedNetworks,
	}
}

func createNetworksOp(st *State, id string, includedNetworks, excludedNetworks []string) txn.Op {
	return txn.Op{
		C:      st.networks.Name,
		Id:     id,
		Assert: txn.DocMissing,
		Insert: newNetworksDoc(includedNetworks, excludedNetworks),
	}
}

// While networks are immutable, there is no setNetworksOp function.

func removeNetworksOp(st *State, id string) txn.Op {
	return txn.Op{
		C:      st.networks.Name,
		Id:     id,
		Remove: true,
	}
}

func readNetworks(st *State, id string) (includedNetworks, excludedNetworks []string, err error) {
	doc := networksDoc{}
	if err = st.networks.FindId(id).One(&doc); err == mgo.ErrNotFound {
		err = errors.NotFoundf("linked networks")
	} else if err == nil {
		includedNetworks = doc.NetworksToInclude
		excludedNetworks = doc.NetworksToExclude
	}
	return includedNetworks, excludedNetworks, err
}
