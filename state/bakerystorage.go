// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"time"

	"github.com/juju/juju/state/bakerystorage"

	"gopkg.in/macaroon-bakery.v1/bakery"
)

// NewBakeryStorage returns a new bakery.Storage that will remove any
// entries after the "expiry" duration starting from when they are
// added to storage.
func (st *State) NewBakeryStorage(expiry time.Duration) (bakery.Storage, error) {
	return bakerystorage.New(bakerystorage.Config{
		GetCollection: st.getCollection,
		Collection:    bakeryStorageItemsC,
		Clock:         GetClock(),
		ExpireAfter:   expiry,
	})
}
