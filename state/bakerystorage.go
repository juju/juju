// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"time"

	"gopkg.in/macaroon-bakery.v2-unstable/bakery"
	"gopkg.in/macaroon-bakery.v2-unstable/bakery/mgostorage"

	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state/bakerystorage"
)

// NewBakeryStorage returns a new bakery.Storage. By default, items
// added to the store are retained until deleted explicitly. The
// store's ExpireAfter method can be called to derive a new store that
// will expire items at the specified time.
func (st *State) NewBakeryStorage() (bakerystorage.ExpirableStorage, error) {
	return bakerystorage.New(bakerystorage.Config{
		GetCollection: func() (mongo.Collection, func()) {
			return st.db().GetCollection(bakeryV2StorageItemsC)
		},
		GetLegacyCollection: func() (mongo.Collection, func()) {
			return st.db().GetCollection(bakeryV2StorageItemsC)
		},
		GetStorage: func(rootKeys *mgostorage.RootKeys, coll mongo.Collection, expireAfter time.Duration) bakery.Storage {
			return rootKeys.NewStorage(coll.Writeable().Underlying(), mgostorage.Policy{
				ExpiryDuration: expireAfter,
			})
		},
	})
}
