// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bakeryutil

import (
	"context"
	"encoding/json"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/checkers"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/apiserver/authentication"
	corelogger "github.com/juju/juju/core/logger"
	internallogger "github.com/juju/juju/internal/logger"
	internalmacaroon "github.com/juju/juju/internal/macaroon"
)

// BakeryThirdPartyLocator is an implementation of
// bakery.BakeryThirdPartyLocator that simply returns
// the embedded public key.
type BakeryThirdPartyLocator struct {
	PublicKey bakery.PublicKey
}

// ThirdPartyInfo implements bakery.PublicKeyLocator.
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
	Store    internalmacaroon.ExpirableStorage
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

var logger = internallogger.GetLogger("juju.apiserver.bakery")

// Auth implements MacaroonChecker.Auth.
func (s *ExpirableStorageBakery) Auth(ctx context.Context, mss ...macaroon.Slice) *bakery.AuthChecker {
	if logger.IsLevelEnabled(corelogger.TRACE) {
		for i, ms := range mss {
			ops, conditions, err := s.Oven.VerifyMacaroon(ctx, ms)
			if err != nil {
				mac, _ := json.Marshal(ms)
				logger.Tracef(ctx, "verify macaroon err: %v\nfor\n%s", err, mac)
				continue
			}
			logger.Tracef(ctx, "macaroon %d: %+v : %v", i, ops, conditions)
		}
	}
	return s.Checker.Auth(mss...)
}
