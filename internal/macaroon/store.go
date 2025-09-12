// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package macaroon

import (
	"context"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/dbrootkeystore"
)

// DefaultExpiration is a sensible default duration for root keys before expiration.
var DefaultExpiration = 24 * time.Hour

type storage struct {
	store bakery.RootKeyStore
}

// NewRootKeyStore returns a new root key store that uses the given backing
// store to persist root keys with the given expiration duration. The clock is
// used to determine when keys expire.
func NewRootKeyStore(backing dbrootkeystore.ContextBacking, expireAfter time.Duration, clock dbrootkeystore.Clock) bakery.RootKeyStore {
	return &storage{
		store: dbrootkeystore.NewRootKeys(5, clock).NewContextStore(backing, dbrootkeystore.Policy{
			ExpiryDuration: expireAfter,
		}),
	}
}

// Get (ExpirableStorage) returns the root key for the given id.
// If the item is not there, it returns bakery.ErrNotFound.
func (s *storage) Get(ctx context.Context, id []byte) ([]byte, error) {
	return s.store.Get(ctx, id)
}

// RootKey (ExpirableStorage) returns the root key to be used for
// making a new macaroon, and an id that can be used to look it up
// later with the Get method.
//
// Note that the root keys should remain available for as long
// as the macaroons using them are valid.
//
// Note that there is no need for it to return a new root key
// for every call - keys may be reused, although some key
// cycling is over time is advisable.
func (s *storage) RootKey(ctx context.Context) ([]byte, []byte, error) {
	return s.store.RootKey(ctx)
}
