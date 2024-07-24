// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package macaroon

import (
	"context"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/dbrootkeystore"
)

// ExpirableStorage extends bakery.Storage with the ExpireAfter method,
// to expire data added at the specified time.
type ExpirableStorage interface {
	bakery.RootKeyStore

	// ExpireAfter returns a new ExpirableStorage that will expire
	// added items after the specified duration.
	ExpireAfter(time.Duration) ExpirableStorage
}

// DefaultExpiration is a sensible default duration for root keys before expiration.
var DefaultExpiration = 24 * time.Hour

// NewExpirableStorage returns an implementation of bakery.Storage
// that stores all items in DQLite with an expiry time.
func NewExpirableStorage(backing dbrootkeystore.ContextBacking, expireAfter time.Duration, clock dbrootkeystore.Clock) ExpirableStorage {
	store := dbrootkeystore.NewRootKeys(5, clock).NewContextStore(backing, dbrootkeystore.Policy{
		ExpiryDuration: expireAfter,
	})
	return &storage{
		RootKeyStore: store,
		backing:      backing,
		clock:        clock,
	}
}

type storage struct {
	bakery.RootKeyStore
	backing dbrootkeystore.ContextBacking
	clock   dbrootkeystore.Clock
}

// Get (ExpirableStorage) returns the root key for the given id.
// If the item is not there, it returns bakery.ErrNotFound.
func (s *storage) Get(ctx context.Context, id []byte) ([]byte, error) {
	return s.RootKeyStore.Get(ctx, id)
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
	return s.RootKeyStore.RootKey(ctx)
}

// ExpireAfter (ExpirableStorage) returns a new ExpirableStorage that will expire
// added items after the specified duration.
func (s *storage) ExpireAfter(expireAfter time.Duration) ExpirableStorage {
	return NewExpirableStorage(s.backing, expireAfter, s.clock)
}
