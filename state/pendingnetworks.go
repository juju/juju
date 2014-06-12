// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"labix.org/v2/mgo/txn"

	"github.com/juju/juju/network"
)

// pendingNetworkDoc represents a network, available in the
// environment (i.e. known to the provider), but not yet used in juju
// for deployments. It needs to be "promoted" to an actual network
// before that.
type pendingNetworkDoc struct {
	ProviderId network.Id `bson:"_id"`
	CIDR       string
	VLANTag    int
}

func newPendingNetworkDoc(providerId network.Id, cidr string, vlanTag int) *pendingNetworkDoc {
	return &pendingNetworkDoc{ProviderId: providerId, CIDR: cidr, VLANTag: vlanTag}
}

func createPendingNetworkOp(st *State, providerId network.Id, cidr string, vlanTag int) txn.Op {
	return txn.Op{
		C:      st.pendingNetworks.Name,
		Id:     providerId,
		Assert: txn.DocMissing,
		Insert: newPendingNetworkDoc(providerId, cidr, vlanTag),
	}
}

func removePendingNetworkOp(st *State, providerId network.Id) txn.Op {
	return txn.Op{
		C:      st.pendingNetworks.Name,
		Id:     providerId,
		Remove: true,
	}
}

func getPendingNetwork(st *State, providerId network.Id) (pendingNetworkDoc, error) {
	var doc pendingNetworkDoc
	err := st.pendingNetworks.FindId(providerId).One(&doc)
	return doc, err
}
