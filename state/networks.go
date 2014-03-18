// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"labix.org/v2/mgo"
	"labix.org/v2/mgo/txn"

	"launchpad.net/juju-core/errors"
)

// networksDoc represents the networks a service should be associated with
type networksDoc struct {
	NetworksToInclude *[]string
	NetworksToExclude *[]string
}

func newNetworksDoc(includeNetworks, excludeNetworks []string) networksDoc {
	return networksDoc{
		NetworksToInclude: &includeNetworks,
		NetworksToExclude: &excludeNetworks,
	}
}

func createNetworksOp(st *State, id string, includeNetworks, excludeNetworks []string) txn.Op {
	return txn.Op{
		C:      st.networks.Name,
		Id:     id,
		Assert: txn.DocMissing,
		Insert: newNetworksDoc(includeNetworks, excludeNetworks),
	}
}

func setNetworksOp(st *State, id string, includeNetworks, excludeNetworks []string) txn.Op {
	return txn.Op{
		C:      st.networks.Name,
		Id:     id,
		Assert: txn.DocExists,
		Update: D{{"$set", newNetworksDoc(includeNetworks, excludeNetworks)}},
	}
}

func removeNetworksOp(st *State, id string) txn.Op {
	return txn.Op{
		C:      st.networks.Name,
		Id:     id,
		Remove: true,
	}
}

func readNetworks(st *State, id string) (includeNetworks, excludeNetworks []string, err error) {
	err = nil
	doc := networksDoc{}
	if err = st.networks.FindId(id).One(&doc); err == mgo.ErrNotFound {
		err = errors.NotFoundf("service networks")
	} else if err == nil {
		// XXX: need to explictly copy these slices to be safe?
		includeNetworks = *doc.NetworksToInclude
		excludeNetworks = *doc.NetworksToExclude
	}
	return includeNetworks, excludeNetworks, err
}

func writeNetworks(st *State, id string, includeNetworks, excludeNetworks []string) error {
	ops := []txn.Op{setNetworksOp(st, id, includeNetworks, excludeNetworks)}
	if err := st.runTransaction(ops); err != nil {
		return fmt.Errorf("cannot set service networks: %v", err)
	}
	return nil
}
