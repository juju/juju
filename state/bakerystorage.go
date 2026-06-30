// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/errors"

	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state/bakerystorage"
)

// NewBakeryStorage returns a new bakery.Storage. By default, items
// added to the store are retained until deleted explicitly. The
// store's ExpireAfter method can be called to derive a new store that
// will expire items at the specified time.
func (st *State) NewBakeryStorage() (bakerystorage.ExpirableStorage, error) {
	return bakerystorage.New(bakerystorage.Config{
		GetCollection: func() (mongo.Collection, func(), error) {
			coll, closer, err := st.db().GetCollection(bakeryStorageItemsC)
			if err != nil {
				return nil, nil, errors.Annotate(err, "getting bakery storage collection")
			}
			return coll, closer, nil
		},
		GetStorage: func(rootKeys *bakerystorage.RootKeys, coll mongo.Collection, expireAfter time.Duration) bakery.RootKeyStore {
			return rootKeys.NewStore(coll.Writeable().Underlying(), bakerystorage.Policy{
				ExpiryDuration: expireAfter,
			})
		},
	})
}

// NewBakeryConfig returns a new bakerystorage.BakeryConfig instance.
func (st *State) NewBakeryConfig() bakerystorage.BakeryConfig {
	collectionGetter := func(collection string) (mongo.Collection, func(), error) {
		coll, closer, err := st.db().GetCollection(collection)
		if err != nil {
			return nil, nil, errors.Annotatef(err, "getting bakery config collection %q", collection)
		}
		return coll, closer, nil
	}
	return bakerystorage.NewBakeryConfig(controllersC, collectionGetter)
}
