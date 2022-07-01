// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/mgorootkeystore"

	"github.com/juju/juju/v3/mongo"
	"github.com/juju/juju/v3/state/bakerystorage"
)

// NewBakeryStorage returns a new bakery.Storage. By default, items
// added to the store are retained until deleted explicitly. The
// store's ExpireAfter method can be called to derive a new store that
// will expire items at the specified time.
func (st *State) NewBakeryStorage() (bakerystorage.ExpirableStorage, error) {
	return bakerystorage.New(bakerystorage.Config{
		GetCollection: func() (mongo.Collection, func()) {
			return st.db().GetCollection(bakeryStorageItemsC)
		},
		GetStorage: func(rootKeys *mgorootkeystore.RootKeys, coll mongo.Collection, expireAfter time.Duration) bakery.RootKeyStore {
			return rootKeys.NewStore(coll.Writeable().Underlying(), mgorootkeystore.Policy{
				ExpiryDuration: expireAfter,
			})
		},
	})
}

// NewBakeryConfig returns a new bakerystorage.BakeryConfig instance.
func (st *State) NewBakeryConfig() bakerystorage.BakeryConfig {
	collectionGetter := func(collection string) (mongo.Collection, func()) {
		return st.db().GetCollection(collection)
	}
	return bakerystorage.NewBakeryConfig(controllersC, collectionGetter)
}
