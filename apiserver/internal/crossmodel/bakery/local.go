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
	oven   Oven
	clock  clock.Clock
	logger logger.Logger
}

// NewLocalOfferBakery returns a new LocalOfferBakery.
func NewLocalOfferBakery(
	keyPair *bakery.KeyPair,
	location string,
	backingStore BakeryStore,
	checker bakery.FirstPartyCaveatChecker,
	authorizer bakery.OpsAuthorizer,
	clock clock.Clock,
	logger logger.Logger,
) (*LocalOfferBakery, error) {

	store := internalmacaroon.NewExpirableStorage(backingStore, offerPermissionExpiryTime, clock)

	locator := bakeryutil.BakeryThirdPartyLocator{PublicKey: keyPair.Public}
	localOfferBakery := bakery.New(bakery.BakeryParams{
		Location:      location,
		Locator:       locator,
		RootKeyStore:  store,
		Checker:       checker,
		OpsAuthorizer: authorizer,
	})
	bakery := &bakeryutil.ExpirableStorageBakery{
		Bakery:   localOfferBakery,
		Location: location,
		Locator:  locator,
		Store:    store,
		Key:      keyPair,
	}

	return &LocalOfferBakery{
		oven:   bakery,
		clock:  clock,
		logger: logger,
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

// InferDeclaredFromMacaroon returns the declared attributes from the macaroon.
func (o *LocalOfferBakery) InferDeclaredFromMacaroon(mac macaroon.Slice, requiredValues map[string]string) map[string]string {
	return checkers.InferDeclared(internalmacaroon.MacaroonNamespace, mac)
}

// CreateDischargeMacaroon creates a discharge macaroon.
func (o *LocalOfferBakery) CreateDischargeMacaroon(
	ctx context.Context, accessEndpoint, username string,
	requiredValues, declaredValues map[string]string,
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
					Location:  accessEndpoint,
					Condition: offerPermissionCaveat + " " + authYaml,
				},
				requiredKeys...,
			),
			checkers.TimeBeforeCaveat(o.clock.Now().Add(offerPermissionExpiryTime)),
		}, op,
	)
}
