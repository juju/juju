// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/txn"

	"launchpad.net/juju-core/errors"
)

// serviceNetworksDoc represents the network restrictions for a service
type serviceNetworksDoc struct {
	NetworksToInclude *[]string
	NetworksToExclude *[]string
}

func newServiceNetworksDoc(includeNetworks, excludeNetworks []string) serviceNetworksDoc {
	return serviceNetworksDoc{
		NetworksToInclude: &includeNetworks,
		NetworksToExclude: &excludeNetworks,
	}
}

func createServiceNetworksOp(st *State, id string, includeNetworks, excludeNetworks []string) txn.Op {
	return txn.Op{
		C:      st.serviceNetworks.Name,
		Id:     id,
		Assert: txn.DocMissing,
		Insert: newServiceNetworksDoc(includeNetworks, excludeNetworks),
	}
}

// While networks are immutable, there is no setServiceNetworksOp function

func removeServiceNetworksOp(st *State, id string) txn.Op {
	return txn.Op{
		C:      st.serviceNetworks.Name,
		Id:     id,
		Remove: true,
	}
}

func readServiceNetworks(st *State, id string) (includeNetworks, excludeNetworks []string, err error) {
	doc := serviceNetworksDoc{}
	if err = st.serviceNetworks.FindId(id).One(&doc); err == mgo.ErrNotFound {
		err = errors.NotFoundf("service networks")
	} else if err == nil {
		includeNetworks = *doc.NetworksToInclude
		excludeNetworks = *doc.NetworksToExclude
	}
	return includeNetworks, excludeNetworks, err
}
