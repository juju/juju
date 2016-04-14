// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state/bakerystorage"
)

// NewBakeryStorage returns a new bakery.Storage. By default, items
// added to the store are retained until deleted explicitly. The
// store's ExpireAt method can be called to derive a new store that
// will expire items at the specified time.
func (st *State) NewBakeryStorage() (bakerystorage.ExpirableStorage, error) {
	return bakerystorage.New(bakerystorage.Config{
		GetCollection: func() (mongo.Collection, func()) {
			return st.getCollection(bakeryStorageItemsC)
		},
	})
}
