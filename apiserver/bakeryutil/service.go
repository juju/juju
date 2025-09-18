// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bakeryutil

import (
	"context"
	"encoding/json"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/checkers"
	"gopkg.in/macaroon.v2"

	corelogger "github.com/juju/juju/core/logger"
	internallogger "github.com/juju/juju/internal/logger"
)

var logger = internallogger.GetLogger("juju.apiserver.bakery")

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

// StorageBakery wraps bakery.Bakery,
type StorageBakery struct {
	*bakery.Bakery
}

// NewMacaroon takes a macaroon with the given version from the oven, associates
// it with the given operations and attaches the given caveats. There must be at
// least one operation specified.
func (s *StorageBakery) NewMacaroon(ctx context.Context, version bakery.Version, caveats []checkers.Caveat, ops ...bakery.Op) (*bakery.Macaroon, error) {
	return s.Oven.NewMacaroon(ctx, version, caveats, ops...)
}

// Auth makes a new AuthChecker instance using the given macaroons to inform
// authorization decisions.
func (s *StorageBakery) Auth(ctx context.Context, mss ...macaroon.Slice) *bakery.AuthChecker {
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
