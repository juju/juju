// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bakery

import (
	"context"
	"sort"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/checkers"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/apiserver/bakeryutil"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/permission"
	internalmacaroon "github.com/juju/juju/internal/macaroon"
)

// LocalOfferBakery provides a bakery for local offer access.
type LocalOfferBakery struct {
	baseBakery
	oven     Oven
	endpoint string
	clock    clock.Clock
	logger   logger.Logger
}

// NewLocalOfferBakery returns a new LocalOfferBakery.
func NewLocalOfferBakery(
	keyPair *bakery.KeyPair,
	location, endpoint string,
	backingStore BakeryStore,
	checker bakery.FirstPartyCaveatChecker,
	authorizer bakery.OpsAuthorizer,
	clock clock.Clock,
	logger logger.Logger,
) (*LocalOfferBakery, error) {
	store := internalmacaroon.NewRootKeyStore(backingStore, offerPermissionExpiryTime, clock)

	locator := bakeryutil.BakeryThirdPartyLocator{PublicKey: keyPair.Public}

	bakery := &bakeryutil.StorageBakery{
		Bakery: bakery.New(bakery.BakeryParams{
			Location:      location,
			Locator:       locator,
			RootKeyStore:  store,
			Checker:       checker,
			OpsAuthorizer: authorizer,
		}),
	}

	return &LocalOfferBakery{
		baseBakery: baseBakery{checker: bakery.Checker},
		oven:       bakery,
		endpoint:   endpoint,
		clock:      clock,
		logger:     logger,
	}, nil
}

// GetConsumeOfferCaveats returns the caveats for consuming an offer.
func (o *LocalOfferBakery) GetConsumeOfferCaveats(offerUUID, sourceModelUUID, username, relation string) []checkers.Caveat {
	caveats := []checkers.Caveat{
		checkers.TimeBeforeCaveat(o.clock.Now().Add(offerPermissionExpiryTime)),
		checkers.DeclaredCaveat(sourceModelKey, sourceModelUUID),
		checkers.DeclaredCaveat(usernameKey, username),
		checkers.DeclaredCaveat(offerUUIDKey, offerUUID),
	}

	if relation != "" {
		caveats = append(caveats, checkers.DeclaredCaveat(relationKey, relation))
	}

	return caveats
}

// GetRemoteRelationCaveats returns the caveats for accessing a remote relation.
func (o *LocalOfferBakery) GetRemoteRelationCaveats(offerUUID, sourceModelUUID, username, relation string) []checkers.Caveat {
	return []checkers.Caveat{
		checkers.TimeBeforeCaveat(o.clock.Now().Add(offerPermissionExpiryTime)),
		checkers.DeclaredCaveat(sourceModelKey, sourceModelUUID),
		checkers.DeclaredCaveat(offerUUIDKey, offerUUID),
		checkers.DeclaredCaveat(usernameKey, username),
		checkers.DeclaredCaveat(relationKey, relation),
	}
}

// InferDeclaredFromMacaroon returns the declared attributes from the macaroon.
func (o *LocalOfferBakery) InferDeclaredFromMacaroon(mac macaroon.Slice, requiredValues map[string]string) DeclaredValues {
	declared := checkers.InferDeclared(internalmacaroon.MacaroonNamespace, mac)
	additional := make(map[string]string)

	for k, v := range declared {
		switch k {
		case sourceModelKey, usernameKey, offerUUIDKey, relationKey:
			// These are handled below.
		default:
			additional[k] = v
		}
	}

	var userName *string
	if user, ok := declared[usernameKey]; ok {
		userName = &user
	}

	return DeclaredValues{
		userName:        userName,
		sourceModelUUID: declared[sourceModelKey],
		offerUUID:       declared[offerUUIDKey],
		relationKey:     declared[relationKey],
		additional:      additional,
	}
}

// NewMacaroon creates a new macaroon for the given version, caveats and ops.
func (o *LocalOfferBakery) NewMacaroon(ctx context.Context, version bakery.Version, caveats []checkers.Caveat, ops ...bakery.Op) (*bakery.Macaroon, error) {
	return o.oven.NewMacaroon(ctx, version, caveats, ops...)
}

// CreateDischargeMacaroon creates a discharge macaroon.
func (o *LocalOfferBakery) CreateDischargeMacaroon(
	ctx context.Context, username string,
	requiredValues map[string]string,
	declaredValues DeclaredValues,
	op bakery.Op, version bakery.Version,
) (*bakery.Macaroon, error) {
	// TODO (stickupkid): If these are required values we should check that
	// they're not empty.
	requiredSourceModelUUID := requiredValues[sourceModelKey]
	requiredOffer := requiredValues[offerUUIDKey]
	requiredRelation := requiredValues[relationKey]

	authYaml, err := encodeOfferAccessDetails(
		requiredSourceModelUUID, username, requiredOffer, requiredRelation,
		permission.ConsumeAccess,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}

	requiredKeys := []string{usernameKey}
	for k := range requiredValues {
		requiredKeys = append(requiredKeys, k)
	}
	sort.Strings(requiredKeys)

	return o.oven.NewMacaroon(
		ctx,
		version,
		[]checkers.Caveat{
			checkers.NeedDeclaredCaveat(
				checkers.Caveat{
					Location:  o.endpoint,
					Condition: offerPermissionCaveat + " " + authYaml,
				},
				requiredKeys...,
			),
			checkers.TimeBeforeCaveat(o.clock.Now().Add(offerPermissionExpiryTime)),
		}, op,
	)
}
