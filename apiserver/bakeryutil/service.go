// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bakeryutil

import (
	"context"
	"time"

	"gopkg.in/macaroon-bakery.v3/bakery"
	"gopkg.in/macaroon-bakery.v3/bakery/checkers"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/state/bakerystorage"
)

// BakeryThirdPartyLocator is an implementation of
// bakery.BakeryThirdPartyLocator that simply returns
// the embedded public key.
type BakeryThirdPartyLocator struct {
	PublicKey bakery.PublicKey
}

// PublicKeyForLocation implements bakery.PublicKeyLocator.
func (b BakeryThirdPartyLocator) ThirdPartyInfo(ctx context.Context, loc string) (bakery.ThirdPartyInfo, error) {
	return bakery.ThirdPartyInfo{
		PublicKey: b.PublicKey,
		Version:   bakery.LatestVersion,
	}, nil
}

// ExpirableStorageBakery wraps bakery.Bakery,
// adding the ExpireStorageAfter method.
type ExpirableStorageBakery struct {
	*bakery.Bakery
	Location string
	Key      *bakery.KeyPair
	Store    bakerystorage.ExpirableStorage
	Locator  bakery.ThirdPartyLocator
}

// ExpireStorageAfter implements authentication.ExpirableStorageBakery.
func (s *ExpirableStorageBakery) ExpireStorageAfter(t time.Duration) (authentication.ExpirableStorageBakery, error) {
	store := s.Store.ExpireAfter(t)
	service := bakery.New(bakery.BakeryParams{
		Location:     s.Location,
		RootKeyStore: store,
		Key:          s.Key,
		Locator:      s.Locator,
	})
	return &ExpirableStorageBakery{service, s.Location, s.Key, store, s.Locator}, nil
}

// NewMacaroon implements MacaroonMinter.NewMacaroon.
func (s *ExpirableStorageBakery) NewMacaroon(ctx context.Context, version bakery.Version, caveats []checkers.Caveat, ops ...bakery.Op) (*bakery.Macaroon, error) {
	return s.Oven.NewMacaroon(ctx, version, caveats, ops...)
}

// Auth implements MacaroonChecker.Auth.
func (s *ExpirableStorageBakery) Auth(mss ...macaroon.Slice) *bakery.AuthChecker {
	return s.Checker.Auth(mss...)
}
